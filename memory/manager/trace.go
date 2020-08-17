package manager

import (
	"os"
	"sort"
	"sync"

	log "github.com/sirupsen/logrus"
)

// Record A tuple with an address
type Record struct {
	offset uint64
}

// Trace Contains records
type Trace struct {
	sync.Mutex
	traceFileName string

	containedOffsets map[uint64]int
	trace            []Record
	regions          map[uint64]int
	offsetIndex      map[uint64]int
}

func initTrace(traceFileName string) *Trace {
	t := new(Trace)

	t.traceFileName = traceFileName
	t.regions = make(map[uint64]int)
	t.containedOffsets = make(map[uint64]int)
	t.trace = make([]Record, 0)
	t.offsetIndex = make(map[uint64]int)

	return t
}

// AppendRecord Appends a record to the trace
func (t *Trace) AppendRecord(r Record) {
	t.Lock()
	defer t.Unlock()

	t.trace = append(t.trace, r)
	t.containedOffsets[r.offset] = 0
}

// Search trace for the record with the same offset
func (t *Trace) containsRecord(rec Record) bool {
	_, ok := t.containedOffsets[rec.offset]

	return ok
}

// ProcessRecord Prepares the trace, the regions map, and the working set file for replay
// Must be called when record is done (i.e., it is not concurrency-safe vs. AppendRecord)
func (t *Trace) ProcessRecord() {
	log.Debug("Preparing replay structures")

	// sort trace records in the ascending order by offset
	sort.Slice(t.trace, func(i, j int) bool {
		return t.trace[i].offset < t.trace[j].offset
	})

	// build the map of contiguous regions from the trace records
	var last, regionStart uint64
	for _, rec := range t.trace {
		if rec.offset != last+uint64(os.Getpagesize()) {
			regionStart = rec.offset
			t.regions[regionStart] = 1
		} else {
			t.regions[regionStart]++
		}

		last = rec.offset
	}
}
