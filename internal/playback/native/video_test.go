//go:build linux || (darwin && cgo)

package native

import "time"

// fakeVideo records a fakeHost's video calls.
type fakeVideo struct {
	loads []videoLoad
	feeds []feedCall
	ends  []uint64
}

type videoLoad struct {
	track uint64
	init  []byte
	kid   []byte
	start time.Duration
}
