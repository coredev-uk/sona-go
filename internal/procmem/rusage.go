package procmem

import "encoding/binary"

// rusageInfoV0 is the layout macOS's proc_pid_rusage fills for flavor
// RUSAGE_INFO_V0 (<sys/resource.h>, struct rusage_info_v0): a 16-byte UUID
// and then 64-bit counters, in this order: ri_user_time, ri_system_time,
// ri_pkg_idle_wkups, ri_interrupt_wkups, ri_pageins, ri_wired_size,
// ri_resident_size, ri_phys_footprint, ri_proc_start_abstime,
// ri_proc_exit_abstime. Later flavors only append fields.
const (
	rusageInfoV0Size    = 16 + 10*8
	rusageResidentAt    = 16 + 6*8
	rusagePhysFootprint = 16 + 7*8
)

// decodeRusage reads the two sizes out of a rusage_info buffer (native byte
// order is little-endian on every Mac Sona runs on). It is plain Go so Linux
// tests cover the layout; only the call is macOS's.
func decodeRusage(b []byte) (Usage, bool) {
	if len(b) < rusageInfoV0Size {
		return Usage{}, false
	}
	return Usage{
		Resident:  binary.LittleEndian.Uint64(b[rusageResidentAt:]),
		Footprint: binary.LittleEndian.Uint64(b[rusagePhysFootprint:]),
	}, true
}
