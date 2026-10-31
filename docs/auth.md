# Sona Authentication Design

Status: decided (web player developer token; native in-app sign-in as the primary MUT flow, loopback and token paste as fallbacks) and implemented in `internal/auth/devtoken`, `internal/auth/appleid`, `internal/auth/mut`, `internal/auth/store`, `internal/auth` (`Manager`), and `cmd/authspike`. Items marked **[verified]** were observed on 2026-10-29 against live Apple endpoints. Items marked **[unverified]** still need an interactive Apple ID login.

## 1. What Must Be Obtained

| Credential | Format | Lifetime | Who needs it |
| --- | --- | --- | --- |
| Developer token | ES256 JWT | signed: ≤ 6 months (`exp - iat ≤ 15777000`); web player: ~70 days **[verified]** | every Apple Music API call; MusicKit JS `configure()` |
| Music User Token (MUT) | opaque string | months; revocable by Apple/user `[INFERENCE]` | every `/v1/me/*` call; MusicKit playback |
| Storefront | 2-letter code | stable per account | catalog paths `/v1/catalog/{sf}/...` |

Request headers for personalized calls: `Authorization: Bearer <devToken>` and `Media-User-Token: <mut>`.

## 2. Why The MusicKit Popup Model Doesn't Fit

Clients built on MusicKit JS inside a browser shell (Electron, for example) usually sign in like this:

- The developer token comes from a server of their own or from Apple's web player bundle.
- The MUT comes from `MusicKit.getInstance().authorize()`. That opens a popup named `apple-music-service-view` to `authorize.music.apple.com/woa`, and the popup returns the token to `window.opener` over JSON-RPC `postMessage`.
- The shell explicitly allows that popup and rewrites `Origin`/`Referer` on every `*.apple.com` request.
- Persistence is MusicKit's own `localStorage` (`music.ampwebplay.media-user-token`), with no keychain.

That model depends on three things an embedded WebKitGTK shell does not reliably give us:

1. `window.open` producing a related webview with a working `window.opener`.
2. Request header rewriting (WebKitGTK has no UI-process API for this).
3. Tokens living in the webview's storage instead of the backend.

Sona's design removes all three dependencies.

## 3. Developer Token: Web Player Token (decided)

Sona uses the token Apple's own web player ships. This is a deliberate choice: it costs nothing and needs no token server. It is also ToS-gray and can break whenever Apple changes the bundle. If that happens, the fallbacks are a self-signed `.p8` token (Apple Developer Program) or a token endpoint Sona operates; both fit the same interface.

Implementation: `internal/auth/devtoken/webplayer.go` (`WebPlayer.Token`, `Invalidate`, `ParseClaims`).

- **[verified]** `https://music.apple.com/us/browse` references one entry bundle, `/assets/index~<hash>.js`. That bundle assigns the JWT to a minified variable that it passes as `developerToken:` into MusicKit configure.
- Extraction: fetch the HTML, take `/assets/index~*.js`, regex `"(eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+)"`, and accept only claims with `iss == "AMPWebPlay"`.
- **[verified]** Claims: `{"iss":"AMPWebPlay","iat":1792780534,"exp":1798828534,"root_https_origin":["apple.com"]}`, header `kid: WebPlayKid` (70-day lifetime).
- **[verified]** On both `api.music.apple.com` and `amp-api.music.apple.com`, `GET /v1/catalog/gb/search` returns **401 without** `Origin` and **200 with** `Origin: https://music.apple.com` + `Referer: https://music.apple.com/`. `internal/applemusic` always sets both.
- Refresh: cached in memory, refetched when fewer than 7 days remain or after a 401 (`Client.Get` invalidates and retries once).
- The playback engine needs no origin spoofing, because its page is served at `https://music.apple.com/sona-engine/` (§6).

### 3.1 MUT Compatibility

**[verified]** The MUT the user supplied in `.env` is accepted by `/v1/me/storefront` with the web player token, and MusicKit in the engine reports `authorized: true` with it. A MUT minted through Sona's own flow is requested with this same token.

## 4. MUT Acquisition: Loopback Redirect Flow And Native Sign-In

### 4.1 Evidence That The Flow Exists **[verified from Apple's shipped JS]**

- `authorize.music.apple.com/assets/index.*.js`, query handler: `a` (base64 third-party info) goes to `sessionStorage.ai`. The ReferrerUrl stored in `sessionStorage.ac` is `document.referrer`, **replaced by the `referrer` query param when both share an origin** (`p&&c.origin===p.origin&&(o=c.href)`).
- On successful sign-in with **no opener**, the page stores `sessionStorage.ap = btoa(JSON.stringify({cid, itre, musicUserToken}))`. It then runs `window.location.assign(`${ReferrerUrl}#${ap}`)`.
- MusicKit JS v3 consumes exactly this format in `_processLocationHash`, using the regex `^#([a-zA-Z0-9+/]{200,}={0,2})$`. It parses `{itre, musicUserToken, cid}` and calls `history.replaceState` to strip the hash. So the redirect flow is an official code path, not a scrape.
- MusicKit builds the URL as `https://authorize.music.apple.com/woa?a=<btoa(thirdPartyInfo)>&referrer=<location.href>&app=music&p=subscribe`. `thirdPartyInfo = {thirdPartyIconURL, thirdPartyName, thirdPartyParameters, thirdPartyToken: <devToken>}`.
- **[verified]** With the web player token and `referrer=http://127.0.0.1:<port>/auth/<nonce>/callback`, reached from Sona's loopback start page in headless Chrome, the authorize page reaches the real Apple ID sign-in (`<title>Sign In - Apple</title>`, a top-level redirect to `idmsa.apple.com/IDMSWebAuth/auth`, `appReturnUrl=…/commerce/account/validateMusicKitToken`).
- **[verified]** Apple's authorize page stored `sessionStorage.ac = "http://127.0.0.1:<port>/auth/<nonce>/callback"` (with `document.referrer` = the start URL), so after sign-in it redirects to Sona's callback.
- **[verified]** Simulating Apple's final hop (navigating to `callback#<btoa({cid,itre,musicUserToken})>` with the real MUT) runs the callback page JS, posts to `/complete`, and `authspike` prints `signed in: storefront=gb (United Kingdom)`.
- **[unverified]** Apple's own post-sign-in redirect to a loopback referrer. Run `go run ./cmd/authspike -save .env` and sign in to close this.

### 4.2 Sequence

```mermaid
sequenceDiagram
  participant UI as React UI
  participant Go as Go AuthService
  participant LB as Loopback server (127.0.0.1:random)
  participant B as Browser surface (system browser or Wails window)
  participant AZ as authorize.music.apple.com
  participant ID as idmsa.apple.com
  participant API as api.music.apple.com

  UI->>Go: SignIn()
  Go->>Go: devToken = webplayer.Token()
  Go->>LB: start, nonce N (128-bit), 10 min deadline
  Go->>B: open http://127.0.0.1:P/auth/N/start
  B->>LB: GET /auth/N/start
  LB-->>B: page: Referrer-Policy unsafe-url, location.assign(authorizeURL)
  B->>AZ: GET /woa?a=b64(info)&referrer=http://127.0.0.1:P/auth/N/callback&app=music&p=subscribe
  AZ->>ID: Apple ID sign-in (+2FA)
  ID-->>AZ: validateMusicKitToken
  AZ-->>B: location.assign(callback + "#" + b64({cid,itre,musicUserToken}))
  B->>LB: GET /auth/N/callback (fragment stays client-side)
  LB-->>B: page JS reads location.hash, history.replaceState, POST /auth/N/complete
  B->>LB: POST /auth/N/complete {fragment}
  LB->>Go: decode, single-use consume N
  Go->>API: GET /v1/me/storefront (Bearer + Media-User-Token)
  API-->>Go: 200 {data:[{id:"gb"}]}
  Go->>Go: store MUT (keyring), storefront (DB)
  Go-->>UI: event auth:changed {state: signed_in}
  LB-->>B: "You can close this window" (Wails window closes itself)
```

### 4.3 Surfaces

1. **Native form (primary)**: Sona's own account/password/code form; Go speaks Apple's sign-in protocol directly. See §4.7.
2. **System browser (fallback)**: Wails `Browser.OpenURL`, i.e. the XDG portal / `xdg-open`. The user gets their real browser's password manager, passkeys, existing Apple session, and 2FA UX. WebKitGTK quirks are irrelevant here. Use it for accounts the native form cannot serve (security keys only, account repair prompts).
3. **In-app Wails window** (superseded by the native form, not planned): the same start URL loaded in a dedicated window. If Apple's page rejects the WebKitGTK UA, set a Safari-like UA on that window only **[unverified]**.
4. **Manual token paste (escape hatch)**: the user copies the `media-user-token` cookie from `music.apple.com` in any signed-in browser. `SignInWithToken` validates it with `/v1/me/storefront` before storing. **[verified]** with the token in `.env`.

### 4.4 Referrer Robustness

- WebKit/Safari ITP and some browser privacy modes trim cross-site referrers to the origin. The flow still works because the `referrer` param is honoured whenever its origin equals `document.referrer`'s origin, and the loopback origin is unique per attempt (random port).
- If the referrer is stripped entirely (`no-referrer` policies or extensions), Apple has nowhere to redirect. Detection: the start page was hit but no callback arrives within the deadline. The UI then offers the other surface or token paste.
- The start page sets both the `Referrer-Policy: unsafe-url` header and `<meta name="referrer" content="unsafe-url">`.

### 4.5 Loopback Server Security

- Bind `127.0.0.1` only, on a random port. One server instance per sign-in attempt, shut down on completion, cancel, or deadline.
- Paths are prefixed with a 128-bit random nonce. Unknown paths return 404. Each nonce is consumed once.
- Reject requests whose `Host` is not `127.0.0.1:<port>` (DNS rebinding defense).
- `POST /complete` requires `Origin: http://127.0.0.1:<port>` and `Content-Type: application/json`, with a body cap of 16 KiB.
- Every response sets `Cache-Control: no-store` and a CSP of `default-src 'none'; script-src 'nonce-…'; connect-src 'self'`.
- Never log the fragment, the MUT, or the dev token. `slog` uses a redacting `LogValuer` on token types.

### 4.6 Failure Modes Handled

| Situation | Signal | Sona behaviour |
| --- | --- | --- |
| No Apple Music subscription | authorize routes to `ErrorNoSubscription` (seen in Apple JS); no callback | UI: "Apple Music subscription required"; offer retry |
| Unsupported country | route `error-unsupported-country` | same, with storefront message |
| User closes the browser tab | deadline with no callback | state back to `signed_out`, toast |
| Restricted account | `itre == "1"` in fragment | store the flag; hide explicit content |
| Fragment decodes but `/v1/me/storefront` is 401/403 | validation failure | discard the MUT; report the error and offer retry |
| Second `SignIn` while one is pending | existing attempt | cancel the old attempt (close its server), start a new one |

### 4.7 Native Sign-In (`internal/auth/appleid`)

Sona collects the Apple Account name, password and two-factor code in its own UI. Go replays the exact requests Apple's sign-in widget and the Apple Music authorize page make, so no browser or webview is involved. The password is used for one SRP exchange, never stored, never logged; each attempt has its own cookie jar, discarded when it ends.

Sources, all fetched 2026-10-29 and read with REA (`analyze-javascript-application`) plus manual reading of the prettified bundles:

| Step | Apple source |
| --- | --- |
| `/woa` → `activate.apple.com/mkauth?jwtToken=<dev>&LogoURL=` | `authorize.music.apple.com/assets/index.db65ee7b.js` (`Ia`, `Fe`) |
| mkauth → `idmsa.apple.com/IDMSWebAuth/auth?oauth_token=…` (boot args: `widgetKey`, `oauth_token`, `appReturnUrl=…/validateMusicKitToken?Pod=..&liteSessionId=..`, `appId` 165; hidden `callBackForm` with `oAuthToken`, `scnt`) | observed response |
| Widget frame `/appleauth/auth/authorize/signin?frame_id&client_id=<widgetKey>&redirect_uri=https://idmsa.apple.com&response_type=code&response_mode=web_message&state&authVersion=latest` (first-party OAuth because `enableTiburonInd`) issues `scnt`, `X-Apple-ID-Session-Id`, `X-Apple-Auth-Attributes`, `X-Apple-HC-Challenge`/`-Bits` | `authService.latest.min.js`, `oauth2WidgetLoginBoot.js` |
| `POST /appleauth/auth/signin/init {a, accountName, protocols:["s2k","s2k_fo"]}` → `{iteration, salt, protocol, b, c}` | `app-message.js` (`vn`, worker `initData` handler) |
| `POST /appleauth/auth/signin/complete?isRememberMeEnabled=false {accountName, rememberMe, m1, c, m2}` + `X-APPLE-HC`; 200 signed in, 409 two-factor | `app-message.js` (`bn.prototype.signIn`) |
| SRP-6a: RFC 5054 2048-bit group, SHA-256, `k=H(N‖PAD(g))`, `x=H(s‖H(":"‖P))` (empty identity), `P=PBKDF2-SHA256(SHA256(pw) or hex(SHA256(pw)) for s2k_fo, s, i, 32)`, `u=H(PAD(A)‖PAD(B))`, `K=H(PAD(S))`, `M1=H(H(N)⊕H(PAD(g)) ‖ H(lower(account)) ‖ s ‖ A ‖ B ‖ K)`, `M2=H(A‖M1‖K)` | `webSRPClientWorker.js` |
| Hashcash `1:<bits>:<UTC yyyymmddhhmmss>:<challenge>::<n>`, SHA-1 with ≥ bits leading zero bits | `generateHashcash.js`, `app-message.js` (`separator:"::"`) |
| 2FA options: `GET /appleauth` + (409 `Location` or `/auth`) with `Accept: text/html`; the first `<script class="boot_args">` gives `direct.hasTrustedDevices` and `direct.twoSV.phoneNumberVerification` (`trustedPhoneNumbers`, `phoneNumber`, `securityCode.length`; `bridgeInitiateData` overrides `twoSV`, `trustedDeviceVerification.phoneNumberVerification` is the fallback). The widget routes `hasTrustedDevices` to `verifyDevice`, else to the phone sections | `app-message.js` (`Io`, `te.prototype.next`), `home-c702d19e62c250fcaaa7.js` (hsa2 reducer, `F`) |
| 2FA codes: `POST …/verify/trusteddevice/securitycode {securityCode:{code}}`, `PUT …/verify/trusteddevice/securitycode` (resend), `PUT …/verify/phone {phoneNumber:{id},mode}`, `POST …/verify/phone/securitycode`, `GET …/2sv/trust` | `home-c702d19e62c250fcaaa7.js` |
| Widget success → `POST https://idmsa.apple.com/auth` (`callBackForm`) → `validateMusicKitToken` → 302 to `authorize.music.apple.com/?liteSessionId&pod&appleId&…` | `oauth2WidgetLoginBoot.js` (`onAuthSuccess`) |
| `GET https://p<pod>-buy.itunes.apple.com/commerce/account/getMusicUserToken?liteSessionId&jwtToken=<dev>` → `{musicUserToken, itre}` (the consent page's Approve button, no cookies) | `index.db65ee7b.js` (`Xr`, `tn`) |

Every `/appleauth` request carries the widget's headers: `X-Apple-Widget-Key`, `X-Apple-OAuth-{Client-Id,Client-Type=firstPartyAuth,Redirect-URI,Response-Mode,Response-Type,State}`, `X-Apple-Frame-Id`, `X-Apple-App-Id`, `X-Apple-Locale`, `X-Apple-Alt-TokenCookie-Flg`, `X-Apple-Requestor-OAuthToken`, and the rolling `scnt` / `X-Apple-ID-Session-Id` / `X-Apple-Auth-Attributes` from the previous response. Sona does not send the browser fingerprint `X-Apple-I-FD-Client-Info` [INFERENCE: Apple may weigh its absence as risk and ask for 2FA more often].

Verification:

- **[verified]** `internal/auth/appleid/testdata/srp_vectors.json` was produced by running Apple's `webSRPClientWorker.js` under Node with a fixed private value; the Go client reproduces A, M1 and M2 byte for byte for both `s2k` and `s2k_fo` (including a zero-led salt, mixed-case account and non-ASCII password).
- **[verified]** Live: mkauth → IDMSWebAuth → widget frame → `signin/init` 200 → `signin/complete` (with hashcash) 401 `-20101` for a nonexistent account, i.e. Apple accepted the request framing and rejected only the credentials.
- **[verified]** 2026-10-30 (app log): a real account completed native sign-in through an SMS code to `getMusicUserToken` and a stored MUT, and the browser fallback completed too.
- **[verified]** 2026-10-30 13:10 (app log): a real account with Apple devices got `hasTrustedDevices` from the HTML boot data (`trustedDevices=true phones=0`), the form asked only for the device code, and `verify/trusteddevice/securitycode` → `getMusicUserToken` signed in.
- **[unverified]** The no-device paths (texted `phoneNumber`, phone list without auto-send, text/call switch, `nonFTEU`) against a real account; covered only by `appleidtest`. Needs an Apple Account with no trusted device.
- `appleidtest` is a fake that checks SRP with a real server-side computation, the hashcash stamp and header rotation; `auth` and `appleid` tests run the whole flow against it.

Manager API (`internal/auth/native.go`): `SignInWithPassword(account, password)` returns `nil` (signed in) or the `*appleid.SecondFactor`. `SubmitSecurityCode(code, phoneID, mode)` (phoneID 0 = trusted device), `SendCodeToPhone(phoneID, mode)` and `ResendSecurityCode()` act on the pending attempt. While waiting, the state is `signing_in` with `Status.SecondFactor` set.

Two-factor routing: Apple prompts the trusted devices server-side when the password is accepted; Apple's own widget sends no request that triggers or withholds that prompt, it only reads the options and shows the device-code screen [INFERENCE from the widget code and the observed prompt; not isolated per request]. An earlier version read `GET /appleauth/auth` as JSON and guessed `trustedDeviceCount`/`noTrustedDevices`; on a real account (2026-10-30, log `trustedDevices=false phones=1`) that missed the devices.

Sona offers every route the account has. The code step always shows a three-way switcher: **Apple device**, **Text message**, **Phone call**. Apple device is disabled (with "No Apple device is signed in to this account…") when `hasTrustedDevices` is false; text and call are disabled when the account lists no phone. Picking text or call with one number asks Apple at once; with several, the number picker opens with that method preselected. Picking Apple device again returns to the device code with no request (Apple already prompted the devices).

| Boot data | `SecondFactor` | Opens on |
| --- | --- | --- |
| `hasTrustedDevices` | `TrustedDevices`, `Phones` from `trustedPhoneNumbers`, `SentToPhone` 0 | device code; text/call one click away |
| no devices, `phoneNumberVerification.phoneNumber` set | `SentToPhone` = that phone (Apple already texted it) | code entry for that number |
| no devices, `trustedPhoneNumbers` only | `Phones`, `SentToPhone` 0 | number picker. [INFERENCE] Apple may already have texted a single number; choosing it sends one more code, which is harmless |
| No phones and no devices, or security keys only | `ErrUnsupported2FA` | browser sign-in |

`appleid.Session` checks the route before any request: device codes and `ResendDeviceCode` need trusted devices; `SendCodeToPhone` and phone codes need a listed phone and mode `sms` or `voice`. Anything else returns `ErrOutOfOrder` and the attempt stays open. When `phoneNumberVerification.enableNonFTEU` is set, a texted code is submitted with `phoneNumber.nonFTEU: true`, as the widget does (`home.js` "phone/verify"). Apple's SMS-consent interstitial and SMS speed bump (`smsSpeedbumpEnabled`) are not reproduced.

| Situation | Signal | Sona behaviour |
| --- | --- | --- |
| Wrong password / unknown account | `signin/complete` 401 `-20101` | attempt ends, prior state returns, Apple's message in `LastError` |
| Wrong code | 400 `-21669` | attempt stays open, `LastError` "Incorrect verification code." |
| Too many codes / send failure | Apple service error | attempt stays open, Apple's message in `LastError` |
| Locked account | 403 or `-20209`/`-20283`/`-24761` | attempt ends; offer browser sign-in |
| Account repair or terms (`412`) | `ErrActionRequired` | attempt ends; point to account.apple.com or browser sign-in |
| Security keys only (`twoSV.fsaVerification`) or no 2FA route | `ErrUnsupported2FA` | attempt ends; offer browser sign-in |
| No subscription / unsupported country | authorize redirect `isAppleMusicSubscriber=false`, `ERROR_NO_DSID`, `/error/no-subscription`, `supported=0` | attempt ends with the matching message |
| No code before `SignInTimeout` | deadline | `signed_out` (or prior state), "sign-in timed out" |
| `CancelSignIn`, `SignOut`, or a new attempt | — | pending session dropped; later code submissions return `ErrOutOfOrder` |

## 5. Lifecycle And State Machine

Implemented by `auth.Manager` (`internal/auth/manager.go`). Every transition is published through `Manager.OnChange` (the `auth:changed` payload is `auth.Status`).

```mermaid
stateDiagram-v2
  [*] --> loading
  loading --> signed_out: no MUT in store
  loading --> signed_in: MUT + cached account (validate in background)
  loading --> validating: MUT, no cached account
  loading --> error: store unreadable (e.g. keyring locked)
  validating --> signed_in: /v1/me/storefront 200
  signed_in --> needs_reauth: MUT rejected (once)
  validating --> needs_reauth: MUT rejected
  signed_in --> error: developer token rejected
  signed_out --> signing_in: SignIn() / SignInWithPassword()
  signing_in --> validating: callback received / native MUT minted
  signing_in --> signing_in: SecondFactor pending (wrong code, resend, SMS)
  signed_out --> validating: SignInWithToken()
  validating --> signed_in: stored
  signing_in --> signed_out: cancel / deadline / error (back to prior state)
  needs_reauth --> signing_in: SignIn() / SignInWithPassword()
  signed_in --> signed_out: SignOut()
```

- Offline-first: `Start` reads only the store and the cached account, so cold start never waits for the network. Network and 5xx errors during validation keep the MUT, set `Status.Offline`, and retry with exponential backoff (2 s to 5 min). Offline never means `needs_reauth`.
- A failed sign-in attempt returns to the state before it. A second `SignIn` (or `SignInWithToken`) cancels the pending attempt and closes its loopback server.

401/403 triage: Apple returns 401 for a bad dev token and 401/403 for a bad MUT, so the status alone cannot tell them apart. **[verified]** A bogus MUT on `/v1/me/storefront` returns `403 {"code":"40300","detail":"Invalid authentication"}`.

1. On 401 from any endpoint: `WebPlayer.Invalidate()`, refetch the dev token, retry once (`applemusic.Client.Do`).
2. Request paths pass a 401/403 that survived that retry to `Manager.ReportUnauthorized`. The manager re-checks `/v1/me/storefront`; if that still fails, it probes `/v1/storefronts/{sf}` without the MUT. Catalog OK means the MUT is bad (`needs_reauth`); catalog 401/403 means the developer token is rejected (`error`, "developer token rejected"); network errors change nothing.
3. Never loop. Each request retries at most once, one probe runs at a time with a cooldown between probes, and `needs_reauth` is entered once: later reports are ignored until a new sign-in. In `needs_reauth`, `UserToken()` returns "" so catalog requests keep working.

The UI renders from SQLite in `loading`/`validating` and while `Status.Offline` is set.

## 6. Credentials And Playback **[verified]**

- Playback runs in Sona's own engine (`internal/playback/native`, `docs/architecture.md` §4). Credentials never leave the Go parent process: every playback request (`webPlayback`, `widevineCert`, `acquireWebPlaybackLicense`, radio `play/assets` and licence, play-activity reports) goes through `applemusic.Client`, which adds the developer token, `Media-User-Token` and `Origin: https://music.apple.com` per request, reading the MUT from `Manager.UserToken()`.
- The CDM host child process receives only certificates, licence responses and encrypted media bytes over its stdin/stdout pipe; it never sees a token and makes no network requests.
- A 401/403 from a playback request surfaces as `applemusic.ErrUnauthorized`: the engine emits `auth`, the coordinator pauses with `AuthRequired`, and the services layer passes the error to `Manager.ReportUnauthorized` (§5 triage).
- Sign-out stops playback, clears the queue and wipes user data (`db.WipeUserData`); there is no engine profile to wipe. Undelivered play-activity events are stored without credentials (tokens are added at send time).

## 7. Storage

- `internal/auth/store`: `github.com/zalando/go-keyring`, service `sona`, key `mut`, value JSON `{token, obtained_at, restricted}`. The dev token is not stored, because it is refetched from the web player. `store.Open` probes the Secret Service by reading the real entry; **[verified]** on the dev host (Hyprland) the Secret Service is available.
- Secret Service unavailable (no gnome-keyring/kwallet): fall back to `$XDG_CONFIG_HOME/sona/credentials.json`, mode 0600 (written atomically), owned by the user. `Store.Backend()` reports `file (unencrypted)`, Settings and Diagnostics show an explicit "credentials stored unencrypted" warning, and `Open` logs a warning. Never fall back silently. Once the Secret Service is reachable again, the file is moved into it and deleted.
- Other secrets use the same store through `store.OpenSecret(configDir, key)`: one keyring entry per key under service `sona` (`lastfm`: the Last.fm session key; `listenbrainz`: the ListenBrainz user token), the same Secret Service probe, and a 0600 fallback file `credentials-<key>` that moves into the keyring once it is reachable. Settings → Connections shows the "stored unencrypted" warning for them.
- Apple sign-out does not disconnect the scrobblers: they are separate accounts the user connected on this desktop, and nothing in them is Apple user data. Disconnecting a scrobbler in Settings → Connections deletes its keyring entry (and fallback file) and its unsent listens.
- SQLite stores metadata only (`auth.AccountStore`): storefront id/name, language, `itre`, obtained/validated timestamps.
- `SignOut()` clears the in-memory token, deletes the keyring entry and the fallback file, deletes the account metadata, then runs the `OnSignOut` hooks in registration order (engine sign-out, engine storage, optional user data). Every step runs even if one fails; failures are returned and shown in `Status.LastError`. The library cache stays unless the user also chooses "clear library".

## 8. Auth Spike Checklist (`cmd/authspike`)

| # | Check | Status |
| --- | --- | --- |
| 1 | Web player token extraction, catalog search with/without `Origin` | done **[verified]** |
| 2 | Loopback start page → Apple sign-in; Apple stores the callback as the redirect target | done **[verified]** |
| 3 | Callback page → `/complete` → MUT decoded → `/v1/me/storefront` | done **[verified]** with a simulated final hop |
| 4 | Interactive login in the system browser | done **[verified]** 2026-10-30 (app) |
| 4a | Interactive native login with 2FA | device-code path done **[verified]** 2026-10-30 (app); no-device SMS/call path pending (needs an account without an Apple device) |
| 5 | Same flow in a Wails window (UA requirements) | superseded by the native form (§4.7) |
| 6 | Paste flow: user-supplied MUT validated and used for playback | done **[verified]** |
| 7 | Revocation: sign out in Apple ID settings, record the 401/403 shape | not done |
