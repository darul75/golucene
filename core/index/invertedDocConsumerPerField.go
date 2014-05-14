package index

import (
	"github.com/balzaczyy/golucene/core/index/model"
	"github.com/balzaczyy/golucene/core/util"
)

// index/InvertedDocConsumerPerField.java

type InvertedDocConsumerPerField interface {
	// Called on hitting an aborting error
	abort()
}

const HASH_INIT_SIZE = 4

type TermsHashPerField struct {
	consumer TermsHashConsumerPerField

	termsHash *TermsHash

	nextPerField *TermsHashPerField
	docState     *docState
	fieldState   *FieldInvertState

	// Copied from our perThread
	intPool      *util.IntBlockPool
	bytePool     *util.ByteBlockPool
	termBytePool *util.ByteBlockPool

	streamCount   int
	numPostingInt int

	fieldInfo *model.FieldInfo

	bytesHash *util.BytesRefHash

	postingsArray *ParallelPostingsArray
	bytesUsed     util.Counter
}

func newTermsHashPerField(docInverterPerField *DocInverterPerField,
	termsHash *TermsHash, nextTermsHash *TermsHash,
	fieldInfo *model.FieldInfo) *TermsHashPerField {

	ans := &TermsHashPerField{
		intPool:      termsHash.intPool,
		bytePool:     termsHash.bytePool,
		termBytePool: termsHash.termBytePool,
		docState:     termsHash.docState,
		termsHash:    termsHash,
		bytesUsed:    termsHash.bytesUsed,
		fieldState:   docInverterPerField.fieldState,
		fieldInfo:    fieldInfo,
	}
	ans.consumer = termsHash.consumer.addField(ans, fieldInfo)
	byteStarts := newPostingsBytesStartArray(ans, termsHash.bytesUsed)
	ans.bytesHash = util.NewBytesRefHash(termsHash.termBytePool, HASH_INIT_SIZE, byteStarts)
	ans.streamCount = ans.consumer.streamCount()
	ans.numPostingInt = 2 * ans.streamCount
	if nextTermsHash != nil {
		ans.nextPerField = nextTermsHash.addField(docInverterPerField, fieldInfo).(*TermsHashPerField)
	}
	return ans
}

func (h *TermsHashPerField) shrinkHash(targetSize int) {
	// Fully free the bytesHash on each flush but keep the pool
	// untouched. bytesHash.clear will clear the BytesStartArray and
	// in turn the ParallelPostingsArray too
	h.bytesHash.Clear(false)
}

func (h *TermsHashPerField) reset() {
	h.bytesHash.Clear(false)
	if h.nextPerField != nil {
		h.nextPerField.reset()
	}
}

func (h *TermsHashPerField) abort() {
	h.reset()
	if h.nextPerField != nil {
		h.nextPerField.abort()
	}
}

type PostingsBytesStartArray struct {
	perField  *TermsHashPerField
	bytesUsed util.Counter
}

func newPostingsBytesStartArray(perField *TermsHashPerField,
	bytesUsed util.Counter) *PostingsBytesStartArray {
	return &PostingsBytesStartArray{perField, bytesUsed}
}

func (ss *PostingsBytesStartArray) Init() []int {
	if ss.perField.postingsArray == nil {
		arr := ss.perField.consumer.createPostingsArray(2)
		ss.bytesUsed.AddAndGet(int64(arr.size * arr.bytesPerPosting()))
		ss.perField.postingsArray = arr
	}
	return ss.perField.postingsArray.textStarts
}

func (ss *PostingsBytesStartArray) Clear() []int {
	if ss.perField.postingsArray != nil {
		ss.bytesUsed.AddAndGet(-int64(ss.perField.postingsArray.size * ss.perField.postingsArray.bytesPerPosting()))
		ss.perField.postingsArray = nil
	}
	return nil
}

func (ss *PostingsBytesStartArray) BytesUsed() util.Counter {
	return ss.bytesUsed
}

// index/ParallelPostingsArray.java

const BYTES_PER_POSTING = 3 * util.NUM_BYTES_INT

type PostingsArray interface {
	bytesPerPosting() int
	newInstance(size int) PostingsArray
	copyTo(toArray PostingsArray, numToCopy int)
}

type ParallelPostingsArray struct {
	PostingsArray
	size       int
	textStarts []int
	intStarts  []int
	byteStarts []int
}

func newParallelPostingsArray(spi PostingsArray, size int) *ParallelPostingsArray {
	return &ParallelPostingsArray{
		PostingsArray: spi,
		size:          size,
		textStarts:    make([]int, size),
		intStarts:     make([]int, size),
		byteStarts:    make([]int, size),
	}
}

func (arr *ParallelPostingsArray) grow() *ParallelPostingsArray {
	panic("not implemented yet")
}
