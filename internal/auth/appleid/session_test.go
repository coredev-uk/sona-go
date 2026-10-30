package appleid_test

import (
	"context"
	"errors"
	"testing"

	"github.com/coredev-uk/sona-go/internal/auth/appleid"
	"github.com/coredev-uk/sona-go/internal/auth/appleid/appleidtest"
)

func begin(t *testing.T, srv *appleidtest.Server) *appleid.Session {
	t.Helper()
	s, err := appleid.Begin(context.Background(), appleidtest.DevToken, srv.Options())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestSignInWithoutSecondFactor(t *testing.T) {
	srv := appleidtest.New(t, appleidtest.Config{Account: "a@example.com", Password: "pw", MUT: "mut-1", Restricted: true})
	s := begin(t, srv)
	ctx := context.Background()
	sf, err := s.SignIn(ctx, "A@Example.com ", "pw")
	if err != nil || sf != nil {
		t.Fatalf("SignIn = %v, %v", sf, err)
	}
	res, err := s.MusicUserToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.MusicUserToken != "mut-1" || !res.Restricted {
		t.Fatalf("result = %+v", res)
	}
	if _, err := s.MusicUserToken(ctx); !errors.Is(err, appleid.ErrOutOfOrder) {
		t.Fatalf("second MusicUserToken: %v", err)
	}
}

func TestSignInFallbackProtocol(t *testing.T) {
	srv := appleidtest.New(t, appleidtest.Config{Account: "a@example.com", Password: "pässwörd", MUT: "m", Protocol: "s2k_fo"})
	if _, err := begin(t, srv).SignIn(context.Background(), "a@example.com", "pässwörd"); err != nil {
		t.Fatal(err)
	}
}

func TestWrongPassword(t *testing.T) {
	srv := appleidtest.New(t, appleidtest.Config{Account: "a@example.com", Password: "pw", MUT: "m"})
	_, err := begin(t, srv).SignIn(context.Background(), "a@example.com", "nope")
	var se *appleid.ServiceError
	if !errors.Is(err, appleid.ErrInvalidCredentials) || !errors.As(err, &se) || se.Code != "-20101" || se.Message == "" {
		t.Fatalf("err = %v", err)
	}
}

func TestTrustedDeviceCode(t *testing.T) {
	srv := appleidtest.New(t, appleidtest.Config{Account: "a@example.com", Password: "pw", MUT: "mut-2", TwoFactor: true})
	s := begin(t, srv)
	ctx := context.Background()
	sf, err := s.SignIn(ctx, "a@example.com", "pw")
	if err != nil {
		t.Fatal(err)
	}
	if sf == nil || !sf.TrustedDevices || sf.CodeLength != 6 || len(sf.Phones) != 1 || sf.Phones[0].ID != 1 || sf.SentToPhone != 0 {
		t.Fatalf("second factor = %+v", sf)
	}
	if _, err := s.MusicUserToken(ctx); !errors.Is(err, appleid.ErrOutOfOrder) {
		t.Fatalf("token before code: %v", err)
	}
	if err := s.VerifyCode(ctx, "000000", 0, ""); !errors.Is(err, appleid.ErrIncorrectCode) {
		t.Fatalf("wrong code: %v", err)
	}
	if s.SecondFactor() == nil {
		t.Fatal("challenge closed by a wrong code")
	}
	if err := s.VerifyCode(ctx, appleidtest.Code, 0, ""); err != nil {
		t.Fatal(err)
	}
	res, err := s.MusicUserToken(ctx)
	if err != nil || res.MusicUserToken != "mut-2" || res.Restricted {
		t.Fatalf("token = %+v, %v", res, err)
	}
}

func TestPhoneCode(t *testing.T) {
	srv := appleidtest.New(t, appleidtest.Config{Account: "a@example.com", Password: "pw", MUT: "m", TwoFactor: true})
	s := begin(t, srv)
	ctx := context.Background()
	if _, err := s.SignIn(ctx, "a@example.com", "pw"); err != nil {
		t.Fatal(err)
	}
	if err := s.SendCodeToPhone(ctx, 1, "sms"); err != nil {
		t.Fatal(err)
	}
	// The code now goes to the phone; submitting it as a device code fails.
	if err := s.VerifyCode(ctx, appleidtest.Code, 0, ""); !errors.Is(err, appleid.ErrIncorrectCode) {
		t.Fatalf("device code after SMS: %v", err)
	}
	if err := s.VerifyCode(ctx, appleidtest.Code, 1, "sms"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MusicUserToken(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestNoTrustedDevicesReportsTextedPhone(t *testing.T) {
	srv := appleidtest.New(t, appleidtest.Config{Account: "a@example.com", Password: "pw", MUT: "m", TwoFactor: true, NoDevices: true})
	sf, err := begin(t, srv).SignIn(context.Background(), "a@example.com", "pw")
	if err != nil || sf.TrustedDevices || sf.SentToPhone != 1 {
		t.Fatalf("second factor = %+v, %v", sf, err)
	}
}

func TestNoSubscription(t *testing.T) {
	srv := appleidtest.New(t, appleidtest.Config{Account: "a@example.com", Password: "pw", NoSubscription: true})
	s := begin(t, srv)
	if _, err := s.SignIn(context.Background(), "a@example.com", "pw"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MusicUserToken(context.Background()); !errors.Is(err, appleid.ErrNoSubscription) {
		t.Fatalf("err = %v", err)
	}
}
