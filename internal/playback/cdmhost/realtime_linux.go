package cdmhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
)

// RealtimeSupported: on Linux the parent asks RTKit or the Realtime portal
// for realtime scheduling of the child's audio threads.
const RealtimeSupported = true

// realtimeInitial is a new host's Realtime: nothing asked for yet.
var realtimeInitial = Realtime{}

// defaultRTEnv asks RTKit or the Realtime portal for the child's threads.
var defaultRTEnv = rtEnv{dial: dialBroker, limitRTTime: limitRTTime, owns: ownsThread}

// audioThreadIDs lists the child's audio threads: those libpipewire
// created, which run the output stream's processing (pipewiresink's
// pipewire-main-loop, and data-loop.N where a stream uses one). PipeWire
// creates each of its threads with SCHED_RESET_ON_FORK; nothing else in
// the child sets it. Their names would say more, but in the sandbox
// pthread_setname_np cannot write /proc/self/task/<tid>/comm (Landlock),
// so they keep the process's name. PipeWire's own module-rt cannot raise
// them from here: it needs D-Bus, or an RLIMIT_RTPRIO users rarely have.
func audioThreadIDs() []int {
	ents, err := os.ReadDir("/proc/self/task")
	if err != nil {
		return nil
	}
	var tids []int
	for _, e := range ents {
		tid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		policy, _, errno := unix.RawSyscall(unix.SYS_SCHED_GETSCHEDULER, uintptr(tid), 0, 0)
		if errno == 0 && policy&unix.SCHED_RESET_ON_FORK != 0 {
			tids = append(tids, tid)
		}
	}
	return tids
}

// ownsThread reports whether tid is one of process pid's threads. The
// child shares the parent's PID namespace, so its thread IDs are valid
// here and RTKit's.
func ownsThread(pid, tid int) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d/task/%d", pid, tid))
	return err == nil
}

// limitRTTime lowers process pid's RLIMIT_RTTIME, soft and hard, to at
// most usec: RTKit refuses a process whose limit is higher, and the child
// cannot raise a hard limit again.
func limitRTTime(pid int, usec int64) error {
	var old unix.Rlimit
	if err := unix.Prlimit(pid, unix.RLIMIT_RTTIME, nil, &old); err != nil {
		return err
	}
	v := uint64(usec)
	lim := unix.Rlimit{Cur: min(old.Cur, v), Max: min(old.Max, v)}
	if lim == old {
		return nil
	}
	return unix.Prlimit(pid, unix.RLIMIT_RTTIME, &lim, nil)
}

// D-Bus names of the brokers. The portal's methods and properties match
// RTKit's.
const (
	rtkitName  = "org.freedesktop.RealtimeKit1"
	rtkitPath  = "/org/freedesktop/RealtimeKit1"
	portalName = "org.freedesktop.portal.Desktop"
	portalPath = "/org/freedesktop/portal/desktop"
	portalRT   = "org.freedesktop.portal.Realtime"
)

// rtCallTimeout bounds each D-Bus call to a broker.
const rtCallTimeout = 5 * time.Second

// dialBroker connects to RTKit on the system bus, else to the Realtime
// portal on the session bus, which forwards to RTKit. Inside Flatpak the
// system bus is out of reach and the portal, which maps process IDs out
// of the sandbox, is the only way.
func dialBroker() (rtBroker, error) {
	var errs []string
	if _, err := os.Stat("/.flatpak-info"); err != nil {
		b, err := dialBus(dbus.ConnectSystemBus, "RTKit", rtkitName, rtkitPath, rtkitName)
		if err == nil {
			return b, nil
		}
		errs = append(errs, err.Error())
	}
	b, err := dialBus(dbus.ConnectSessionBus, "portal", portalName, portalPath, portalRT)
	if err == nil {
		return b, nil
	}
	return nil, errors.New(strings.Join(append(errs, err.Error()), "; "))
}

// busBroker is RTKit or the portal over a private bus connection.
type busBroker struct {
	label string
	conn  *dbus.Conn
	obj   dbus.BusObject
	iface string
	lim   rtLimits
}

func dialBus(connect func(...dbus.ConnOption) (*dbus.Conn, error), label, dest, path, iface string) (*busBroker, error) {
	conn, err := connect()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	b := &busBroker{label: label, conn: conn, obj: conn.Object(dest, dbus.ObjectPath(path)), iface: iface}
	maxPriority, err := b.intProperty("MaxRealtimePriority")
	if err == nil {
		b.lim.maxPriority = int(maxPriority)
		var minNice, rttime int64
		if minNice, err = b.intProperty("MinNiceLevel"); err == nil {
			b.lim.minNice = int(minNice)
			if rttime, err = b.intProperty("RTTimeUSecMax"); err == nil {
				b.lim.rttimeMax = rttime
			}
		}
	}
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	return b, nil
}

func (b *busBroker) intProperty(name string) (int64, error) {
	var v dbus.Variant
	if err := b.call("org.freedesktop.DBus.Properties.Get", &v, b.iface, name); err != nil {
		return 0, err
	}
	switch n := v.Value().(type) {
	case int32:
		return int64(n), nil
	case int64:
		return n, nil
	case uint32:
		return int64(n), nil
	}
	return 0, fmt.Errorf("%s is a %s, not an integer", name, v.Signature())
}

// call calls method, storing its reply in out unless out is nil.
func (b *busBroker) call(method string, out any, args ...any) error {
	ctx, cancel := context.WithTimeout(context.Background(), rtCallTimeout)
	defer cancel()
	c := b.obj.CallWithContext(ctx, method, 0, args...)
	err := c.Err
	if err == nil && out != nil {
		err = c.Store(out)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("no answer within %v", rtCallTimeout)
	}
	return err
}

func (b *busBroker) name() string     { return b.label }
func (b *busBroker) limits() rtLimits { return b.lim }
func (b *busBroker) close()           { _ = b.conn.Close() }

func (b *busBroker) makeRealtime(pid, tid, priority int) error {
	return b.call(b.iface+".MakeThreadRealtimeWithPID", nil, uint64(pid), uint64(tid), uint32(priority))
}

func (b *busBroker) makeHighPriority(pid, tid, nice int) error {
	return b.call(b.iface+".MakeThreadHighPriorityWithPID", nil, uint64(pid), uint64(tid), int32(nice))
}
