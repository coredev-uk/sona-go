// Package widevine hosts Google's Widevine CDM (libwidevinecdm.so from an
// installed browser) through the Chromium CDM API. A C++ shim implements the
// host interface; this package runs every CDM call on one locked OS thread,
// fires the CDM's timers, and surfaces licence messages to Go. Sona never
// downloads, copies or patches the CDM and only uses its public API.
package widevine

/*
#cgo CXXFLAGS: -std=c++17 -O2 -Wall -fno-exceptions
#cgo LDFLAGS: -ldl
#include <stdlib.h>
#include "shim.h"
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// MessageType classifies a CDM session message (cdm::MessageType).
type MessageType int

const (
	LicenseRequest MessageType = iota
	LicenseRenewal
	LicenseRelease
	IndividualizationRequest
)

func (t MessageType) String() string {
	switch t {
	case LicenseRequest:
		return "license-request"
	case LicenseRenewal:
		return "license-renewal"
	case LicenseRelease:
		return "license-release"
	case IndividualizationRequest:
		return "individualization-request"
	}
	return fmt.Sprintf("message-type-%d", int(t))
}

// Message is a CDM message for the licence server.
type Message struct {
	SessionID string
	Type      MessageType
	Data      []byte
}

// KeyStatus is cdm::KeyStatus.
type KeyStatus int

const (
	KeyUsable KeyStatus = iota
	KeyInternalError
	KeyExpired
	KeyOutputRestricted
	KeyOutputDownscaled
	KeyStatusPending
	KeyReleased
)

// Errors returned by Decrypt.
var (
	ErrNoKey    = errors.New("widevine: no key for sample")
	ErrDecrypt  = errors.New("widevine: decrypt failed")
	ErrClosed   = errors.New("widevine: CDM closed")
	ErrNoUsable = errors.New("widevine: licence has no usable key")
)

// PromiseError is a rejected CDM promise.
type PromiseError struct {
	Op         string
	Exception  int // cdm::Exception
	SystemCode uint32
	Message    string
}

func (e *PromiseError) Error() string {
	return fmt.Sprintf("widevine: %s rejected: exception %d, system code %d: %s", e.Op, e.Exception, e.SystemCode, e.Message)
}

// Interfaces this host implements, preferred first. The CDM's manifest lists
// the CDM interface (10 for Chrome's 4.10 CDMs); CDM_11 shares CDM_10's ABI.
var supportedInterfaces = []int{10, 11}

// CDM is one Widevine CDM instance.
type CDM struct {
	ptr     *C.sona_cdm
	handle  uintptr // key in instances
	log     *slog.Logger
	version string
	iface   int
	hostIf  int

	calls chan func()
	wake  chan struct{}
	done  chan struct{}
	once  sync.Once

	evMu sync.Mutex
	evq  []event

	messages chan Message
	fired    atomic.Uint64

	// Owned by the loop goroutine.
	nextPromise uint32
	promises    map[uint32]chan promiseResult
	sessions    map[string]*session
	initDone    chan bool
	timers      map[*time.Timer]struct{}
	deferred    []func() // host requests answered after the current callback batch
}

type session struct {
	request    chan Message // the licence request answering CreateSession
	gotRequest bool
	keys       map[string]KeyStatus
	expiry     time.Time
}

type promiseResult struct {
	err     error
	session string
}

type event struct {
	kind    int
	a, b, c uint32
	t       float64
	ptr     uintptr
	sid     string
	data    []byte
}

// Open loads the CDM library at path (once per process) and creates an
// initialised instance. log may be nil.
func Open(path string, log *slog.Logger) (*CDM, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var errbuf [512]C.char
	if C.sona_cdm_module_load(cpath, &errbuf[0], C.size_t(len(errbuf))) != 0 {
		return nil, fmt.Errorf("widevine: load %s: %s", path, C.GoString(&errbuf[0]))
	}
	c := &CDM{
		log:      log,
		version:  C.GoString(C.sona_cdm_module_version()),
		calls:    make(chan func(), 16),
		wake:     make(chan struct{}, 1),
		done:     make(chan struct{}),
		messages: make(chan Message, 16),
		promises: map[uint32]chan promiseResult{},
		sessions: map[string]*session{},
		timers:   map[*time.Timer]struct{}{},
		initDone: make(chan bool, 1),
	}
	c.handle = uintptr(nextHandle.Add(1))
	instances.Store(c.handle, c)
	go c.loop()

	created := make(chan error, 1)
	c.post(func() {
		for _, v := range supportedInterfaces {
			if c.ptr = C.sona_cdm_create(C.int(v), C.uintptr_t(c.handle)); c.ptr != nil {
				c.iface = v
				c.hostIf = int(C.sona_cdm_host_interface(c.ptr))
				C.sona_cdm_initialize(c.ptr, 0, 0)
				created <- nil
				return
			}
		}
		created <- fmt.Errorf("widevine: CDM %s accepts none of the interfaces %v", c.version, supportedInterfaces)
	})
	if err := <-created; err != nil {
		c.stop()
		return nil, err
	}
	select {
	case ok := <-c.initDone:
		if !ok {
			c.Close()
			return nil, errors.New("widevine: CDM initialisation failed")
		}
	case <-time.After(10 * time.Second):
		c.Close()
		return nil, errors.New("widevine: CDM did not finish initialising")
	}
	log.Debug("widevine CDM ready", "version", c.version, "interface", c.iface, "host", c.hostIf)
	return c, nil
}

// Version is the CDM's GetCdmVersion().
func (c *CDM) Version() string { return c.version }

// Interface is the CDM interface version in use; HostInterface is the host
// interface version the CDM asked for.
func (c *CDM) Interface() int     { return c.iface }
func (c *CDM) HostInterface() int { return c.hostIf }

// TimersFired counts CDM timers this host has fired with TimerExpired.
func (c *CDM) TimersFired() uint64 { return c.fired.Load() }

// Messages delivers session messages not consumed by CreateSession, most
// importantly licence renewals. It is closed by Close.
func (c *CDM) Messages() <-chan Message { return c.messages }

// SetServerCertificate installs the licence server's service certificate.
func (c *CDM) SetServerCertificate(ctx context.Context, cert []byte) error {
	if len(cert) == 0 {
		return errors.New("widevine: empty server certificate")
	}
	_, err := c.promise(ctx, "SetServerCertificate", func(p uint32) {
		C.sona_cdm_set_server_certificate(c.ptr, C.uint32_t(p), bytesPtr(cert), C.uint32_t(len(cert)))
	})
	return err
}

// CreateSession starts a temporary session for cenc initData (a PSSH box)
// and returns its ID and the licence request to send to the server.
func (c *CDM) CreateSession(ctx context.Context, initData []byte) (id string, request []byte, err error) {
	if len(initData) == 0 {
		return "", nil, errors.New("widevine: empty init data")
	}
	var first chan Message
	id, err = c.promise(ctx, "CreateSession", func(p uint32) {
		C.sona_cdm_create_session(c.ptr, C.uint32_t(p), 0 /* kCenc */, bytesPtr(initData), C.uint32_t(len(initData)))
	})
	if err != nil {
		return "", nil, err
	}
	if id == "" {
		return "", nil, errors.New("widevine: CDM returned an empty session ID")
	}
	if err := c.do(ctx, func() { first = c.sessions[id].request }); err != nil {
		return "", nil, err
	}
	select {
	case m := <-first:
		return id, m.Data, nil
	case <-ctx.Done():
		return "", nil, ctx.Err()
	case <-c.done:
		return "", nil, ErrClosed
	case <-time.After(10 * time.Second):
		return "", nil, errors.New("widevine: no licence request for new session")
	}
}

// UpdateSession passes the licence server's response to session id. It
// fails with ErrNoUsable when the session has no usable key afterwards.
func (c *CDM) UpdateSession(ctx context.Context, id string, response []byte) error {
	if len(response) == 0 {
		return errors.New("widevine: empty licence response")
	}
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	if _, err := c.promise(ctx, "UpdateSession", func(p uint32) {
		C.sona_cdm_update_session(c.ptr, C.uint32_t(p), cid, C.uint32_t(len(id)), bytesPtr(response), C.uint32_t(len(response)))
	}); err != nil {
		return err
	}
	usable := false
	if err := c.do(ctx, func() {
		if s := c.sessions[id]; s != nil {
			for _, st := range s.keys {
				usable = usable || st == KeyUsable
			}
		}
	}); err != nil {
		return err
	}
	if !usable {
		return ErrNoUsable
	}
	return nil
}

// Keys returns the key statuses of session id, keyed by raw key ID.
func (c *CDM) Keys(id string) map[string]KeyStatus {
	out := map[string]KeyStatus{}
	_ = c.do(context.Background(), func() {
		if s := c.sessions[id]; s != nil {
			for k, v := range s.keys {
				out[k] = v
			}
		}
	})
	return out
}

// Expiry returns when session id's licence expires as the CDM reported it
// (OnExpirationChange); zero means it does not expire.
func (c *CDM) Expiry(id string) time.Time {
	var t time.Time
	_ = c.do(context.Background(), func() {
		if s := c.sessions[id]; s != nil {
			t = s.expiry
		}
	})
	return t
}

// CloseSession closes session id and drops its keys.
func (c *CDM) CloseSession(ctx context.Context, id string) error {
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	_, err := c.promise(ctx, "CloseSession", func(p uint32) {
		C.sona_cdm_close_session(c.ptr, C.uint32_t(p), cid, C.uint32_t(len(id)))
	})
	_ = c.do(ctx, func() { delete(c.sessions, id) })
	return err
}

// Subsample is a clear/encrypted run inside a sample; layout-compatible
// with cdm::SubsampleEntry.
type Subsample struct {
	Clear  uint32
	Cipher uint32
}

// Buffer is one 'cenc' (or, with CBCS, 'cbcs') sample to decrypt.
type Buffer struct {
	KeyID      []byte
	IV         []byte // 16 bytes (pad 8-byte CENC IVs with zeros)
	Subsamples []Subsample
	Data       []byte // encrypted input
	Out        []byte // receives the clear sample; len(Out) >= len(Data); may alias Data
	Timestamp  time.Duration
	// CBCS selects 'cbcs' (AES-CBC pattern) decryption with the tenc
	// pattern CryptBlocks:SkipBlocks; false is 'cenc' (AES-CTR).
	CBCS        bool
	CryptBlocks uint8
	SkipBlocks  uint8
	N           int // set by Decrypt: bytes written to Out
}

// Decrypt decrypts bufs in order with a single hop to the CDM thread. It
// stops at the first failure (ErrNoKey, ErrDecrypt, …).
func (c *CDM) Decrypt(bufs []Buffer) error {
	var err error
	if derr := c.do(context.Background(), func() {
		for i := range bufs {
			if err = c.decrypt(&bufs[i]); err != nil {
				err = fmt.Errorf("sample %d: %w", i, err)
				return
			}
		}
	}); derr != nil {
		return derr
	}
	return err
}

func (c *CDM) decrypt(b *Buffer) error {
	if len(b.Data) == 0 || len(b.Out) < len(b.Data) || len(b.KeyID) == 0 || len(b.IV) == 0 {
		return errors.New("widevine: bad decrypt buffer")
	}
	var subs *C.uint32_t
	if len(b.Subsamples) > 0 {
		subs = (*C.uint32_t)(unsafe.Pointer(&b.Subsamples[0]))
	}
	var n C.uint32_t
	var cbcs C.int
	if b.CBCS {
		cbcs = 1
	}
	st := C.sona_cdm_decrypt(c.ptr, bytesPtr(b.KeyID), C.uint32_t(len(b.KeyID)), bytesPtr(b.IV), C.uint32_t(len(b.IV)),
		bytesPtr(b.Data), C.uint32_t(len(b.Data)), subs, C.uint32_t(len(b.Subsamples)), C.int64_t(b.Timestamp.Microseconds()),
		cbcs, C.uint32_t(b.CryptBlocks), C.uint32_t(b.SkipBlocks),
		(*C.uint8_t)(unsafe.Pointer(&b.Out[0])), C.uint32_t(len(b.Out)), &n)
	switch st {
	case 0:
		b.N = int(n)
		return nil
	case 2:
		return ErrNoKey
	case 4:
		return ErrDecrypt
	default:
		return fmt.Errorf("%w: status %d", ErrDecrypt, int(st))
	}
}

// Close destroys the instance. The library stays loaded for the process.
func (c *CDM) Close() error {
	closed := make(chan struct{})
	select {
	case c.calls <- func() {
		for t := range c.timers {
			t.Stop()
		}
		c.timers = nil
		if c.ptr != nil {
			C.sona_cdm_destroy(c.ptr)
			c.ptr = nil
		}
		close(closed)
	}:
		<-closed
	case <-c.done:
	}
	c.stop()
	return nil
}

func (c *CDM) stop() {
	c.once.Do(func() {
		close(c.done)
	})
}

// loop runs every CDM call on one OS thread: the Chromium CDM API is single
// threaded, and host callbacks must not re-enter the CDM.
func (c *CDM) loop() {
	runtime.LockOSThread()
	defer func() {
		instances.Delete(c.handle)
		close(c.messages)
	}()
	for {
		select {
		case fn := <-c.calls:
			fn()
		case <-c.wake:
		case <-c.done:
			return
		}
		c.drain()
	}
}

// post queues fn for the CDM thread without waiting for it.
func (c *CDM) post(fn func()) bool {
	select {
	case c.calls <- func() {
		if c.ptr != nil || c.iface == 0 {
			fn()
		}
	}:
		return true
	case <-c.done:
		return false
	}
}

// do runs fn on the CDM thread and waits for it.
func (c *CDM) do(ctx context.Context, fn func()) error {
	ran := make(chan struct{})
	if !c.post(func() { fn(); close(ran) }) {
		return ErrClosed
	}
	select {
	case <-ran:
		return nil
	case <-c.done:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// promise runs call with a fresh promise ID and waits for its resolution.
func (c *CDM) promise(ctx context.Context, op string, call func(uint32)) (string, error) {
	res := make(chan promiseResult, 1)
	if err := c.do(ctx, func() {
		c.nextPromise++
		p := c.nextPromise
		c.promises[p] = res
		call(p)
	}); err != nil {
		return "", err
	}
	select {
	case r := <-res:
		if pe, ok := r.err.(*PromiseError); ok {
			pe.Op = op
		}
		return r.session, r.err
	case <-ctx.Done():
		return "", ctx.Err()
	case <-c.done:
		return "", ErrClosed
	case <-time.After(30 * time.Second):
		return "", fmt.Errorf("widevine: %s: no answer from CDM", op)
	}
}

//export sonaCdmEvent
func sonaCdmEvent(h C.uintptr_t, kind C.int, a, b, cc C.uint32_t, t C.double, ptr C.uintptr_t, sid *C.char, sidLen C.uint32_t, data *C.char, dataLen C.uint32_t) {
	v, ok := instances.Load(uintptr(h))
	if !ok {
		return
	}
	c := v.(*CDM)
	ev := event{kind: int(kind), a: uint32(a), b: uint32(b), c: uint32(cc), t: float64(t), ptr: uintptr(ptr)}
	if sid != nil && sidLen > 0 {
		ev.sid = C.GoStringN(sid, C.int(sidLen))
	}
	if data != nil && dataLen > 0 {
		ev.data = C.GoBytes(unsafe.Pointer(data), C.int(dataLen))
	}
	c.evMu.Lock()
	c.evq = append(c.evq, ev)
	c.evMu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// drain handles queued host callbacks on the CDM thread, then answers the
// host requests they made (never from inside a callback).
func (c *CDM) drain() {
	for {
		c.evMu.Lock()
		q := c.evq
		c.evq = nil
		c.evMu.Unlock()
		for _, ev := range q {
			c.handle1(ev)
		}
		d := c.deferred
		c.deferred = nil
		for _, fn := range d {
			if c.ptr != nil {
				fn()
			}
		}
		if len(q) == 0 && len(d) == 0 {
			return
		}
	}
}

// instances maps the handle passed to the shim to its CDM; events for a
// destroyed instance find nothing and are dropped.
var (
	instances  sync.Map
	nextHandle atomic.Uintptr
)

func (c *CDM) resolve(p uint32, r promiseResult) {
	if ch, ok := c.promises[p]; ok {
		delete(c.promises, p)
		ch <- r
	}
}

func (c *CDM) sess(id string) *session {
	s := c.sessions[id]
	if s == nil {
		s = &session{request: make(chan Message, 1), keys: map[string]KeyStatus{}}
		c.sessions[id] = s
	}
	return s
}

func (c *CDM) handle1(ev event) {
	switch ev.kind {
	case C.SONA_EV_INITIALIZED:
		select {
		case c.initDone <- ev.a != 0:
		default:
		}
	case C.SONA_EV_PROMISE_RESOLVED, C.SONA_EV_KEY_STATUS_PROMISE:
		c.resolve(ev.a, promiseResult{})
	case C.SONA_EV_NEW_SESSION:
		if ev.sid != "" {
			c.sess(ev.sid)
		}
		c.resolve(ev.a, promiseResult{session: ev.sid})
	case C.SONA_EV_PROMISE_REJECTED:
		c.resolve(ev.a, promiseResult{err: &PromiseError{Exception: int(ev.b), SystemCode: ev.c, Message: string(ev.data)}})
	case C.SONA_EV_MESSAGE:
		m := Message{SessionID: ev.sid, Type: MessageType(ev.a), Data: ev.data}
		s := c.sess(ev.sid)
		if m.Type == LicenseRequest && !s.gotRequest {
			s.gotRequest = true
			s.request <- m
			return
		}
		select {
		case c.messages <- m:
		default:
			c.log.Warn("widevine: dropped CDM message, receiver too slow", "type", m.Type, "session", m.SessionID)
		}
	case C.SONA_EV_KEY_STATUS:
		c.sess(ev.sid).keys[string(ev.data)] = KeyStatus(ev.a)
	case C.SONA_EV_KEYS_CHANGED:
	case C.SONA_EV_EXPIRATION:
		s := c.sess(ev.sid)
		s.expiry = time.Time{}
		if ev.t > 0 {
			s.expiry = time.Unix(0, int64(ev.t*float64(time.Second)))
		}
	case C.SONA_EV_SESSION_CLOSED:
		delete(c.sessions, ev.sid)
	case C.SONA_EV_TIMER:
		ctxPtr := ev.ptr
		var t *time.Timer
		t = time.AfterFunc(time.Duration(ev.t*float64(time.Millisecond)), func() {
			c.post(func() {
				if c.timers != nil {
					delete(c.timers, t)
				}
				if c.ptr != nil {
					C.sona_cdm_timer_expired(c.ptr, C.uintptr_t(ctxPtr))
					c.fired.Add(1)
				}
			})
		})
		if c.timers != nil {
			c.timers[t] = struct{}{}
		}
	case C.SONA_EV_QUERY_OUTPUT_PROTECTION:
		c.deferred = append(c.deferred, func() { C.sona_cdm_output_protection_status(c.ptr) })
	case C.SONA_EV_STORAGE_ID:
		v := ev.a
		c.deferred = append(c.deferred, func() { C.sona_cdm_storage_id(c.ptr, C.uint32_t(v)) })
	case C.SONA_EV_PLATFORM_CHALLENGE:
		c.deferred = append(c.deferred, func() { C.sona_cdm_platform_challenge_failed(c.ptr) })
	}
}

func bytesPtr(b []byte) *C.uint8_t { return (*C.uint8_t)(unsafe.Pointer(&b[0])) }
