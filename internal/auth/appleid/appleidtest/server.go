// Package appleidtest is a fake of Apple's native sign-in endpoints
// (activate.apple.com, idmsa.apple.com, buy.itunes.apple.com) for tests.
// It verifies the SRP evidence with a real server-side computation, the
// hashcash stamp, and the rolling scnt/session headers, so a client that
// passes against it speaks the protocol package appleid documents.
package appleidtest

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"math/bits"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/coredev-uk/sona-go/internal/auth/appleid"
)

// Fixed identifiers the fake hands out.
const (
	DevToken   = "dev"
	WidgetKey  = "widget-key"
	OAuthToken = "OATTKN-fake"
	FormSCNT   = "form-scnt"
	LiteID     = "lite-session"
	Code       = "123456"
	HCBits     = 6
)

// Config describes the fake account.
type Config struct {
	Account   string // matched case-insensitively
	Password  string
	TwoFactor bool // 409 after the password; trusted devices prompted, phone 1 listed
	// NoDevices: two-factor by phone only. Apple texts phone 1 itself
	// unless NoAutoSend, which lists phones 1 (sms) and 2 (voice) and
	// sends nothing until asked.
	NoDevices  bool
	NoAutoSend bool
	// NonFTEU sets phoneNumberVerification.enableNonFTEU; texted codes
	// must then carry phoneNumber.nonFTEU.
	NonFTEU    bool
	MUT        string
	Restricted bool
	// NoSubscription redirects to the no-subscription route.
	NoSubscription bool
	Protocol       string // s2k (default) or s2k_fo
}

// Server is a running fake.
type Server struct {
	*httptest.Server
	t   testing.TB
	cfg Config

	mu       sync.Mutex
	scnt     int
	session  string
	b        *big.Int
	A, B     []byte
	salt     []byte
	authed   bool   // password (and second factor) accepted
	pending  bool   // waiting for a second factor
	codeTo   int    // phone the last code went to; 0 = devices, -1 = none
	codeMode string // "sms" or "voice" for phone codes
	calls    []string
}

// New starts a fake; it stops with the test.
func New(t testing.TB, cfg Config) *Server {
	if cfg.Protocol == "" {
		cfg.Protocol = "s2k"
	}
	s := &Server{t: t, cfg: cfg, session: "sess-1", salt: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// Options points an appleid.Session at the fake.
func (s *Server) Options() appleid.Options {
	return appleid.Options{
		HTTP:          s.Client(),
		ActivateURL:   s.URL + "/mkauth",
		IDMSOrigin:    s.URL,
		MusicTokenURL: s.URL + "/getMusicUserToken",
	}
}

// Requests returns "METHOD /path" for every request served so far.
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

func (s *Server) nextSCNT(w http.ResponseWriter) {
	s.scnt++
	w.Header().Set("scnt", "scnt-"+strconv.Itoa(s.scnt))
	w.Header().Set("X-Apple-ID-Session-Id", s.session)
	w.Header().Set("X-Apple-Auth-Attributes", "attrs-"+strconv.Itoa(s.scnt))
}

func serviceErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"serviceErrors":[{"code":%q,"message":%q}]}`, code, msg)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, r.Method+" "+r.URL.Path)

	if strings.HasPrefix(r.URL.Path, "/appleauth/auth") && r.URL.Path != "/appleauth/auth/authorize/signin" {
		if bad := s.checkWidgetHeaders(r); bad != "" {
			serviceErr(w, http.StatusBadRequest, "-1", bad)
			return
		}
	}

	switch r.Method + " " + r.URL.Path {
	case "GET /mkauth":
		if r.URL.Query().Get("jwtToken") != DevToken {
			http.Error(w, "bad jwt", http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/IDMSWebAuth/auth?oauth_token="+OAuthToken, http.StatusFound)

	case "GET /IDMSWebAuth/auth":
		http.SetCookie(w, &http.Cookie{Name: "dawsp", Value: "page", Path: "/"})
		boot, _ := json.Marshal(map[string]any{"direct": map[string]any{
			"oauth_token":      OAuthToken,
			"locale":           "US-EN",
			"appId":            165,
			"appReturnUrl":     "https://p32-buy.itunes.apple.com/commerce/account/validateMusicKitToken?Pod=32&liteSessionId=" + LiteID,
			"authWidgetConfig": map[string]any{"widgetKey": WidgetKey, "skVersion": "7"},
		}})
		fmt.Fprintf(w, `<html><body><script type="application/json" id="oauth2_widget_login_boot_args">%s</script>
<form id="callBackForm" action="/auth" method="post"><input type="hidden" id="rememberMe" name="rememberMe" value="false"/>
<input type="hidden" id="oAuthToken" name="oAuthToken" value="%s"/><input type="hidden" id="scnt" name="scnt" value="%s" /></form></body></html>`,
			boot, OAuthToken, FormSCNT)

	case "GET /appleauth/auth/authorize/signin":
		q := r.URL.Query()
		if q.Get("client_id") != WidgetKey || q.Get("frame_id") == "" || q.Get("state") != q.Get("frame_id") {
			http.Error(w, "bad widget query", http.StatusBadRequest)
			return
		}
		s.nextSCNT(w)
		w.Header().Set("X-Apple-HC-Challenge", "chal-1")
		w.Header().Set("X-Apple-HC-Bits", strconv.Itoa(HCBits))
		fmt.Fprint(w, "<html>widget</html>")

	case "POST /appleauth/auth/signin/init":
		var in struct {
			A           string   `json:"a"`
			AccountName string   `json:"accountName"`
			Protocols   []string `json:"protocols"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Protocols) == 0 {
			serviceErr(w, http.StatusBadRequest, "-1", "bad init")
			return
		}
		s.A, _ = base64.StdEncoding.DecodeString(in.A)
		v := new(big.Int).Exp(g, s.x(), n)
		bb := make([]byte, 32)
		rand.Read(bb)
		s.b = new(big.Int).SetBytes(bb)
		B := new(big.Int).Exp(g, s.b, n)
		B.Add(B, new(big.Int).Mul(k(), v)).Mod(B, n)
		s.B = B.Bytes()
		s.nextSCNT(w)
		json.NewEncoder(w).Encode(map[string]any{
			"iteration": 1000, "salt": base64.StdEncoding.EncodeToString(s.salt),
			"protocol": s.cfg.Protocol, "b": base64.StdEncoding.EncodeToString(s.B), "c": "c-1",
		})

	case "POST /appleauth/auth/signin/complete":
		var in struct {
			AccountName string `json:"accountName"`
			M1, M2, C   string
		}
		json.NewDecoder(r.Body).Decode(&in)
		s.nextSCNT(w)
		w.Header().Set("X-Apple-HC-Challenge", "chal-2")
		w.Header().Set("X-Apple-HC-Bits", strconv.Itoa(HCBits))
		if !validStamp(r.Header.Get("X-APPLE-HC"), "chal-1", HCBits) {
			serviceErr(w, http.StatusBadRequest, "-20209", "bad hashcash")
			return
		}
		m1, _ := base64.StdEncoding.DecodeString(in.M1)
		m2, _ := base64.StdEncoding.DecodeString(in.M2)
		wantM1, wantM2 := s.evidence(in.AccountName)
		if in.C != "c-1" || string(m1) != string(wantM1) || string(m2) != string(wantM2) {
			serviceErr(w, http.StatusUnauthorized, "-20101", "Check the account information you entered and try again.")
			return
		}
		if s.cfg.TwoFactor {
			// Apple prompts the trusted devices when it answers 409.
			s.pending, s.codeTo = true, 0
			w.Header().Set("Location", "/auth")
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"authType":"hsa2"}`)
			return
		}
		s.signedIn(w)

	case "GET /appleauth/auth":
		// The widget loads this page as HTML and reads its boot_args.
		if !s.pending || r.Header.Get("Accept") != "text/html" {
			serviceErr(w, http.StatusUnauthorized, "-1", "no pending 2fa page")
			return
		}
		s.nextSCNT(w)
		phone := map[string]any{"id": 1, "numberWithDialCode": "+44 ••••• ••••12", "obfuscatedNumber": "••••• ••••12", "pushMode": "sms"}
		phones := []any{phone}
		if s.cfg.NoAutoSend {
			phones = append(phones, map[string]any{"id": 2, "numberWithDialCode": "+1 (•••) •••-••34", "pushMode": "voice"})
		}
		pnv := map[string]any{
			"securityCode":        map[string]any{"length": 6, "tooManyCodesSent": false},
			"trustedPhoneNumbers": phones,
			"enableNonFTEU":       s.cfg.NonFTEU,
		}
		direct := map[string]any{
			"hasTrustedDevices": !s.cfg.NoDevices,
			"authType":          "hsa2",
			"authUserType":      "hsa2",
			"twoSV":             map[string]any{"phoneNumberVerification": pnv, "authFactors": []string{"type_hsa2"}},
		}
		switch {
		case s.cfg.NoDevices && s.cfg.NoAutoSend:
			s.codeTo = -1
		case s.cfg.NoDevices:
			pnv["phoneNumber"] = phone
			s.codeTo, s.codeMode = 1, "sms"
		}
		args, _ := json.Marshal(map[string]any{"direct": direct, "additional": map[string]any{"canRoute2sv": true}})
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<div id="stocking"><script type="application/json" class="boot_args">%s</script></div>`, args)

	case "PUT /appleauth/auth/verify/phone":
		var in struct {
			PhoneNumber struct{ ID int } `json:"phoneNumber"`
			Mode        string           `json:"mode"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		if !s.pending || (in.PhoneNumber.ID != 1 && (!s.cfg.NoAutoSend || in.PhoneNumber.ID != 2)) ||
			(in.Mode != "sms" && in.Mode != "voice") {
			serviceErr(w, http.StatusBadRequest, "-1", "bad phone")
			return
		}
		s.codeTo, s.codeMode = in.PhoneNumber.ID, in.Mode
		s.nextSCNT(w)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"phoneNumber":  map[string]any{"id": in.PhoneNumber.ID, "pushMode": in.Mode},
			"securityCode": map[string]any{"length": 6},
		})

	case "PUT /appleauth/auth/verify/trusteddevice/securitycode":
		if s.cfg.NoDevices {
			serviceErr(w, http.StatusBadRequest, "-1", "no trusted devices")
			return
		}
		s.codeTo = 0
		s.nextSCNT(w)
		w.WriteHeader(http.StatusAccepted)

	case "POST /appleauth/auth/verify/trusteddevice/securitycode",
		"POST /appleauth/auth/verify/phone/securitycode":
		var in struct {
			SecurityCode struct{ Code string } `json:"securityCode"`
			PhoneNumber  *struct {
				ID      int  `json:"id"`
				NonFTEU bool `json:"nonFTEU"`
			} `json:"phoneNumber"`
			Mode string `json:"mode"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		s.nextSCNT(w)
		ok := s.pending && in.SecurityCode.Code == Code
		if in.PhoneNumber == nil {
			ok = ok && s.codeTo == 0
		} else {
			ok = ok && in.PhoneNumber.ID == s.codeTo && in.Mode == s.codeMode && in.PhoneNumber.NonFTEU == s.cfg.NonFTEU
		}
		if !ok {
			serviceErr(w, http.StatusBadRequest, "-21669", "Incorrect verification code.")
			return
		}
		s.pending = false
		w.WriteHeader(http.StatusNoContent)

	case "GET /appleauth/auth/2sv/trust":
		if s.pending {
			serviceErr(w, http.StatusUnauthorized, "-1", "2fa not done")
			return
		}
		s.signedIn(w)

	case "POST /auth":
		r.ParseForm()
		c, err := r.Cookie("myacinfo")
		if err != nil || c.Value != "ok" || !s.authed ||
			r.PostForm.Get("oAuthToken") != OAuthToken || r.PostForm.Get("scnt") != FormSCNT {
			http.Error(w, "not signed in", http.StatusUnauthorized)
			return
		}
		if s.cfg.NoSubscription {
			http.Redirect(w, r, "/validate?no=1", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/validate", http.StatusFound)

	case "GET /validate": // stands in for buy.itunes.apple.com/…/validateMusicKitToken
		q := url.Values{"liteSessionId": {LiteID}, "pod": {"32"}, "appleId": {s.cfg.Account}}
		if r.URL.Query().Get("no") == "1" {
			q = url.Values{"isAppleMusicSubscriber": {"false"}, "cid": {"c"}}
		}
		http.Redirect(w, r, appleid.AuthorizeOrigin+"/?"+q.Encode(), http.StatusFound)

	case "GET /getMusicUserToken":
		q := r.URL.Query()
		if q.Get("liteSessionId") != LiteID || q.Get("jwtToken") != DevToken || !s.authed {
			http.Error(w, "bad lite session", http.StatusUnauthorized)
			return
		}
		itre := "0"
		if s.cfg.Restricted {
			itre = "1"
		}
		json.NewEncoder(w).Encode(map[string]any{"musicUserToken": s.cfg.MUT, "itre": itre})

	default:
		http.NotFound(w, r)
	}
}

func (s *Server) signedIn(w http.ResponseWriter) {
	s.authed = true
	http.SetCookie(w, &http.Cookie{Name: "myacinfo", Value: "ok", Path: "/"})
	w.Header().Set("X-Apple-Session-Token", "session-token")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"authType":"hsa2"}`)
}

// checkWidgetHeaders reports the first missing widget header, or "".
func (s *Server) checkWidgetHeaders(r *http.Request) string {
	want := map[string]string{
		"X-Apple-Widget-Key":        WidgetKey,
		"X-Apple-OAuth-Client-Id":   WidgetKey,
		"X-Apple-OAuth-Client-Type": "firstPartyAuth",
		"X-Apple-ID-Session-Id":     s.session,
		"scnt":                      "scnt-" + strconv.Itoa(s.scnt),
		"X-Apple-Auth-Attributes":   "attrs-" + strconv.Itoa(s.scnt),
		"X-Requested-With":          "XMLHttpRequest",
	}
	for k, v := range want {
		if r.Header.Get(k) != v {
			return "header " + k
		}
	}
	if f := r.Header.Get("X-Apple-Frame-Id"); f == "" || r.Header.Get("X-Apple-OAuth-State") != f {
		return "frame id"
	}
	return ""
}

// Server-side SRP-6a for the same group and derivation as the widget.

var (
	n, _ = new(big.Int).SetString("AC6BDB41324A9A9BF166DE5E1389582FAF72B6651987EE07FC3192943DB56050A37329CBB4A099ED8193E0757767A13DD52312AB4B03310DCD7F48A9DA04FD50E8083969EDB767B0CF6095179A163AB3661A05FBD5FAAAE82918A9962F0B93B855F97993EC975EEAA80D740ADBF4FF747359D041D5C33EA71D281E446B14773BCA97B43A23FB801676BD207A436C6481F1D2B9078717461A5B9D32E688F87748544523B524B0D57D5EA77A2775D2ECFA032CFBDBF52FB3786160279004E57AE6AF874E7303CE53299CCC041C7BC308D82A5698F3A8D0C38271AE35F8E9DBFBB694B5C803D89F7AE435DE236D525F54759B65E372FCD68EF20FA7111F9E4AFF73", 16)
	g    = big.NewInt(2)
)

func h(parts ...[]byte) []byte {
	d := sha256.New()
	for _, p := range parts {
		d.Write(p)
	}
	return d.Sum(nil)
}

func pad(x *big.Int) []byte { return x.FillBytes(make([]byte, 256)) }

func k() *big.Int { return new(big.Int).SetBytes(h(n.Bytes(), pad(g))) }

func (s *Server) x() *big.Int {
	pw := h([]byte(s.cfg.Password))
	if s.cfg.Protocol == "s2k_fo" {
		pw = []byte(fmt.Sprintf("%x", pw))
	}
	p, err := pbkdf2.Key(sha256.New, string(pw), s.salt, 1000, 32)
	if err != nil {
		s.t.Fatal(err)
	}
	return new(big.Int).SetBytes(h(s.salt, h([]byte(":"), p)))
}

// evidence is what a client knowing the password must send.
func (s *Server) evidence(account string) (m1, m2 []byte) {
	if !strings.EqualFold(account, s.cfg.Account) {
		return nil, nil
	}
	A := new(big.Int).SetBytes(s.A)
	u := new(big.Int).SetBytes(h(pad(A), pad(new(big.Int).SetBytes(s.B))))
	v := new(big.Int).Exp(g, s.x(), n)
	S := new(big.Int).Exp(v, u, n)
	S.Mul(S, A).Mod(S, n).Exp(S, s.b, n)
	K := h(pad(S))
	hn, hg := h(n.Bytes()), h(pad(g))
	xor := make([]byte, len(hn))
	for i := range xor {
		xor[i] = hn[i] ^ hg[i]
	}
	xor = new(big.Int).SetBytes(xor).Bytes()
	m1 = h(xor, h([]byte(strings.ToLower(account))), s.salt, s.A, s.B, K)
	return m1, h(s.A, m1, K)
}

func validStamp(stamp, challenge string, nbits int) bool {
	f := strings.Split(stamp, ":")
	if len(f) != 6 || f[0] != "1" || f[1] != strconv.Itoa(nbits) || len(f[2]) != 14 || f[3] != challenge || f[4] != "" {
		return false
	}
	sum := sha1.Sum([]byte(stamp))
	z := 0
	for _, b := range sum {
		if b != 0 {
			z += bits.LeadingZeros8(b)
			break
		}
		z += 8
	}
	return z >= nbits
}
