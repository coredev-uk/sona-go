// Package appleid signs in to Apple Music natively: Sona collects the Apple
// Account name, password and two-factor code, and speaks the same protocol
// Apple's web sign-in widget does (idmsa.apple.com SRP-6a, hashcash,
// two-factor verification), then exchanges the resulting session for a
// Music User Token exactly as authorize.music.apple.com does. No browser is
// involved. See docs/auth.md §4.7.
//
// The password is used for one SRP exchange and never stored or logged.
// Each Session has its own cookie jar, dropped with the Session.
package appleid

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Default endpoints, observed on 2026-10-29.
const (
	ActivateURL      = "https://activate.apple.com/mkauth"
	IDMSOrigin       = "https://idmsa.apple.com"
	AuthorizeOrigin  = "https://authorize.music.apple.com"
	MusicTokenURL    = "https://buy.itunes.apple.com/commerce/account/getMusicUserToken"
	podMusicTokenURL = "https://p%s-buy.itunes.apple.com/commerce/account/getMusicUserToken"

	defaultUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36"
	maxBody   = 1 << 20
)

// Errors a sign-in can end with. Apple's own message, when it sent one, is
// in the wrapping *ServiceError.
var (
	ErrInvalidCredentials = errors.New("appleid: incorrect Apple Account or password")
	ErrIncorrectCode      = errors.New("appleid: incorrect verification code")
	ErrLocked             = errors.New("appleid: account locked or temporarily unavailable")
	ErrActionRequired     = errors.New("appleid: the account needs attention at account.apple.com")
	ErrNoSubscription     = errors.New("appleid: Apple Music subscription required")
	ErrUnsupportedCountry = errors.New("appleid: Apple Music is not available in this account's country")
	ErrUnsupported2FA     = errors.New("appleid: this account's two-factor method is not supported here")
	ErrOutOfOrder         = errors.New("appleid: sign-in step called out of order")
)

// ServiceError is an error response from Apple.
type ServiceError struct {
	Status  int    // HTTP status
	Code    string // Apple service error code, e.g. "-20101"
	Message string // Apple's user-facing message, possibly localized
	kind    error
}

func (e *ServiceError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	if e.Code != "" {
		return fmt.Sprintf("appleid: %s (%s)", msg, e.Code)
	}
	return "appleid: " + msg
}

func (e *ServiceError) Unwrap() error { return e.kind }

// Options configures a Session. Zero values use Apple's endpoints.
type Options struct {
	// HTTP supplies the transport and timeout. Its Jar and CheckRedirect
	// are not used: every Session owns a fresh jar.
	HTTP      *http.Client
	UserAgent string

	ActivateURL   string
	IDMSOrigin    string
	MusicTokenURL string // fixed token URL; disables Apple's pod host selection
	Now           func() time.Time
}

// Phone is a trusted phone number that can receive a code.
type Phone struct {
	ID     int    `json:"id"`
	Number string `json:"number"` // obfuscated by Apple, e.g. "+44 ••••• ••••12"
	Mode   string `json:"mode"`   // "sms" or "voice"
}

// SecondFactor describes the pending two-factor challenge.
//
// Every route the account has is offered: the trusted-device code
// (VerifyCode phoneID 0) when TrustedDevices, and a text or call to any of
// Phones. Apple prompts the trusted devices by itself when the password is
// accepted; accounts without one cannot use the device route.
type SecondFactor struct {
	CodeLength int `json:"codeLength"`
	// TrustedDevices is true when the account has trusted devices and
	// Apple prompted them; VerifyCode with phoneID 0 submits their code.
	TrustedDevices bool    `json:"trustedDevices"`
	Phones         []Phone `json:"phones"`
	// SentToPhone is the phone Apple already texted (accounts without
	// trusted devices), or 0.
	SentToPhone int `json:"sentToPhone,omitempty"`
}

// Result is the Music User Token Apple minted.
type Result struct {
	MusicUserToken string
	Restricted     bool // itre: explicit content restricted
}

// String keeps the token out of logs.
func (r Result) String() string {
	return fmt.Sprintf("mut(len=%d restricted=%v)", len(r.MusicUserToken), r.Restricted)
}

type stage int

const (
	stageBegun stage = iota
	stageSecondFactor
	stageAuthenticated
	stageDone
)

// Session is one native sign-in attempt. Methods are safe for concurrent
// use; they run one at a time.
type Session struct {
	mu   sync.Mutex
	hc   *http.Client
	ua   string
	now  func() time.Time
	idms string

	devToken string
	tokenURL string

	// From Apple's IDMSWebAuth page.
	pageURL    string
	widgetKey  string
	appID      string
	locale     string
	oauthToken string
	formSCNT   string
	frameID    string

	// Rolling request state from idmsa responses.
	scnt        string
	sessionID   string
	authAttrs   string
	hcChallenge string
	hcBits      int

	account string
	stage   stage
	second  *SecondFactor
	nonFTEU bool // phoneNumberVerification.enableNonFTEU
}

// Begin opens Apple's Apple Music sign-in for devToken (the web player
// token) and loads the sign-in widget's state. It makes no credential
// requests.
func Begin(ctx context.Context, devToken string, opts Options) (*Session, error) {
	if devToken == "" {
		return nil, errors.New("appleid: developer token required")
	}
	base := opts.HTTP
	if base == nil {
		base = http.DefaultClient
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	s := &Session{
		ua:       opts.UserAgent,
		now:      opts.Now,
		idms:     strings.TrimRight(opts.IDMSOrigin, "/"),
		devToken: devToken,
		tokenURL: opts.MusicTokenURL,
		frameID:  frameID(),
	}
	if s.ua == "" {
		s.ua = defaultUA
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.idms == "" {
		s.idms = IDMSOrigin
	}
	activate := opts.ActivateURL
	if activate == "" {
		activate = ActivateURL
	}
	s.hc = &http.Client{
		Transport: base.Transport,
		Timeout:   base.Timeout,
		Jar:       jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("appleid: too many redirects")
			}
			if isAuthorizeRedirect(req.URL) {
				return http.ErrUseLastResponse // the answer; never fetched
			}
			return nil
		},
	}

	u := activate + "?" + url.Values{"jwtToken": {devToken}, "LogoURL": {""}}.Encode()
	resp, body, err := s.do(ctx, http.MethodGet, u, nil, nil)
	if err != nil {
		return nil, err
	}
	if loc, ok := authorizeLocation(resp); ok {
		return nil, authorizeError(loc)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("appleid: sign-in page: %s", resp.Status)
	}
	s.pageURL = resp.Request.URL.String()
	if err := s.parsePage(body); err != nil {
		return nil, err
	}
	s.track(resp)

	// Load the widget frame as the parent page would; it issues the
	// session id, scnt, auth attributes and the first hashcash challenge.
	q := url.Values{
		"frame_id":      {s.frameID},
		"language":      {s.locale},
		"skVersion":     {"7"},
		"iframeId":      {s.frameID},
		"client_id":     {s.widgetKey},
		"redirect_uri":  {s.idms},
		"response_type": {"code"},
		"response_mode": {"web_message"},
		"state":         {s.frameID},
		"authVersion":   {"latest"},
	}
	resp, _, err = s.do(ctx, http.MethodGet, s.idms+"/appleauth/auth/authorize/signin?"+q.Encode(), nil, http.Header{
		"Referer": {s.idms + "/"},
	})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("appleid: sign-in widget: %s", resp.Status)
	}
	s.track(resp)
	if s.scnt == "" || s.sessionID == "" {
		return nil, errors.New("appleid: sign-in widget did not start a session")
	}
	return s, nil
}

var (
	bootArgsRe = regexp.MustCompile(`(?s)<script[^>]*id="oauth2_widget_login_boot_args"[^>]*>(.*?)</script>`)
	inputRe    = regexp.MustCompile(`<input[^>]*\bid="([A-Za-z]+)"[^>]*\bvalue="([^"]*)"`)
)

func (s *Session) parsePage(body []byte) error {
	m := bootArgsRe.FindSubmatch(body)
	if m == nil {
		return errors.New("appleid: sign-in page has no widget configuration")
	}
	var boot struct {
		Direct struct {
			OAuthToken       string `json:"oauth_token"`
			Locale           string `json:"locale"`
			AppID            any    `json:"appId"`
			AppReturnURL     string `json:"appReturnUrl"`
			AuthWidgetConfig struct {
				WidgetKey string `json:"widgetKey"`
			} `json:"authWidgetConfig"`
		} `json:"direct"`
	}
	if err := json.Unmarshal(m[1], &boot); err != nil {
		return fmt.Errorf("appleid: widget configuration: %w", err)
	}
	d := boot.Direct
	s.oauthToken = d.OAuthToken
	s.widgetKey = d.AuthWidgetConfig.WidgetKey
	s.locale = d.Locale
	if s.locale == "" {
		s.locale = "US-EN"
	}
	if d.AppID != nil {
		s.appID = strings.Trim(fmt.Sprint(d.AppID), `"`)
	}
	for _, in := range inputRe.FindAllSubmatch(body, -1) {
		switch string(in[1]) {
		case "scnt":
			s.formSCNT = html.UnescapeString(string(in[2]))
		case "oAuthToken":
			if s.oauthToken == "" {
				s.oauthToken = html.UnescapeString(string(in[2]))
			}
		}
	}
	if s.oauthToken == "" || s.widgetKey == "" {
		return errors.New("appleid: sign-in page is missing the OAuth token or widget key")
	}
	return nil
}

// SignIn runs the SRP exchange for account and password. It returns nil
// when Apple signed the account in, or the two-factor challenge to answer
// with VerifyCode.
func (s *Session) SignIn(ctx context.Context, account, password string) (*SecondFactor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account = strings.TrimSpace(account)
	if account == "" || password == "" {
		return nil, ErrInvalidCredentials
	}
	if s.stage != stageBegun {
		return nil, ErrOutOfOrder
	}
	srp, err := newSRPClient(account, nil)
	if err != nil {
		return nil, err
	}
	var init struct {
		Iteration int    `json:"iteration"`
		Salt      string `json:"salt"`
		Protocol  string `json:"protocol"`
		B         string `json:"b"`
		C         string `json:"c"`
	}
	if _, err := s.api(ctx, http.MethodPost, "/appleauth/auth/signin/init", map[string]any{
		"a":           base64.StdEncoding.EncodeToString(srp.A),
		"accountName": account,
		"protocols":   srpProtocols,
	}, nil, &init); err != nil {
		return nil, err
	}
	salt, err1 := base64.StdEncoding.DecodeString(init.Salt)
	B, err2 := base64.StdEncoding.DecodeString(init.B)
	if err := errors.Join(err1, err2); err != nil || len(B) == 0 {
		return nil, fmt.Errorf("appleid: malformed sign-in challenge: %v", err)
	}
	m1, m2, err := srp.evidence(password, salt, B, init.Iteration, init.Protocol)
	if err != nil {
		return nil, err
	}

	hdr := http.Header{}
	if s.hcChallenge != "" {
		stamp, err := hashcash(ctx, s.hcChallenge, s.hcBits, s.now())
		if err != nil {
			return nil, err
		}
		hdr.Set("X-APPLE-HC", stamp)
	}
	resp, err := s.api(ctx, http.MethodPost, "/appleauth/auth/signin/complete?isRememberMeEnabled=false", map[string]any{
		"accountName": account,
		"rememberMe":  false,
		"m1":          base64.StdEncoding.EncodeToString(m1),
		"c":           init.C,
		"m2":          base64.StdEncoding.EncodeToString(m2),
	}, hdr, nil, http.StatusConflict)
	if err != nil {
		return nil, err
	}
	s.account = account
	if resp.StatusCode != http.StatusConflict {
		s.stage = stageAuthenticated
		return nil, nil
	}
	sf, err := s.loadSecondFactor(ctx, resp.Header.Get("Location"))
	if err != nil {
		return nil, err
	}
	s.second, s.stage = sf, stageSecondFactor
	return sf, nil
}

// loadSecondFactor reads the two-factor options after a 409 from
// signin/complete the way Apple's widget does (app-message.js Io, home.js
// hsa2 reducer): GET the Location path (default /auth) as text/html and
// parse the first `.boot_args` script. hasTrustedDevices enables the
// device route; twoSV.phoneNumberVerification lists the phones.
// Apple prompts the trusted devices server-side; this request only reads.
func (s *Session) loadSecondFactor(ctx context.Context, location string) (*SecondFactor, error) {
	path := "/auth"
	if strings.HasPrefix(location, "/") && !strings.HasPrefix(location, "//") {
		path = location
	}
	_, raw, err := s.send(ctx, http.MethodGet, "/appleauth"+path, nil, http.Header{"Accept": {"text/html"}})
	if err != nil {
		return nil, err
	}
	boot, err := parseTwoFactorBoot(raw)
	if err != nil {
		return nil, err
	}
	sf, err := boot.secondFactor()
	if err != nil {
		return nil, err
	}
	if pnv := boot.phoneVerification(); pnv != nil {
		s.nonFTEU = pnv.EnableNonFTEU
	}
	return sf, nil
}

var bootArgsClassRe = regexp.MustCompile(`(?s)<script[^>]*\bclass="boot_args"[^>]*>(.*?)</script>`)

type phoneVerification struct {
	TrustedPhoneNumbers []bootPhone `json:"trustedPhoneNumbers"`
	PhoneNumber         *bootPhone  `json:"phoneNumber"`
	// EnableNonFTEU makes the widget flag the phone with nonFTEU when it
	// submits a texted code (home.js "phone/verify").
	EnableNonFTEU bool `json:"enableNonFTEU"`
	SecurityCode  struct {
		Length int `json:"length"`
	} `json:"securityCode"`
}

type bootPhone struct {
	ID                 int    `json:"id"`
	NumberWithDialCode string `json:"numberWithDialCode"`
	ObfuscatedNumber   string `json:"obfuscatedNumber"`
	PushMode           string `json:"pushMode"`
}

// twoFactorBoot is the part of the 2FA page's boot data the widget reads.
type twoFactorBoot struct {
	HasTrustedDevices bool `json:"hasTrustedDevices"`
	TwoSV             struct {
		PhoneNumberVerification   *phoneVerification `json:"phoneNumberVerification"`
		TrustedDeviceVerification *struct {
			PhoneNumberVerification *phoneVerification `json:"phoneNumberVerification"`
		} `json:"trustedDeviceVerification"`
		DeviceVerification *struct {
			SecurityCode struct {
				Length int `json:"length"`
			} `json:"securityCode"`
		} `json:"deviceVerification"`
		// bridgeInitiateData is merged over twoSV by the widget.
		BridgeInitiateData *struct {
			PhoneNumberVerification *phoneVerification `json:"phoneNumberVerification"`
		} `json:"bridgeInitiateData"`
	} `json:"twoSV"`
}

func parseTwoFactorBoot(page []byte) (*twoFactorBoot, error) {
	m := bootArgsClassRe.FindSubmatch(page)
	if m == nil {
		return nil, errors.New("appleid: two-factor page has no boot data")
	}
	var args struct {
		Direct twoFactorBoot `json:"direct"`
	}
	// The widget unescapes \' before JSON.parse.
	if err := json.Unmarshal(bytes.ReplaceAll(m[1], []byte(`\'`), []byte(`'`)), &args); err != nil {
		return nil, fmt.Errorf("appleid: two-factor boot data: %w", err)
	}
	return &args.Direct, nil
}

// phoneVerification picks the phone data the widget would use:
// bridgeInitiateData over twoSV, then trustedDeviceVerification's copy.
func (b *twoFactorBoot) phoneVerification() *phoneVerification {
	tsv := &b.TwoSV
	if tsv.BridgeInitiateData != nil && tsv.BridgeInitiateData.PhoneNumberVerification != nil {
		return tsv.BridgeInitiateData.PhoneNumberVerification
	}
	if tsv.PhoneNumberVerification != nil {
		return tsv.PhoneNumberVerification
	}
	if tsv.TrustedDeviceVerification != nil {
		return tsv.TrustedDeviceVerification.PhoneNumberVerification
	}
	return nil
}

func (b *twoFactorBoot) secondFactor() (*SecondFactor, error) {
	tsv := &b.TwoSV
	pnv := b.phoneVerification()
	sf := &SecondFactor{}
	if pnv != nil {
		sf.CodeLength = pnv.SecurityCode.Length
	}
	if tsv.DeviceVerification != nil && tsv.DeviceVerification.SecurityCode.Length > 0 {
		sf.CodeLength = tsv.DeviceVerification.SecurityCode.Length
	}
	if sf.CodeLength <= 0 {
		sf.CodeLength = 6
	}

	sf.TrustedDevices = b.HasTrustedDevices
	if pnv != nil {
		for _, p := range pnv.TrustedPhoneNumbers {
			sf.Phones = append(sf.Phones, p.phone())
		}
		// phoneNumber is the number Apple texted itself; it only does so
		// when there is no device to prompt.
		if pnv.PhoneNumber != nil {
			if !sf.TrustedDevices {
				sf.SentToPhone = pnv.PhoneNumber.ID
			}
			if !slices.ContainsFunc(sf.Phones, func(p Phone) bool { return p.ID == pnv.PhoneNumber.ID }) {
				sf.Phones = append(sf.Phones, pnv.PhoneNumber.phone())
			}
		}
	}
	if !sf.TrustedDevices && len(sf.Phones) == 0 {
		// Security keys only (twoSV.fsaVerification) or no route Sona can
		// serve.
		return nil, ErrUnsupported2FA
	}
	return sf, nil
}

func (p bootPhone) phone() Phone {
	num := p.NumberWithDialCode
	if num == "" {
		num = p.ObfuscatedNumber
	}
	mode := p.PushMode
	if mode == "" {
		mode = "sms"
	}
	return Phone{ID: p.ID, Number: num, Mode: mode}
}

// route reports whether a code may go through phoneID (0 = trusted
// devices) for the pending challenge: device codes only for accounts with
// trusted devices; texts and calls ("sms", "voice") for any account, to
// one of its listed phones. It makes no request.
func (s *Session) route(phoneID int, mode string) error {
	if s.stage != stageSecondFactor || s.second == nil {
		return ErrOutOfOrder
	}
	if phoneID == 0 {
		if !s.second.TrustedDevices {
			return ErrOutOfOrder
		}
		return nil
	}
	if mode != "sms" && mode != "voice" {
		return ErrOutOfOrder
	}
	for _, p := range s.second.Phones {
		if p.ID == phoneID {
			return nil
		}
	}
	return ErrOutOfOrder
}

// SendCodeToPhone asks Apple to text (mode "sms") or call ("voice") one of
// the challenge's phones; see route.
func (s *Session) SendCodeToPhone(ctx context.Context, phoneID int, mode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if mode == "" {
		mode = "sms"
	}
	if phoneID == 0 {
		return ErrOutOfOrder
	}
	if err := s.route(phoneID, mode); err != nil {
		return err
	}
	_, err := s.api(ctx, http.MethodPut, "/appleauth/auth/verify/phone", map[string]any{
		"phoneNumber": map[string]int{"id": phoneID},
		"mode":        mode,
	}, nil, nil)
	return err
}

// ResendDeviceCode prompts the trusted devices again.
func (s *Session) ResendDeviceCode(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.route(0, ""); err != nil {
		return err
	}
	_, err := s.api(ctx, http.MethodPut, "/appleauth/auth/verify/trusteddevice/securitycode", nil, nil, nil)
	return err
}

// VerifyCode submits a two-factor code: from a trusted device when phoneID
// is 0, otherwise the code texted to (or spoken on) phoneID. A wrong code
// returns an error wrapping ErrIncorrectCode and leaves the challenge
// open; so does ErrOutOfOrder for a route the account does not have.
func (s *Session) VerifyCode(ctx context.Context, code string, phoneID int, mode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if phoneID != 0 && mode == "" {
		mode = "sms"
	}
	if err := s.route(phoneID, mode); err != nil {
		return err
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return ErrIncorrectCode
	}
	var err error
	if phoneID == 0 {
		_, err = s.api(ctx, http.MethodPost, "/appleauth/auth/verify/trusteddevice/securitycode", map[string]any{
			"securityCode": map[string]string{"code": code},
		}, nil, nil)
	} else {
		phone := map[string]any{"id": phoneID}
		if s.nonFTEU {
			phone["nonFTEU"] = true
		}
		_, err = s.api(ctx, http.MethodPost, "/appleauth/auth/verify/phone/securitycode", map[string]any{
			"phoneNumber":  phone,
			"securityCode": map[string]string{"code": code},
			"mode":         mode,
		}, nil, nil)
	}
	if err != nil {
		return err
	}
	// The widget's "Trust this browser?" step; it finalizes the session.
	// The trust token lands in this Session's jar and dies with it.
	if _, err := s.api(ctx, http.MethodGet, "/appleauth/auth/2sv/trust", nil, nil, nil); err != nil {
		return err
	}
	s.stage = stageAuthenticated
	return nil
}

// MusicUserToken completes Apple's Apple Music authorization for the
// signed-in account (the widget's callback form, validateMusicKitToken,
// then the consent page's getMusicUserToken call) and returns the token.
func (s *Session) MusicUserToken(ctx context.Context) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stage != stageAuthenticated {
		return Result{}, ErrOutOfOrder
	}
	form := url.Values{"rememberMe": {"false"}, "oAuthToken": {s.oauthToken}, "scnt": {s.formSCNT}}
	resp, _, err := s.do(ctx, http.MethodPost, s.idms+"/auth", strings.NewReader(form.Encode()), http.Header{
		"Content-Type": {"application/x-www-form-urlencoded"},
		"Origin":       {s.idms},
		"Referer":      {s.pageURL},
	})
	if err != nil {
		return Result{}, err
	}
	loc, ok := authorizeLocation(resp)
	if !ok {
		return Result{}, fmt.Errorf("appleid: Apple did not return to Apple Music authorization (%s)", resp.Status)
	}
	q := loc.Query()
	if err := authorizeError(loc); err != nil {
		return Result{}, err
	}
	s.stage = stageDone
	if mut := q.Get("musicUserToken"); mut != "" {
		return Result{MusicUserToken: mut, Restricted: truthy(q.Get("itre"))}, nil
	}
	lite := q.Get("liteSessionId")
	if lite == "" {
		return Result{}, errors.New("appleid: authorization returned no session")
	}
	tokenURL := s.tokenURL
	if tokenURL == "" {
		tokenURL = MusicTokenURL
		if pod := q.Get("pod"); podRe.MatchString(pod) {
			tokenURL = fmt.Sprintf(podMusicTokenURL, pod)
		}
	}
	u := tokenURL + "?" + url.Values{"liteSessionId": {lite}, "jwtToken": {s.devToken}}.Encode()
	resp, body, err := s.do(ctx, http.MethodGet, u, nil, http.Header{
		"Accept":  {"application/json"},
		"Origin":  {AuthorizeOrigin},
		"Referer": {AuthorizeOrigin + "/"},
	})
	if err != nil {
		return Result{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, &ServiceError{Status: resp.StatusCode, Message: "Apple Music token request failed"}
	}
	var tok struct {
		MusicUserToken string          `json:"musicUserToken"`
		Itre           json.RawMessage `json:"itre"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.MusicUserToken == "" {
		return Result{}, errors.New("appleid: Apple Music token response had no token")
	}
	return Result{MusicUserToken: tok.MusicUserToken, Restricted: truthy(strings.Trim(string(tok.Itre), `"`))}, nil
}

// SecondFactor returns the pending challenge, or nil.
func (s *Session) SecondFactor() *SecondFactor {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stage != stageSecondFactor {
		return nil
	}
	return s.second
}

// Close drops the session's cookies.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hc.Jar, _ = cookiejar.New(nil)
	s.stage = stageDone
}

var podRe = regexp.MustCompile(`^\d{1,3}$`)

// api calls an idmsa.apple.com/appleauth JSON endpoint with the widget's
// headers. Statuses in ok (besides 2xx) are not errors. out may be nil.
func (s *Session) api(ctx context.Context, method, path string, in any, extra http.Header, out any, ok ...int) (*http.Response, error) {
	resp, raw, err := s.send(ctx, method, path, in, extra, ok...)
	if err != nil {
		return resp, err
	}
	if out != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp, fmt.Errorf("appleid: %s: %w", path, err)
		}
	}
	return resp, nil
}

// send makes an /appleauth request with the widget's headers (extra
// overrides them) and returns the raw body.
func (s *Session) send(ctx context.Context, method, path string, in any, extra http.Header, ok ...int) (*http.Response, []byte, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, nil, err
		}
		body = bytes.NewReader(b)
	}
	h := http.Header{
		"Accept":                       {"application/json, text/javascript, */*; q=0.01"},
		"Origin":                       {s.idms},
		"Referer":                      {s.idms + "/"},
		"X-Requested-With":             {"XMLHttpRequest"},
		"X-Apple-Widget-Key":           {s.widgetKey},
		"X-Apple-Frame-Id":             {s.frameID},
		"X-Apple-Locale":               {s.locale},
		"X-Apple-OAuth-Client-Id":      {s.widgetKey},
		"X-Apple-OAuth-Client-Type":    {"firstPartyAuth"},
		"X-Apple-OAuth-Redirect-URI":   {s.idms},
		"X-Apple-OAuth-Response-Mode":  {"web_message"},
		"X-Apple-OAuth-Response-Type":  {"code"},
		"X-Apple-OAuth-State":          {url.QueryEscape(s.frameID)},
		"X-Apple-Alt-TokenCookie-Flg":  {"true"},
		"X-Apple-Requestor-OAuthToken": {s.oauthToken},
	}
	if in != nil {
		h.Set("Content-Type", "application/json")
	}
	if s.appID != "" {
		h.Set("X-Apple-App-Id", s.appID)
	}
	if s.scnt != "" {
		h.Set("scnt", s.scnt)
	}
	if s.sessionID != "" {
		h.Set("X-Apple-ID-Session-Id", s.sessionID)
	}
	if s.authAttrs != "" {
		h.Set("X-Apple-Auth-Attributes", s.authAttrs)
	}
	for k, v := range extra {
		h[k] = v
	}
	resp, raw, err := s.do(ctx, method, s.idms+path, body, h)
	if err != nil {
		return nil, nil, err
	}
	s.track(resp)
	if resp.StatusCode/100 != 2 && !containsInt(ok, resp.StatusCode) {
		return resp, raw, serviceError(resp.StatusCode, raw)
	}
	return resp, raw, nil
}

func (s *Session) do(ctx context.Context, method, u string, body io.Reader, h http.Header) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, nil, err
	}
	for k, v := range h {
		req.Header[k] = v
	}
	req.Header.Set("User-Agent", s.ua)
	if req.Header.Get("Accept-Language") == "" {
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("appleid: %s %s: %w", method, req.URL.Host+req.URL.Path, scrub(err))
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, nil, fmt.Errorf("appleid: read %s: %w", req.URL.Path, err)
	}
	return resp, b, nil
}

// scrub drops the URL (which can carry the developer token or a session
// id) from transport errors.
func scrub(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// track keeps the rolling headers Apple expects echoed on the next call.
func (s *Session) track(resp *http.Response) {
	if v := resp.Header.Get("scnt"); v != "" {
		s.scnt = v
	}
	if v := resp.Header.Get("X-Apple-ID-Session-Id"); v != "" {
		s.sessionID = v
	}
	if v := resp.Header.Get("X-Apple-Auth-Attributes"); v != "" {
		s.authAttrs = v
	}
	if v := resp.Header.Get("X-Apple-HC-Challenge"); v != "" {
		s.hcChallenge = v
		if n, err := strconv.Atoi(resp.Header.Get("X-Apple-HC-Bits")); err == nil {
			s.hcBits = n
		}
	}
}

func serviceError(status int, body []byte) error {
	var r struct {
		ServiceErrors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"serviceErrors"`
		ValidationErrors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"validationErrors"`
	}
	_ = json.Unmarshal(body, &r)
	e := &ServiceError{Status: status}
	if len(r.ServiceErrors) == 0 {
		r.ServiceErrors = r.ValidationErrors
	}
	if len(r.ServiceErrors) > 0 {
		e.Code, e.Message = r.ServiceErrors[0].Code, r.ServiceErrors[0].Message
	}
	switch {
	case e.Code == "-20101" || e.Code == "-20751" || (status == http.StatusUnauthorized && e.Code == ""):
		e.kind = ErrInvalidCredentials
	case e.Code == "-21669" || e.Code == "-21546":
		e.kind = ErrIncorrectCode
	case e.Code == "-20209" || e.Code == "-20283" || e.Code == "-24761" || status == http.StatusForbidden:
		e.kind = ErrLocked
	case status == http.StatusPreconditionFailed:
		e.kind = ErrActionRequired
	}
	return e
}

func containsInt(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func isAuthorizeRedirect(u *url.URL) bool {
	a, _ := url.Parse(AuthorizeOrigin)
	return u.Host == a.Host
}

func authorizeLocation(resp *http.Response) (*url.URL, bool) {
	if resp.StatusCode/100 != 3 {
		return nil, false
	}
	loc, err := resp.Location()
	if err != nil || !isAuthorizeRedirect(loc) {
		return nil, false
	}
	return loc, true
}

// authorizeError maps the error routes of authorize.music.apple.com
// (index.js route guards Ea, Ye, Pa and the /error/* routes).
func authorizeError(loc *url.URL) error {
	q := loc.Query()
	switch {
	case strings.HasPrefix(loc.Path, "/error/no-subscription"),
		q.Get("error") == "ERROR_NO_DSID", q.Get("isAppleMusicSubscriber") == "false":
		return ErrNoSubscription
	case strings.HasPrefix(loc.Path, "/error/unsupported-country"), q.Get("supported") == "0":
		return ErrUnsupportedCountry
	case q.Get("error") != "":
		return &ServiceError{Status: http.StatusOK, Message: q.Get("error")}
	case strings.HasPrefix(loc.Path, "/error"):
		return &ServiceError{Status: http.StatusOK, Message: "Apple Music authorization failed"}
	}
	return nil
}

func truthy(v string) bool { return v == "1" || v == "true" }

// frameID mimics the widget's iframe id: auth-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxx.
func frameID() string {
	part := func(n int) string {
		const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
		b := make([]byte, n)
		for i := range b {
			v, _ := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
			b[i] = digits[v.Int64()]
		}
		return string(b)
	}
	return "auth-" + part(8) + "-" + part(4) + "-" + part(4) + "-" + part(4) + "-" + part(8)
}
