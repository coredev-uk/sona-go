package store

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/zalando/go-keyring"
)

// SystemBackend is where SystemKeyring keeps secrets on this OS: the login Keychain.
const SystemBackend = BackendKeychain

// SystemKeyring keeps secrets in the login Keychain as generic passwords
// (service, account), through Security.framework's SecItem calls reached
// with purego.
//
// Sona creates each item itself, so the item's access list lets only Sona's
// executable read it without asking: another program that reads it makes
// macOS ask the user first. go-keyring ran /usr/bin/security instead, whose
// items trust /usr/bin/security, so any program of the user's could read
// them silently by running it (GHSA-49cj-524r-6gp3). Sona's items carry the
// creator code sonaCreator; an item for the same service and account
// without it is go-keyring's, and Get, Set and Delete move it out of the
// way through /usr/bin/security, which owns it (moveGoKeyring).
//
// macOS ties the trust to the creating executable's code signature, and an
// ad-hoc signature (every Sona build) changes with each update. So the
// first read after an update asks the user (Always Allow adds the new build
// to the item). Without that the updated build cannot delete the item:
// Delete then overwrites the secret with a tombstone, which any program may
// do, and Get reports a tombstone as no item.
type SystemKeyring struct {
	// keychain (a SecKeychainRef) and keychainPath name a keychain to use
	// instead of the default keychain and search list; tests point them at a
	// throwaway keychain.
	keychain     uintptr
	keychainPath string
}

// sonaCreator is the creator code ('Sona') on every item Sona creates.
const sonaCreator = 'S'<<24 | 'o'<<16 | 'n'<<8 | 'a'

// tombstone replaces the secret of an item Delete may not delete. It is not
// empty because SecItemUpdate ignores empty data.
const tombstone = "\x00sona:deleted"

// Get returns the secret, or keyring.ErrNotFound when there is none (or
// only an empty item or a tombstone). A go-keyring item is moved into one
// Sona creates first.
func (k SystemKeyring) Get(service, user string) (string, error) {
	if err := secLoad(); err != nil {
		return "", err
	}
	v, err := k.read(service, user)
	switch {
	case err == nil && (v == "" || v == tombstone):
		return "", keyring.ErrNotFound
	case !errors.Is(err, keyring.ErrNotFound):
		return v, err
	}
	if err := k.exists(service, user); err != nil {
		return "", err
	}
	return k.moveGoKeyring(service, user)
}

// Set stores the secret: in place in Sona's item (an update keeps the
// item's access list), else in a new item, after moving a go-keyring item
// out of the way.
func (k SystemKeyring) Set(service, user, password string) error {
	if err := secLoad(); err != nil {
		return err
	}
	err := k.update(service, user, password)
	if !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	if err := k.exists(service, user); err == nil {
		if err := k.goKeyringDelete(service, user); err != nil {
			return err
		}
	} else if !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return k.add(service, user, password)
}

// Delete removes the secret: keyring.ErrNotFound when there is none. An item
// an earlier build created that this build may not delete (it was allowed
// to read it once, not always) gets the secret tombstone instead.
func (k SystemKeyring) Delete(service, user string) error {
	if err := secLoad(); err != nil {
		return err
	}
	q := k.query(service, user, true, true)
	defer q.release()
	err := secStatus("delete", sec.itemDelete(q.dict()))
	var st *osStatusError
	switch {
	case errors.As(err, &st) && st.status == errSecInvalidOwnerEdit:
		if err := k.update(service, user, tombstone); err != nil {
			return fmt.Errorf("keychain: overwrite an item an earlier build of Sona created: %w", err)
		}
		return nil
	case !errors.Is(err, keyring.ErrNotFound):
		return err
	}
	if err := k.exists(service, user); err != nil {
		return err
	}
	return k.goKeyringDelete(service, user)
}

// moveGoKeyring reads the go-keyring item for (service, user), deletes it
// and stores its secret in an item Sona creates. Both steps go through
// /usr/bin/security, the item's owner, so neither asks the user. If the
// delete fails the item stays and the next Get tries again; the secret is
// returned either way.
func (k SystemKeyring) moveGoKeyring(service, user string) (string, error) {
	v, err := k.goKeyringGet(service, user)
	if err != nil {
		return "", err
	}
	if v == "" {
		return "", keyring.ErrNotFound
	}
	if k.goKeyringDelete(service, user) == nil {
		_ = k.add(service, user, v)
	}
	return v, nil
}

// goKeyringGet reads a go-keyring item as go-keyring did and decodes it.
func (k SystemKeyring) goKeyringGet(service, user string) (string, error) {
	out, err := k.security("find-generic-password", "-s", service, "-a", user, "-w")
	if err != nil {
		return "", err
	}
	v := strings.TrimSuffix(out, "\n")
	switch {
	case strings.HasPrefix(v, goKeyringBase64Prefix):
		b, err := base64.StdEncoding.DecodeString(v[len(goKeyringBase64Prefix):])
		if err != nil {
			return "", fmt.Errorf("keychain: decode a go-keyring item: %w", err)
		}
		return string(b), nil
	case strings.HasPrefix(v, goKeyringHexPrefix):
		b, err := hex.DecodeString(v[len(goKeyringHexPrefix):])
		if err != nil {
			return "", fmt.Errorf("keychain: decode a go-keyring item: %w", err)
		}
		return string(b), nil
	}
	return v, nil
}

func (k SystemKeyring) goKeyringDelete(service, user string) error {
	_, err := k.security("delete-generic-password", "-s", service, "-a", user)
	return err
}

// go-keyring's value prefixes (keyring_darwin.go in v0.2.8): it stored every
// secret as base64Prefix + base64, and earlier versions as hexPrefix + hex.
const (
	goKeyringBase64Prefix = "go-keyring-base64:"
	goKeyringHexPrefix    = "go-keyring-encoded:"
)

// security runs /usr/bin/security on k's keychain. "could not be found" is
// keyring.ErrNotFound, as go-keyring had it.
func (k SystemKeyring) security(args ...string) (string, error) {
	if k.keychainPath != "" {
		args = append(args, k.keychainPath)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/security", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if strings.Contains(stderr.String(), "could not be found") {
			return "", keyring.ErrNotFound
		}
		return "", fmt.Errorf("keychain: security %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// read returns the data of Sona's item.
func (k SystemKeyring) read(service, user string) (string, error) {
	q := k.query(service, user, true, true)
	q.set(sec.kSecReturnData, sec.kCFBooleanTrue)
	q.set(sec.kSecMatchLimit, sec.kSecMatchLimitOne)
	defer q.release()
	var data uintptr
	if err := secStatus("read", sec.itemCopyMatching(q.dict(), &data)); err != nil {
		return "", err
	}
	defer sec.release(data)
	n := sec.dataGetLength(data)
	if n == 0 {
		return "", nil
	}
	return string(unsafe.Slice((*byte)(sec.dataGetBytePtr(data)), n)), nil
}

// exists reports whether there is an item for (service, user), Sona's or
// not: nil or keyring.ErrNotFound. It asks for no data, so it never prompts.
// A keychain holds one item per service and account, so when read found no
// item of Sona's this is go-keyring's.
func (k SystemKeyring) exists(service, user string) error {
	q := k.query(service, user, false, true)
	defer q.release()
	return secStatus("look up", sec.itemCopyMatching(q.dict(), nil))
}

func (k SystemKeyring) add(service, user, password string) error {
	q := k.query(service, user, true, false)
	defer q.release()
	q.own(sec.kSecValueData, cfData(password))
	return secStatus("write", sec.itemAdd(q.dict(), nil))
}

// update replaces the data of Sona's item.
func (k SystemKeyring) update(service, user, password string) error {
	q := k.query(service, user, true, true)
	defer q.release()
	attrs := &cfDict{}
	defer attrs.release()
	attrs.own(sec.kSecValueData, cfData(password))
	return secStatus("update", sec.itemUpdate(q.dict(), attrs.dict()))
}

// query starts a generic-password dictionary for (service, user). sona
// limits it to Sona's items (and marks an added one). search names k's
// keychain as the search list (reads, updates, deletes) rather than the
// keychain to add to.
func (k SystemKeyring) query(service, user string, sona, search bool) *cfDict {
	q := &cfDict{}
	q.set(sec.kSecClass, sec.kSecClassGenericPassword)
	q.own(sec.kSecAttrService, cfString(service))
	q.own(sec.kSecAttrAccount, cfString(user))
	if sona {
		c := int32(sonaCreator)
		q.own(sec.kSecAttrCreator, sec.numberCreate(0, cfNumberSInt32Type, unsafe.Pointer(&c)))
	}
	if k.keychain != 0 {
		if search {
			list := []uintptr{k.keychain}
			q.own(sec.kSecMatchSearchList, sec.arrayCreate(0, &list[0], 1, sec.arrayCallBacks))
		} else {
			q.set(sec.kSecUseKeychain, k.keychain)
		}
	}
	return q
}

// OSStatus values from Security/SecBase.h.
const (
	errSecItemNotFound     = -25300
	errSecInvalidOwnerEdit = -25244
)

// osStatusError is a failed SecItem call.
type osStatusError struct {
	op     string
	status int32
	msg    string
}

func (e *osStatusError) Error() string {
	return fmt.Sprintf("keychain %s: %s (OSStatus %d)", e.op, e.msg, e.status)
}

func secStatus(op string, status int32) error {
	switch status {
	case 0:
		return nil
	case errSecItemNotFound:
		return keyring.ErrNotFound
	}
	msg := "unknown error"
	if s := sec.copyErrorMessageString(status, 0); s != 0 {
		msg = goStringOf(s)
		sec.release(s)
	}
	return &osStatusError{op: op, status: status, msg: msg}
}

// cfDict collects a CFDictionary's keys and values; values it owns are
// released with it.
type cfDict struct {
	keys, values, owned []uintptr
	d                   uintptr
}

func (q *cfDict) set(key, value uintptr) {
	q.keys = append(q.keys, key)
	q.values = append(q.values, value)
}

// own sets a value the dictionary must release (a CF object this file created).
func (q *cfDict) own(key, value uintptr) {
	q.set(key, value)
	q.owned = append(q.owned, value)
}

// dict builds the CFDictionary (once), which retains its keys and values.
func (q *cfDict) dict() uintptr {
	if q.d == 0 {
		q.d = sec.dictionaryCreate(0, &q.keys[0], &q.values[0], len(q.keys), sec.dictKeyCallBacks, sec.dictValueCallBacks)
	}
	return q.d
}

func (q *cfDict) release() {
	if q.d != 0 {
		sec.release(q.d)
	}
	for _, v := range q.owned {
		if v != 0 {
			sec.release(v)
		}
	}
}

const (
	cfStringEncodingUTF8 = 0x08000100
	cfNumberSInt32Type   = 3
)

// cfString is a new CFString of s; the caller releases it.
func cfString(s string) uintptr {
	r := sec.stringCreateWithBytes(0, unsafe.StringData(s), len(s), cfStringEncodingUTF8, false)
	runtime.KeepAlive(s)
	return r
}

// cfData is a new CFData holding a copy of s; the caller releases it.
func cfData(s string) uintptr {
	r := sec.dataCreate(0, unsafe.StringData(s), len(s))
	runtime.KeepAlive(s)
	return r
}

// goStringOf copies a CFString into a Go string.
func goStringOf(s uintptr) string {
	buf := make([]byte, 4*sec.stringGetLength(s)+1)
	if !sec.stringGetCString(s, &buf[0], len(buf), cfStringEncodingUTF8) {
		return ""
	}
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return string(buf[:n])
}

// sec holds Core Foundation and Security.framework, loaded on first use.
var sec struct {
	once sync.Once
	err  error

	release               func(cf uintptr)
	stringCreateWithBytes func(alloc uintptr, b *byte, n int, enc uint32, external bool) uintptr
	stringGetLength       func(s uintptr) int
	stringGetCString      func(s uintptr, buf *byte, size int, enc uint32) bool
	dataCreate            func(alloc uintptr, b *byte, n int) uintptr
	dataGetBytePtr        func(d uintptr) unsafe.Pointer
	dataGetLength         func(d uintptr) int
	numberCreate          func(alloc uintptr, typ int, v unsafe.Pointer) uintptr
	dictionaryCreate      func(alloc uintptr, keys, values *uintptr, n int, keyCB, valueCB uintptr) uintptr
	arrayCreate           func(alloc uintptr, values *uintptr, n int, cb uintptr) uintptr

	itemAdd                func(attrs uintptr, result *uintptr) int32
	itemCopyMatching       func(query uintptr, result *uintptr) int32
	itemUpdate             func(query, attrs uintptr) int32
	itemDelete             func(query uintptr) int32
	copyErrorMessageString func(status int32, reserved uintptr) uintptr

	// Addresses of the callback structs.
	dictKeyCallBacks, dictValueCallBacks, arrayCallBacks uintptr

	// Values of the constant CF objects.
	kCFBooleanTrue, kSecClass, kSecClassGenericPassword, kSecAttrService,
	kSecAttrAccount, kSecAttrCreator, kSecValueData, kSecReturnData,
	kSecMatchLimit, kSecMatchLimitOne, kSecUseKeychain, kSecMatchSearchList uintptr
}

func secLoad() error {
	sec.once.Do(func() {
		defer func() {
			if r := recover(); r != nil { // RegisterLibFunc panics on a missing symbol
				sec.err = fmt.Errorf("keychain: load Security.framework: %v", r)
			}
		}()
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			sec.err = fmt.Errorf("keychain: load CoreFoundation: %w", err)
			return
		}
		s, err := purego.Dlopen("/System/Library/Frameworks/Security.framework/Security", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			sec.err = fmt.Errorf("keychain: load Security.framework: %w", err)
			return
		}
		purego.RegisterLibFunc(&sec.release, cf, "CFRelease")
		purego.RegisterLibFunc(&sec.stringCreateWithBytes, cf, "CFStringCreateWithBytes")
		purego.RegisterLibFunc(&sec.stringGetLength, cf, "CFStringGetLength")
		purego.RegisterLibFunc(&sec.stringGetCString, cf, "CFStringGetCString")
		purego.RegisterLibFunc(&sec.dataCreate, cf, "CFDataCreate")
		purego.RegisterLibFunc(&sec.dataGetBytePtr, cf, "CFDataGetBytePtr")
		purego.RegisterLibFunc(&sec.dataGetLength, cf, "CFDataGetLength")
		purego.RegisterLibFunc(&sec.numberCreate, cf, "CFNumberCreate")
		purego.RegisterLibFunc(&sec.dictionaryCreate, cf, "CFDictionaryCreate")
		purego.RegisterLibFunc(&sec.arrayCreate, cf, "CFArrayCreate")
		purego.RegisterLibFunc(&sec.itemAdd, s, "SecItemAdd")
		purego.RegisterLibFunc(&sec.itemCopyMatching, s, "SecItemCopyMatching")
		purego.RegisterLibFunc(&sec.itemUpdate, s, "SecItemUpdate")
		purego.RegisterLibFunc(&sec.itemDelete, s, "SecItemDelete")
		purego.RegisterLibFunc(&sec.copyErrorMessageString, s, "SecCopyErrorMessageString")

		for _, a := range []struct {
			lib  uintptr
			name string
			dst  *uintptr
		}{
			{cf, "kCFTypeDictionaryKeyCallBacks", &sec.dictKeyCallBacks},
			{cf, "kCFTypeDictionaryValueCallBacks", &sec.dictValueCallBacks},
			{cf, "kCFTypeArrayCallBacks", &sec.arrayCallBacks},
		} {
			if *a.dst, err = purego.Dlsym(a.lib, a.name); err != nil {
				sec.err = fmt.Errorf("keychain: %s: %w", a.name, err)
				return
			}
		}
		for _, c := range []struct {
			lib  uintptr
			name string
			dst  *uintptr
		}{
			{cf, "kCFBooleanTrue", &sec.kCFBooleanTrue},
			{s, "kSecClass", &sec.kSecClass},
			{s, "kSecClassGenericPassword", &sec.kSecClassGenericPassword},
			{s, "kSecAttrService", &sec.kSecAttrService},
			{s, "kSecAttrAccount", &sec.kSecAttrAccount},
			{s, "kSecAttrCreator", &sec.kSecAttrCreator},
			{s, "kSecValueData", &sec.kSecValueData},
			{s, "kSecReturnData", &sec.kSecReturnData},
			{s, "kSecMatchLimit", &sec.kSecMatchLimit},
			{s, "kSecMatchLimitOne", &sec.kSecMatchLimitOne},
			{s, "kSecUseKeychain", &sec.kSecUseKeychain},
			{s, "kSecMatchSearchList", &sec.kSecMatchSearchList},
		} {
			addr, err := purego.Dlsym(c.lib, c.name)
			if err != nil {
				sec.err = fmt.Errorf("keychain: %s: %w", c.name, err)
				return
			}
			// addr is the address of the constant, which points to the object.
			*c.dst = **(**uintptr)(unsafe.Pointer(&addr))
		}
	})
	return sec.err
}
