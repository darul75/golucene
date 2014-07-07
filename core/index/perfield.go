package index

import (
	"fmt"
	"github.com/balzaczyy/golucene/core/index/model"
	"github.com/balzaczyy/golucene/core/util"
	"io"
	"log"
	"strconv"
)

// perfield/PerFieldPostingsFormat.java

/*
Enables per field postings support.

Note, when extending this class, the name Name() is written into the
index. In order for the field to be read, the name must resolve to
your implementation via LoadXYZ(). This method use hard-coded map to
resolve codec names.

Files written by each posting format have an additional suffix containing
the format name. For example, in a per-field configuration instead of
_1.prx fielnames would look like _1_Lucene40_0.prx.
*/
type PerFieldPostingsFormat struct {
	postingsFormatForField func(string) PostingsFormat
}

func newPerFieldPostingsFormat(f func(field string) PostingsFormat) *PerFieldPostingsFormat {
	return &PerFieldPostingsFormat{f}
}

func (pf *PerFieldPostingsFormat) Name() string {
	return "PerField40"
}

func (pf *PerFieldPostingsFormat) FieldsConsumer(state *model.SegmentWriteState) (FieldsConsumer, error) {
	return newPerFieldPostingsWriter(pf, state), nil
}

func (pf *PerFieldPostingsFormat) FieldsProducer(state SegmentReadState) (FieldsProducer, error) {
	return newPerFieldPostingsReader(state)
}

const (
	PER_FIELD_FORMAT_KEY = "PerFieldPostingsFormat.format"
	PER_FIELD_SUFFIX_KEY = "PerFieldPostingsFormat.suffix"
)

type FieldsConsumerAndSuffix struct {
	consumer FieldsConsumer
	suffix   int
}

func (fcas *FieldsConsumerAndSuffix) Close() error {
	return fcas.consumer.Close()
}

type PerFieldPostingsWriter struct {
	owner             *PerFieldPostingsFormat
	formats           map[PostingsFormat]*FieldsConsumerAndSuffix
	suffixes          map[string]int
	segmentWriteState *model.SegmentWriteState
}

func newPerFieldPostingsWriter(owner *PerFieldPostingsFormat,
	state *model.SegmentWriteState) FieldsConsumer {
	return &PerFieldPostingsWriter{
		owner,
		make(map[PostingsFormat]*FieldsConsumerAndSuffix),
		make(map[string]int),
		state,
	}
}

func (w *PerFieldPostingsWriter) addField(field *model.FieldInfo) (TermsConsumer, error) {
	format := w.owner.postingsFormatForField(field.Name)
	assert2(format != nil, "invalid nil PostingsFormat for field='%v'", field.Name)
	formatName := format.Name()

	previousValue := field.PutAttribute(PER_FIELD_FORMAT_KEY, formatName)
	assert(previousValue == "")

	var suffix int

	consumer, ok := w.formats[format]
	if !ok {
		// First time we are seeing this format; create a new instance

		// bump the suffix
		if suffix, ok = w.suffixes[formatName]; !ok {
			suffix = 0
		} else {
			suffix = suffix + 1
		}
		w.suffixes[formatName] = suffix

		segmentSuffix := fullSegmentSuffix(field.Name,
			w.segmentWriteState.SegmentSuffix,
			_suffix(formatName, strconv.Itoa(suffix)))

		consumer = new(FieldsConsumerAndSuffix)
		var err error
		consumer.consumer, err = format.FieldsConsumer(
			model.NewSegmentWriteStateFrom(w.segmentWriteState, segmentSuffix))
		if err != nil {
			return nil, err
		}
		consumer.suffix = suffix
		w.formats[format] = consumer
	} else {
		panic("not implemented yet")
	}

	previousValue = field.PutAttribute(PER_FIELD_SUFFIX_KEY, fmt.Sprintf("%v", suffix))
	assert(previousValue == "")

	// TODO: we should only provide the "slice" of FIS that this PF
	// actually sees ... then stuff like .hasProx could work correctly?
	// NOTE: .hasProx is already broken in the same way for the
	// non-perfield case, if there is a fieldInfo with prox that has no
	// postings, you get a 0 byte file.
	return consumer.consumer.addField(field)
}

func (w *PerFieldPostingsWriter) Close() error {
	var subs []io.Closer
	for _, v := range w.formats {
		subs = append(subs, v)
	}
	return util.Close(subs...)
}

func _suffix(formatName, suffix string) string {
	return formatName + "_" + suffix
}

func fullSegmentSuffix(fieldName, outerSegmentSuffix, segmentSuffix string) string {
	if len(outerSegmentSuffix) == 0 {
		return segmentSuffix
	}
	// TODO: support embedding; I think it should work but
	// we need a test confirm to confirm
	// return outerSegmentSuffix + "_" + segmentSuffix;
	panic(fmt.Sprintf(
		"cannot embed PerFieldPostingsFormat inside itself (field '%v' returned PerFieldPostingsFormat)",
		fieldName))
}

type PerFieldPostingsReader struct {
	fields  map[string]FieldsProducer
	formats map[string]FieldsProducer
}

func newPerFieldPostingsReader(state SegmentReadState) (fp FieldsProducer, err error) {
	ans := PerFieldPostingsReader{
		make(map[string]FieldsProducer),
		make(map[string]FieldsProducer),
	}
	// Read _X.per and init each format:
	success := false
	defer func() {
		if !success {
			log.Printf("Failed to initialize PerFieldPostingsReader.")
			if err != nil {
				log.Print("DEBUG ", err)
			}
			fps := make([]FieldsProducer, 0)
			for _, v := range ans.formats {
				fps = append(fps, v)
			}
			items := make([]io.Closer, len(fps))
			for i, v := range fps {
				items[i] = v
			}
			util.CloseWhileSuppressingError(items...)
		}
	}()
	// Read field name -> format name
	for _, fi := range state.fieldInfos.Values {
		log.Printf("Processing %v...", fi)
		if fi.IsIndexed() {
			fieldName := fi.Name
			log.Printf("Name: %v", fieldName)
			if formatName := fi.Attribute(PER_FIELD_FORMAT_KEY); formatName != "" {
				log.Printf("Format: %v", formatName)
				// null formatName means the field is in fieldInfos, but has no postings!
				suffix := fi.Attribute(PER_FIELD_SUFFIX_KEY)
				log.Printf("Suffix: %v", suffix)
				assert(suffix != "")
				format := LoadPostingsFormat(formatName)
				segmentSuffix := formatName + "_" + suffix
				log.Printf("Segment suffix: %v", segmentSuffix)
				if _, ok := ans.formats[segmentSuffix]; !ok {
					log.Printf("Loading fields producer: %v", segmentSuffix)
					newReadState := state // clone
					newReadState.segmentSuffix = formatName + "_" + suffix
					fp, err = format.FieldsProducer(newReadState)
					if err != nil {
						return fp, err
					}
					ans.formats[segmentSuffix] = fp
				}
				ans.fields[fieldName] = ans.formats[segmentSuffix]
			}
		}
	}
	success = true
	return &ans, nil
}

func (r *PerFieldPostingsReader) Terms(field string) Terms {
	if p, ok := r.fields[field]; ok {
		return p.Terms(field)
	}
	return nil
}

func (r *PerFieldPostingsReader) Close() error {
	fps := make([]FieldsProducer, 0)
	for _, v := range r.formats {
		fps = append(fps, v)
	}
	items := make([]io.Closer, len(fps))
	for i, v := range fps {
		items[i] = v
	}
	return util.Close(items...)
}

// perfield/PerFieldDocValuesFormat.java

/*
Enables per field docvalues support,

Note, when extending this class, the name Name() is written into the
index. In order for the field to be read, the name must resolve to
your implementation via LoadXYZ(). This method use hard-coded map to
resolve codec names.

Files written by each docvalues format have an additional suffix
containing the format name. For example, in a per-field configuration
instead of _1.dat fielnames would look like _1_Lucene40_0.dat.
*/
type PerFieldDocValuesFormat struct {
}

func newPerFieldDocValuesFormat(f func(field string) DocValuesFormat) *PerFieldDocValuesFormat {
	return &PerFieldDocValuesFormat{}
}

func (pf *PerFieldDocValuesFormat) Name() string {
	return "PerFieldDV40"
}

func (pf *PerFieldDocValuesFormat) FieldsConsumer(state *model.SegmentWriteState) (w DocValuesConsumer, err error) {
	panic("not implemented yet")
}

func (pf *PerFieldDocValuesFormat) FieldsProducer(state SegmentReadState) (r DocValuesProducer, err error) {
	return newPerFieldDocValuesReader(state)
}

type PerFieldDocValuesReader struct {
	fields  map[string]DocValuesProducer
	formats map[string]DocValuesProducer
}

func newPerFieldDocValuesReader(state SegmentReadState) (dvp DocValuesProducer, err error) {
	ans := PerFieldDocValuesReader{
		make(map[string]DocValuesProducer), make(map[string]DocValuesProducer)}
	// Read _X.per and init each format:
	success := false
	defer func() {
		if !success {
			fps := make([]DocValuesProducer, 0)
			for _, v := range ans.formats {
				fps = append(fps, v)
			}
			items := make([]io.Closer, len(fps))
			for i, v := range fps {
				items[i] = v
			}
			util.CloseWhileSuppressingError(items...)
		}
	}()
	// Read field name -> format name
	for _, fi := range state.fieldInfos.Values {
		if fi.HasDocValues() {
			fieldName := fi.Name
			if formatName := fi.Attribute(PER_FIELD_FORMAT_KEY); formatName != "" {
				// null formatName means the field is in fieldInfos, but has no docvalues!
				suffix := fi.Attribute(PER_FIELD_SUFFIX_KEY)
				// assert suffix != nil
				segmentSuffix := formatName + "_" + suffix
				if _, ok := ans.formats[segmentSuffix]; !ok {
					newReadState := state // clone
					newReadState.segmentSuffix = formatName + "_" + suffix
					if p, err := LoadDocValuesProducer(formatName, newReadState); err == nil {
						ans.formats[segmentSuffix] = p
					}
				}
				ans.fields[fieldName] = ans.formats[segmentSuffix]
			}
		}
	}
	success = true
	return &ans, nil
}

func (dvp *PerFieldDocValuesReader) Numeric(field *model.FieldInfo) (v NumericDocValues, err error) {
	if p, ok := dvp.fields[field.Name]; ok {
		return p.Numeric(field)
	}
	return nil, nil
}

func (dvp *PerFieldDocValuesReader) Binary(field *model.FieldInfo) (v BinaryDocValues, err error) {
	if p, ok := dvp.fields[field.Name]; ok {
		return p.Binary(field)
	}
	return nil, nil
}

func (dvp *PerFieldDocValuesReader) Sorted(field *model.FieldInfo) (v SortedDocValues, err error) {
	if p, ok := dvp.fields[field.Name]; ok {
		return p.Sorted(field)
	}
	return nil, nil
}

func (dvp *PerFieldDocValuesReader) SortedSet(field *model.FieldInfo) (v SortedSetDocValues, err error) {
	if p, ok := dvp.fields[field.Name]; ok {
		return p.SortedSet(field)
	}
	return nil, nil
}

func (dvp *PerFieldDocValuesReader) Close() error {
	fps := make([]DocValuesProducer, 0)
	for _, v := range dvp.formats {
		fps = append(fps, v)
	}
	items := make([]io.Closer, len(fps))
	for i, v := range fps {
		items[i] = v
	}
	return util.Close(items...)
}
