package store

import (
	"errors"
	"testing"
	"time"
)

type memBio struct {
	unsupported error
	item        []byte
	getErr      error
}

func (m *memBio) Available() error { return m.unsupported }
func (m *memBio) Get() ([]byte, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.item == nil {
		return nil, ErrNotFound
	}
	return m.item, nil
}
func (m *memBio) Set(b []byte) error { m.item = b; return nil }
func (m *memBio) Delete() error      { m.item = nil; return nil }

func TestOpenBiometricRoundTripAndErrors(t *testing.T) {
	kc := &memBio{}
	s, err := OpenBiometric(kc)
	if err != nil {
		t.Fatal(err)
	}
	if s.Backend() != BackendKeychainBiometric {
		t.Fatalf("backend = %v", s.Backend())
	}
	if _, err := s.Get(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty Get = %v", err)
	}
	want := Record{Token: "tok", ObtainedAt: time.Unix(1700000000, 0).UTC(), Restricted: true}
	if err := s.Set(want); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get(); err != nil || got != want {
		t.Fatalf("Get = %v, %v", got, err)
	}
	kc.getErr = ErrBiometricDenied
	if _, err := s.Get(); !errors.Is(err, ErrBiometricDenied) {
		t.Fatalf("cancelled Get = %v", err)
	}
	if err := s.Delete(); err != nil || kc.item != nil {
		t.Fatalf("Delete = %v, item %v", err, kc.item)
	}
}

func TestOpenBiometricUnsupported(t *testing.T) {
	if _, err := OpenBiometric(nil); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("nil keychain: %v", err)
	}
	kc := &memBio{unsupported: errors.Join(errors.ErrUnsupported, errors.New("ad hoc"))}
	if _, err := OpenBiometric(kc); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("ad-hoc build: %v", err)
	}
}
