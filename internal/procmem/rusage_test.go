package procmem

import (
	"encoding/binary"
	"testing"
)

func TestDecodeRusage(t *testing.T) {
	b := make([]byte, rusageInfoV0Size)
	for i := range 10 { // the ten counters after the UUID, each its own index + 1
		binary.LittleEndian.PutUint64(b[16+i*8:], uint64(i+1))
	}
	u, ok := decodeRusage(b)
	// ri_resident_size is the 7th counter and ri_phys_footprint the 8th.
	if !ok || u.Resident != 7 || u.Footprint != 8 {
		t.Errorf("%+v %v: want resident 7 and footprint 8 (the 7th and 8th counters)", u, ok)
	}
	if _, ok := decodeRusage(b[:rusageInfoV0Size-1]); ok {
		t.Error("a short buffer was decoded")
	}
}
