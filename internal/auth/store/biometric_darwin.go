//go:build darwin

package store

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// The biometric Keychain item, through Security.framework and
// CoreFoundation loaded with purego (no cgo, no Objective-C).
//
// The item is a generic password in the data protection Keychain
// (kSecUseDataProtectionKeychain), the only Keychain that honours biometric
// access controls, guarded by SecAccessControl with .biometryCurrentSet: a
// change to the enrolled fingerprints invalidates it. Reading it makes macOS
// show the Touch ID prompt, with the reason from kSecUseOperationPrompt
// (the LAContext-free spelling, which macOS still honours).
//
// The data protection Keychain needs an application identifier, which only a
// signed bundle with a team ID and the keychain-access-groups entitlement has
// (build/darwin/bundle.sh, docs/auth.md §7.1); Available reads the running
// code signature for both, so an ad-hoc build reports unsupported and never
// calls SecItem*.

const (
	biometricAccount = "mut-touch-id"

	securityPath       = "/System/Library/Frameworks/Security.framework/Security"
	coreFoundationPath = "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"

	cfStringEncodingUTF8        = 0x08000100
	secAccessControlBiometryCur = 1 << 3 // kSecAccessControlBiometryCurrentSet
	secCSSigningInformation     = 2      // kSecCSSigningInformation

	errSecSuccess            = 0
	errSecUserCanceled       = -128
	errSecItemNotFound       = -25300
	errSecAuthFailed         = -25293
	errSecInteractionBlocked = -25308
	errSecMissingEntitlement = -34018

	keychainAccessGroups = "keychain-access-groups"
)

// cf is the loaded frameworks: the functions and constants the item needs.
type cf struct {
	stringCreate  func(alloc uintptr, s string, enc uint32) uintptr
	stringCString func(s uintptr, buf *byte, size int, enc uint32) uint8
	stringLength  func(s uintptr) int
	dataCreate    func(alloc uintptr, b *byte, n int) uintptr
	dataBytes     func(d uintptr) uintptr
	dataLength    func(d uintptr) int
	dictCreate    func(alloc uintptr, capacity int, keyCB, valCB uintptr) uintptr
	dictSet       func(d, k, v uintptr)
	dictGet       func(d, k uintptr) uintptr
	release       func(o uintptr)

	itemAdd    func(attrs uintptr, result *uintptr) int32
	itemCopy   func(query uintptr, result *uintptr) int32
	itemDelete func(query uintptr) int32
	accessCtl  func(alloc, protection uintptr, flags uint, err *uintptr) uintptr
	codeSelf   func(flags uint32, out *uintptr) int32
	codeInfo   func(code uintptr, flags uint32, info *uintptr) int32

	keyCB, valCB uintptr // &kCFTypeDictionary{Key,Value}CallBacks

	kTrue, class, classGeneric, service, account, valueData, returnData,
	accessControl, dataProtection, prompt, matchLimit, matchLimitOne,
	protection, codeTeamID, codeEntitlements uintptr
}

var (
	loadOnce sync.Once
	loaded   *cf
	loadErr  error
)

func loadFrameworks() (*cf, error) {
	loadOnce.Do(func() { loaded, loadErr = openFrameworks() })
	return loaded, loadErr
}

func openFrameworks() (c *cf, err error) {
	defer func() {
		// purego panics on a missing symbol.
		if r := recover(); r != nil {
			c, err = nil, fmt.Errorf("store: Security framework: %v", r)
		}
	}()
	sec, err := purego.Dlopen(securityPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("store: load Security.framework: %w", err)
	}
	core, err := purego.Dlopen(coreFoundationPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("store: load CoreFoundation.framework: %w", err)
	}
	c = &cf{}
	purego.RegisterLibFunc(&c.stringCreate, core, "CFStringCreateWithCString")
	purego.RegisterLibFunc(&c.stringCString, core, "CFStringGetCString")
	purego.RegisterLibFunc(&c.stringLength, core, "CFStringGetLength")
	purego.RegisterLibFunc(&c.dataCreate, core, "CFDataCreate")
	purego.RegisterLibFunc(&c.dataBytes, core, "CFDataGetBytePtr")
	purego.RegisterLibFunc(&c.dataLength, core, "CFDataGetLength")
	purego.RegisterLibFunc(&c.dictCreate, core, "CFDictionaryCreateMutable")
	purego.RegisterLibFunc(&c.dictSet, core, "CFDictionarySetValue")
	purego.RegisterLibFunc(&c.dictGet, core, "CFDictionaryGetValue")
	purego.RegisterLibFunc(&c.release, core, "CFRelease")
	purego.RegisterLibFunc(&c.itemAdd, sec, "SecItemAdd")
	purego.RegisterLibFunc(&c.itemCopy, sec, "SecItemCopyMatching")
	purego.RegisterLibFunc(&c.itemDelete, sec, "SecItemDelete")
	purego.RegisterLibFunc(&c.accessCtl, sec, "SecAccessControlCreateWithFlags")
	purego.RegisterLibFunc(&c.codeSelf, sec, "SecCodeCopySelf")
	purego.RegisterLibFunc(&c.codeInfo, sec, "SecCodeCopySigningInformation")

	// Exported data symbols: the address is the variable's, whose value is
	// the CFStringRef (or the callbacks struct itself).
	val := func(h uintptr, name string) uintptr {
		addr, err := purego.Dlsym(h, name)
		if err != nil {
			panic(err)
		}
		return **(**uintptr)(unsafe.Pointer(&addr))
	}
	addr := func(h uintptr, name string) uintptr {
		a, err := purego.Dlsym(h, name)
		if err != nil {
			panic(err)
		}
		return a
	}
	c.keyCB = addr(core, "kCFTypeDictionaryKeyCallBacks")
	c.valCB = addr(core, "kCFTypeDictionaryValueCallBacks")
	c.kTrue = val(core, "kCFBooleanTrue")
	c.class = val(sec, "kSecClass")
	c.classGeneric = val(sec, "kSecClassGenericPassword")
	c.service = val(sec, "kSecAttrService")
	c.account = val(sec, "kSecAttrAccount")
	c.valueData = val(sec, "kSecValueData")
	c.returnData = val(sec, "kSecReturnData")
	c.accessControl = val(sec, "kSecAttrAccessControl")
	c.dataProtection = val(sec, "kSecUseDataProtectionKeychain")
	c.prompt = val(sec, "kSecUseOperationPrompt")
	c.matchLimit = val(sec, "kSecMatchLimit")
	c.matchLimitOne = val(sec, "kSecMatchLimitOne")
	c.protection = val(sec, "kSecAttrAccessibleWhenPasscodeSetThisDeviceOnly")
	c.codeTeamID = val(sec, "kSecCodeInfoTeamIdentifier")
	c.codeEntitlements = val(sec, "kSecCodeInfoEntitlementsDict")
	return c, nil
}

// SystemBiometric is the Keychain item on macOS. prompt is the reason the
// Touch ID prompt shows, in the user's language when it is asked.
func SystemBiometric(prompt func() string) BiometricKeychain {
	return &systemBiometric{prompt: prompt}
}

type systemBiometric struct {
	prompt func() string

	availOnce sync.Once
	avail     error

	mu sync.Mutex // one SecItem call at a time
}

// Available reads the running code signature: a team ID and the
// keychain-access-groups entitlement, which an ad-hoc signature has neither of.
func (s *systemBiometric) Available() error {
	s.availOnce.Do(func() { s.avail = checkSignature() })
	return s.avail
}

func checkSignature() error {
	c, err := loadFrameworks()
	if err != nil {
		return fmt.Errorf("%w: %w", errors.ErrUnsupported, err)
	}
	var code, info uintptr
	if st := c.codeSelf(0, &code); st != errSecSuccess {
		return fmt.Errorf("store: Touch ID needs a signed Sona.app (code signature unreadable, OSStatus %d): %w", st, errors.ErrUnsupported)
	}
	defer c.release(code)
	if st := c.codeInfo(code, secCSSigningInformation, &info); st != errSecSuccess {
		return fmt.Errorf("store: Touch ID needs a signed Sona.app (OSStatus %d): %w", st, errors.ErrUnsupported)
	}
	defer c.release(info)
	if c.dictGet(info, c.codeTeamID) == 0 {
		return fmt.Errorf("store: Touch ID needs a Sona build signed with a Developer ID (this one has no team ID, as an ad-hoc signature has none): %w", errors.ErrUnsupported)
	}
	ents := c.dictGet(info, c.codeEntitlements)
	if ents == 0 {
		return fmt.Errorf("store: Touch ID needs the %s entitlement, which this build isn't signed with: %w", keychainAccessGroups, errors.ErrUnsupported)
	}
	key := c.stringCreate(0, keychainAccessGroups, cfStringEncodingUTF8)
	defer c.release(key)
	if c.dictGet(ents, key) == 0 {
		return fmt.Errorf("store: Touch ID needs the %s entitlement, which this build isn't signed with: %w", keychainAccessGroups, errors.ErrUnsupported)
	}
	return nil
}

// query builds the item's identity (class, service, account, data
// protection Keychain) as a mutable dictionary and the strings it owns.
func (s *systemBiometric) query(c *cf) (q uintptr, free func()) {
	var owned []uintptr
	str := func(v string) uintptr {
		o := c.stringCreate(0, v, cfStringEncodingUTF8)
		owned = append(owned, o)
		return o
	}
	q = c.dictCreate(0, 0, c.keyCB, c.valCB)
	owned = append(owned, q)
	c.dictSet(q, c.class, c.classGeneric)
	c.dictSet(q, c.service, str(Service))
	c.dictSet(q, c.account, str(biometricAccount))
	c.dictSet(q, c.dataProtection, c.kTrue)
	return q, func() {
		for _, o := range owned {
			c.release(o)
		}
	}
}

// Get shows the Touch ID prompt and returns the item.
func (s *systemBiometric) Get() ([]byte, error) {
	if err := s.Available(); err != nil {
		return nil, err
	}
	c, _ := loadFrameworks()
	s.mu.Lock()
	defer s.mu.Unlock()
	q, free := s.query(c)
	defer free()
	reason := "Unlock Sona"
	if s.prompt != nil {
		reason = s.prompt()
	}
	p := c.stringCreate(0, reason, cfStringEncodingUTF8)
	defer c.release(p)
	c.dictSet(q, c.prompt, p)
	c.dictSet(q, c.returnData, c.kTrue)
	c.dictSet(q, c.matchLimit, c.matchLimitOne)
	var res uintptr
	if st := c.itemCopy(q, &res); st != errSecSuccess {
		return nil, mapStatus("read", st)
	}
	defer c.release(res)
	n := c.dataLength(res)
	if n == 0 {
		return nil, fmt.Errorf("%w: empty item", ErrCorrupt)
	}
	// Copy out of CoreFoundation's buffer before res is released.
	ptr := c.dataBytes(res)
	out := make([]byte, n)
	copy(out, unsafe.Slice(*(**byte)(unsafe.Pointer(&ptr)), n))
	return out, nil
}

// Set replaces the item: the old one is deleted first (an item whose access
// control was invalidated can't be updated in place), then added with
// .biometryCurrentSet.
func (s *systemBiometric) Set(b []byte) error {
	if err := s.Available(); err != nil {
		return err
	}
	if len(b) == 0 {
		return errors.New("store: empty secret")
	}
	c, _ := loadFrameworks()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.deleteLocked(c); err != nil {
		return err
	}
	q, free := s.query(c)
	defer free()
	var cerr uintptr
	ac := c.accessCtl(0, c.protection, secAccessControlBiometryCur, &cerr)
	if ac == 0 {
		if cerr != 0 {
			c.release(cerr)
		}
		return fmt.Errorf("store: Touch ID access control: %w", errors.ErrUnsupported)
	}
	defer c.release(ac)
	data := c.dataCreate(0, &b[0], len(b))
	defer c.release(data)
	c.dictSet(q, c.accessControl, ac)
	c.dictSet(q, c.valueData, data)
	if st := c.itemAdd(q, nil); st != errSecSuccess {
		return mapStatus("write", st)
	}
	return nil
}

// Delete removes the item; macOS asks nothing to delete it.
func (s *systemBiometric) Delete() error {
	if err := s.Available(); err != nil {
		// Without the entitlement there can be no item to remove.
		return nil
	}
	c, _ := loadFrameworks()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deleteLocked(c)
}

func (s *systemBiometric) deleteLocked(c *cf) error {
	q, free := s.query(c)
	defer free()
	if st := c.itemDelete(q); st != errSecSuccess && st != errSecItemNotFound {
		return mapStatus("delete", st)
	}
	return nil
}

// mapStatus is an OSStatus as the package's errors. A fingerprint change
// makes the item read as not found or as an authentication failure, which
// is indistinguishable from a wrong finger: the former is ErrNotFound (the
// manager, knowing Touch ID is on, takes it as invalidated), the latter
// ErrBiometricDenied (asking again can succeed).
func mapStatus(op string, st int32) error {
	switch st {
	case errSecItemNotFound:
		return ErrNotFound
	case errSecUserCanceled, errSecAuthFailed, errSecInteractionBlocked:
		return fmt.Errorf("%w (OSStatus %d)", ErrBiometricDenied, st)
	case errSecMissingEntitlement:
		return fmt.Errorf("store: Keychain %s: the signature lacks the keychain-access-groups entitlement (OSStatus %d): %w", op, st, errors.ErrUnsupported)
	}
	return fmt.Errorf("store: Keychain %s failed (OSStatus %d)", op, st)
}
