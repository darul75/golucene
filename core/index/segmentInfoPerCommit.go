package index

import (
	"fmt"
	"github.com/balzaczyy/golucene/core/index/model"
	"github.com/balzaczyy/golucene/core/store"
)

// index/SegmentCommitInfo.java

// Embeds a [read-only] SegmentInfo and adds per-commit fields.
type SegmentCommitInfo struct {
	// The SegmentInfo that we wrap.
	info *model.SegmentInfo
	// How many deleted docs in the segment:
	delCount int
	// Generation number of the live docs file (-1 if there are no deletes yet)
	delGen int64
	// Normally 1+delGen, unless an exception was hit on last attempt to write:
	nextWriteDelGen int64

	sizeInBytes int64 // volatile

	// NOTE: only used by in-RAM by IW to track buffered deletes;
	// this is never written to/read from the Directory
	BufferedUpdatesGen int64
}

func NewSegmentCommitInfo(info *model.SegmentInfo,
	delCount int, delGen, fieldInfosGen, docValuesGen int64) *SegmentCommitInfo {

	panic("not implemented yet")

	nextWriteDelGen := int64(1)
	if delGen != -1 {
		nextWriteDelGen = delGen + 1
	}
	return &SegmentCommitInfo{
		info:            info,
		delCount:        delCount,
		delGen:          delGen,
		nextWriteDelGen: nextWriteDelGen,
		sizeInBytes:     -1,
	}
}

/* Called when we succeed in writing deletes */
func (info *SegmentCommitInfo) advanceDelGen() {
	info.delGen, info.nextWriteDelGen = info.nextWriteDelGen, info.delGen+1
	info.sizeInBytes = -1
}

/*
Called if there was an error while writing deletes, so that we don't
try to write to the same file more than once.
*/
func (info *SegmentCommitInfo) advanceNextWriteDelGen() {
	info.nextWriteDelGen++
}

/*
Returns total size in bytes of all files for this segment.

NOTE: This value is not correct for 3.0 segments that have shared
docstores. To get correct value, upgrade.
*/
func (si *SegmentCommitInfo) SizeInBytes() (sum int64, err error) {
	if si.sizeInBytes == -1 {
		sum = 0
		for _, fileName := range si.Files() {
			d, err := si.info.Dir.FileLength(fileName)
			if err != nil {
				return 0, err
			}
			sum += d
		}
		si.sizeInBytes = sum
	}
	return si.sizeInBytes, nil
}

// Returns all files in use by this segment.
func (si *SegmentCommitInfo) Files() []string {
	panic("not implemented yet")
	// Start from the wrapped info's files:
	files := make(map[string]bool)
	for name, _ := range si.info.Files() {
		files[name] = true
	}

	// Must separately add any live docs files
	for _, name := range si.info.Codec().(Codec).LiveDocsFormat().Files(si) {
		files[name] = true
	}

	ans := make([]string, 0, len(files))
	for s, _ := range files {
		ans = append(ans, s)
	}
	return ans
}

func (si *SegmentCommitInfo) setBufferedUpdatesGen(v int64) {
	si.BufferedUpdatesGen = v
	si.sizeInBytes = -1
}

// Returns true if there are any deletions for the segment at this
// commit.
func (si *SegmentCommitInfo) HasDeletions() bool {
	return si.delGen != -1
}

func (si *SegmentCommitInfo) setDelCount(delCount int) {
	assert2(delCount >= 0 && delCount <= si.info.DocCount(),
		"invalid delCount=%v (docCount=%v)", delCount, si.info.DocCount())
	si.delCount = delCount
}

func (si *SegmentCommitInfo) StringOf(dir store.Directory, pendingDelCount int) string {
	panic("not implemented yet")
	return si.info.StringOf(dir, si.delCount+pendingDelCount)
}

func (si *SegmentCommitInfo) String() string {
	panic("not implemented yet")
	s := si.info.StringOf(si.info.Dir, si.delCount)
	if si.delGen != -1 {
		s = fmt.Sprintf("%v:delGen=%v", s, si.delGen)
	}
	return s
}

func (si *SegmentCommitInfo) Clone() *SegmentCommitInfo {
	panic("not implemented yet")
	// Not clear that we need ot carry over nextWriteDelGen (i.e. do we
	// ever clone after a failed write and before the next successful
	// write?), but just do it to be safe:
	return &SegmentCommitInfo{
		info:            si.info,
		delCount:        si.delCount,
		delGen:          si.delGen,
		nextWriteDelGen: si.nextWriteDelGen,
	}
}
