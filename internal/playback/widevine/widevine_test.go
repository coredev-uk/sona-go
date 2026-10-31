//go:build linux || (darwin && cgo)

package widevine

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/cdm"
	"github.com/coredev-uk/sona-go/internal/playback/cdm/cdmtest"
)

func TestPSSH(t *testing.T) {
	kid, _ := hex.DecodeString("000000004224e8e0001db05d823e28e5")
	got, err := PSSH(kid)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := hex.DecodeString("00000034" + "70737368" + "00000000" + "edef8ba979d64acea3c827dcd51d21ed" + "00000014" + "08011210" + "000000004224e8e0001db05d823e28e5")
	if !bytes.Equal(got, want) {
		t.Fatalf("pssh %x", got)
	}
	if _, err := PSSH(kid[:8]); err == nil {
		t.Fatal("short KID accepted")
	}
}

// TestCDM exercises Google's real CDM, which cdmtest downloads and
// verifies the way Sona does (or SONA_WIDEVINE_CDM names): initialisation,
// licence request generation, timer delivery, a decrypt without a key,
// and session close. No network once the CDM is installed.
func TestCDM(t *testing.T) {
	path := cdmtest.Library(t)
	c, err := Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	t.Logf("CDM %s (manifest %s, %s) interface %d host %d", c.Version(), cdm.LibraryVersion(path), path, c.Interface(), c.HostInterface())
	if c.Version() == "" || (c.Interface() != 10 && c.Interface() != 11) {
		t.Fatalf("version %q interface %d", c.Version(), c.Interface())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	kid, _ := hex.DecodeString("000000004224e8e0001db05d823e28e5")
	pssh, _ := PSSH(kid)
	id, req, err := c.CreateSession(ctx, pssh)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" || len(req) < 100 {
		t.Fatalf("session %q request %d bytes", id, len(req))
	}
	if err := c.UpdateSession(ctx, id, []byte("not a licence")); err == nil {
		t.Fatal("garbage licence accepted")
	}

	iv := make([]byte, 16)
	data := []byte("0123456789abcdef")
	bufs := []Buffer{{KeyID: kid, IV: iv, Data: data, Out: make([]byte, len(data))}}
	if err := c.Decrypt(bufs); !errors.Is(err, ErrNoKey) && !errors.Is(err, ErrDecrypt) {
		t.Fatalf("decrypt without licence: %v", err)
	}
	if err := c.CloseSession(ctx, id); err != nil {
		t.Fatal(err)
	}
	// The CDM schedules periodic work through Host::SetTimer; the host must
	// call TimerExpired back.
	deadline := time.Now().Add(3 * time.Second)
	for c.TimersFired() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if c.TimersFired() == 0 {
		t.Fatal("no CDM timer fired within 3s")
	}
	t.Logf("timers fired: %d", c.TimersFired())
}

func TestKeyStatusString(t *testing.T) {
	for s, want := range map[KeyStatus]string{
		KeyUsable: "usable", KeyOutputRestricted: "output-restricted", KeyInternalError: "internal-error", 9: "key-status-9",
	} {
		if got := s.String(); got != want {
			t.Errorf("KeyStatus(%d) = %q, want %q", int(s), got, want)
		}
	}
}
