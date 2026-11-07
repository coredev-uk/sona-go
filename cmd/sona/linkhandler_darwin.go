//go:build darwin

package main

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/app"
)

// macLinks is the default app for Apple Music links on macOS
// (app.LinkSchemes): Launch Services' choice, read with NSWorkspace's
// URLForApplicationToOpenURL: and written with its
// setDefaultApplicationAtURL:toOpenURLsWithScheme:completionHandler: (macOS
// 12 and later). It is a services.LinkHandler.
//
// Music.app owns music:// and itms:// until the user picks another app, so
// declaring the schemes in Info.plist is not enough for Sona to get them, and
// Sona can only choose itself while the running bundle declares them (a
// release Sona.app does; the development bundle of devbundle.sh, which has no
// URL types, and a source build don't, and report errors.ErrUnsupported).
type macLinks struct{}

// Default reports whether Launch Services opens every scheme with this
// bundle, else the display name of the app it opens the first other one with.
func (macLinks) Default() (bool, string, error) {
	var (
		sona  = true
		other string
		err   error
	)
	inPool(func() {
		var self string
		if self, err = ownBundle(); err != nil {
			return
		}
		for _, scheme := range app.LinkSchemes {
			id, name := handlerOf(scheme)
			if id == self {
				continue
			}
			if sona {
				other = name
			}
			sona = false
		}
	})
	return sona && err == nil, other, err
}

// MakeDefault asks Launch Services to open every scheme with this bundle.
// macOS may ask the user to confirm each change, so the wait is ctx's, not a
// fixed time. It returns as soon as the system has answered for each scheme.
func (macLinks) MakeDefault(ctx context.Context) error {
	var err error
	inPool(func() { _, err = ownBundle() })
	if err != nil {
		return err
	}
	for _, scheme := range app.LinkSchemes {
		answer := make(chan error, 1)
		ran := false
		// NSWorkspace is an AppKit object: message it from the main thread,
		// like MyGo's own AppKit calls. The completion handler arrives later
		// on a system queue.
		mygo.RunOnMain(func() {
			ran = true
			inPool(func() { setHandler(scheme, answer) })
		})
		if !ran {
			return errors.New("macOS link handler: Sona is quitting")
		}
		select {
		case err := <-answer:
			if err != nil {
				return fmt.Errorf("macOS link handler: %s: %w", scheme, err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// The Objective-C runtime is reached through purego, like MyGo's own AppKit
// calls: no cgo for these few messages.
var (
	loadOnce sync.Once
	loadErr  error
)

func load() {
	for _, path := range []string{
		"/System/Library/Frameworks/Foundation.framework/Foundation",
		"/System/Library/Frameworks/AppKit.framework/AppKit",
	} {
		if _, err := purego.Dlopen(path, purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
			loadErr = err
			return
		}
	}
}

const nsUTF8StringEncoding = 4

func class(name string) objc.ID { return objc.ID(objc.GetClass(name)) }

func send(id objc.ID, sel string, args ...any) objc.ID {
	return id.Send(objc.RegisterName(sel), args...)
}

// inPool runs fn on a locked thread inside an autorelease pool, which drops
// the autoreleased objects the messages return.
func inPool(fn func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	loadOnce.Do(load)
	pool := send(send(class("NSAutoreleasePool"), "alloc"), "init")
	defer send(pool, "drain")
	fn()
}

// nsString is an autoreleased NSString of s.
func nsString(s string) objc.ID {
	str := send(send(class("NSString"), "alloc"), "initWithBytes:length:encoding:",
		unsafe.Pointer(unsafe.StringData(s)), uint(len(s)), uint(nsUTF8StringEncoding))
	runtime.KeepAlive(s)
	return send(str, "autorelease")
}

// goString copies an NSString into a Go string.
func goString(str objc.ID) string {
	if str == 0 {
		return ""
	}
	p := objc.Send[*byte](str, objc.RegisterName("UTF8String"))
	n := objc.Send[uint](str, objc.RegisterName("lengthOfBytesUsingEncoding:"), uint(nsUTF8StringEncoding))
	if p == nil {
		return ""
	}
	return string(unsafe.Slice(p, n))
}

// ownBundle is the running bundle's identifier. It fails with
// errors.ErrUnsupported unless the bundle declares every scheme in its
// Info.plist (CFBundleURLTypes): macOS only lets an app be the handler of
// schemes it declares.
func ownBundle() (string, error) {
	if loadErr != nil {
		return "", loadErr
	}
	bundle := send(class("NSBundle"), "mainBundle")
	id := goString(send(bundle, "bundleIdentifier"))
	if id == "" {
		return "", fmt.Errorf("macOS link handler: Sona isn't running from an app bundle: %w", errors.ErrUnsupported)
	}
	declared := map[string]bool{}
	types := send(bundle, "objectForInfoDictionaryKey:", nsString("CFBundleURLTypes"))
	for i := range count(types) {
		schemes := send(send(types, "objectAtIndex:", i), "objectForKey:", nsString("CFBundleURLSchemes"))
		for j := range count(schemes) {
			declared[strings.ToLower(goString(send(schemes, "objectAtIndex:", j)))] = true
		}
	}
	for _, scheme := range app.LinkSchemes {
		if !declared[scheme] {
			return "", fmt.Errorf("macOS link handler: %s doesn't declare %s:// links: %w", id, scheme, errors.ErrUnsupported)
		}
	}
	return id, nil
}

// count is an NSArray's length; 0 for nil.
func count(array objc.ID) uint {
	if array == 0 {
		return 0
	}
	return objc.Send[uint](array, objc.RegisterName("count"))
}

// handlerOf is the bundle identifier and display name of the app Launch
// Services opens scheme links with, "" for both when none does.
func handlerOf(scheme string) (id, name string) {
	url := send(class("NSURL"), "URLWithString:", nsString(scheme+"://music.apple.com/"))
	appURL := send(send(class("NSWorkspace"), "sharedWorkspace"), "URLForApplicationToOpenURL:", url)
	if appURL == 0 {
		return "", ""
	}
	id = goString(send(send(class("NSBundle"), "bundleWithURL:", appURL), "bundleIdentifier"))
	path := send(appURL, "path")
	name = goString(send(send(class("NSFileManager"), "defaultManager"), "displayNameAtPath:", path))
	return id, strings.TrimSuffix(name, ".app")
}

// setHandler starts making this bundle the handler of scheme; the system's
// answer (nil when it took) goes to answer, which has room for it.
func setHandler(scheme string, answer chan<- error) {
	block := objc.NewBlock(func(_ objc.Block, failure objc.ID) {
		if failure == 0 {
			answer <- nil
			return
		}
		inPool(func() { answer <- errors.New(goString(send(failure, "localizedDescription"))) })
	})
	// The system keeps its own reference until it has called the handler.
	defer block.Release()
	send(send(class("NSWorkspace"), "sharedWorkspace"), "setDefaultApplicationAtURL:toOpenURLsWithScheme:completionHandler:",
		send(send(class("NSBundle"), "mainBundle"), "bundleURL"), nsString(scheme), block)
}
