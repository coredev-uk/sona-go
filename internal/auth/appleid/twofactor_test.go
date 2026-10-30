package appleid

import (
	"errors"
	"testing"
)

// Boot data shapes the widget's hsa2 reducer handles (home.js); see
// loadSecondFactor.
func TestTwoFactorBootRouting(t *testing.T) {
	const phone = `{"id":3,"obfuscatedNumber":"••• ••34","pushMode":"voice"}`
	page := func(direct string) []byte {
		return []byte(`<div id="stocking"><script type="application/json" class="boot_args">{"direct":` + direct + `}</script></div>`)
	}
	tests := []struct {
		name    string
		direct  string
		want    SecondFactor
		wantErr error
	}{
		{
			name:   "devices and phones both offered, nothing counted as texted",
			direct: `{"hasTrustedDevices":true,"twoSV":{"phoneNumberVerification":{"trustedPhoneNumbers":[` + phone + `],"phoneNumber":` + phone + `,"securityCode":{"length":8}}}}`,
			want:   SecondFactor{CodeLength: 8, TrustedDevices: true, Phones: []Phone{{ID: 3, Number: "••• ••34", Mode: "voice"}}},
		},
		{
			name:   "devices without phones",
			direct: `{"hasTrustedDevices":true,"twoSV":{}}`,
			want:   SecondFactor{CodeLength: 6, TrustedDevices: true},
		},
		{
			name:   "bridge data overrides twoSV, apostrophe unescaped",
			direct: `{"hasTrustedDevices":false,"name":"O\'Neil","twoSV":{"phoneNumberVerification":{"securityCode":{"length":6}},"bridgeInitiateData":{"phoneNumberVerification":{"phoneNumber":` + phone + `,"securityCode":{"length":6}}}}}`,
			want:   SecondFactor{CodeLength: 6, Phones: []Phone{{ID: 3, Number: "••• ••34", Mode: "voice"}}, SentToPhone: 3},
		},
		{
			name:   "phone list via trustedDeviceVerification, nothing sent",
			direct: `{"twoSV":{"trustedDeviceVerification":{"phoneNumberVerification":{"trustedPhoneNumbers":[` + phone + `]}}}}`,
			want:   SecondFactor{CodeLength: 6, Phones: []Phone{{ID: 3, Number: "••• ••34", Mode: "voice"}}},
		},
		{
			name:    "security keys only",
			direct:  `{"hasTrustedDevices":false,"twoSV":{"fsaVerification":{},"authFactors":["fsa2_hsa2"]}}`,
			wantErr: ErrUnsupported2FA,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			boot, err := parseTwoFactorBoot(page(tt.direct))
			if err != nil {
				t.Fatal(err)
			}
			sf, err := boot.secondFactor()
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if sf.CodeLength != tt.want.CodeLength || sf.TrustedDevices != tt.want.TrustedDevices ||
				sf.SentToPhone != tt.want.SentToPhone || len(sf.Phones) != len(tt.want.Phones) {
				t.Fatalf("got %+v, want %+v", *sf, tt.want)
			}
			for i := range sf.Phones {
				if sf.Phones[i] != tt.want.Phones[i] {
					t.Fatalf("phone %d = %+v", i, sf.Phones[i])
				}
			}
		})
	}
	if _, err := parseTwoFactorBoot([]byte(`<html>no data</html>`)); err == nil {
		t.Fatal("page without boot data accepted")
	}
}
