package cdm

import (
	"bytes"
	"context"
	"encoding/hex"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// realManifest is Mozilla's widevinecdm.json as published (trimmed URL
// paths, fake hashes): aliases, -asan entries, 32-bit Windows, no Linux
// arm64.
var realManifest = strings.NewReplacer(
	"HASH_MAC_ARM64", strings.Repeat("a1", 64),
	"HASH_MAC64", strings.Repeat("b2", 64),
	"HASH_LINUX64", strings.Repeat("c3", 64),
	"HASH_WIN_ARM64", strings.Repeat("d4", 64),
	"HASH_WIN32", strings.Repeat("e5", 64),
	"HASH_WIN64", strings.Repeat("f6", 64),
).Replace(`{
  "hashFunction": "sha512",
  "name": "Widevine-4.10.3112.0",
  "schema_version": 1000,
  "vendors": {
    "gmp-widevinecdm": {
      "platforms": {
        "Darwin_aarch64-gcc3": {
          "fileUrl": "https://edgedl.me.gvt1.com/edgedl/release2/chrome_component/x_4.10.3112.0/oimompecagnajdejgnnjijobebaeigek_4.10.3112.0_mac_arm64_x.crx3",
          "mirrorUrls": ["https://www.google.com/dl/release2/chrome_component/x_4.10.3112.0/oimompecagnajdejgnnjijobebaeigek_4.10.3112.0_mac_arm64_x.crx3"],
          "filesize": 20423379,
          "hashValue": "HASH_MAC_ARM64"
        },
        "Darwin_x86_64-gcc3": {"alias": "Darwin_x86_64-gcc3-u-i386-x86_64"},
        "Darwin_x86_64-gcc3-u-i386-x86_64": {
          "fileUrl": "https://edgedl.me.gvt1.com/edgedl/release2/chrome_component/x_4.10.3112.0/oimompecagnajdejgnnjijobebaeigek_4.10.3112.0_mac64_x.crx3",
          "mirrorUrls": ["https://www.google.com/dl/release2/chrome_component/x_4.10.3112.0/oimompecagnajdejgnnjijobebaeigek_4.10.3112.0_mac64_x.crx3"],
          "filesize": 22867646,
          "hashValue": "HASH_MAC64"
        },
        "Linux_x86_64-gcc3": {
          "fileUrl": "https://edgedl.me.gvt1.com/edgedl/release2/chrome_component/x_4.10.3112.0/oimompecagnajdejgnnjijobebaeigek_4.10.3112.0_linux64_x.crx3",
          "mirrorUrls": ["https://www.google.com/dl/release2/chrome_component/x_4.10.3112.0/oimompecagnajdejgnnjijobebaeigek_4.10.3112.0_linux64_x.crx3"],
          "filesize": 21970001,
          "hashValue": "HASH_LINUX64"
        },
        "Linux_x86_64-gcc3-asan": {"alias": "Linux_x86_64-gcc3"},
        "WINNT_aarch64-msvc-aarch64": {
          "fileUrl": "https://edgedl.me.gvt1.com/edgedl/release2/chrome_component/x_4.10.3112.0/oimompecagnajdejgnnjijobebaeigek_4.10.3112.0_win_arm64_x.crx3",
          "mirrorUrls": ["https://www.google.com/dl/release2/chrome_component/x_4.10.3112.0/oimompecagnajdejgnnjijobebaeigek_4.10.3112.0_win_arm64_x.crx3"],
          "filesize": 20270133,
          "hashValue": "HASH_WIN_ARM64"
        },
        "WINNT_x86-msvc": {
          "fileUrl": "https://edgedl.me.gvt1.com/edgedl/release2/chrome_component/x_4.10.3112.0/oimompecagnajdejgnnjijobebaeigek_4.10.3112.0_win32_x.crx3",
          "mirrorUrls": ["https://www.google.com/dl/release2/chrome_component/x_4.10.3112.0/oimompecagnajdejgnnjijobebaeigek_4.10.3112.0_win32_x.crx3"],
          "filesize": 21861906,
          "hashValue": "HASH_WIN32"
        },
        "WINNT_x86-msvc-x64": {"alias": "WINNT_x86-msvc"},
        "WINNT_x86-msvc-x86": {"alias": "WINNT_x86-msvc"},
        "WINNT_x86_64-msvc": {
          "fileUrl": "https://edgedl.me.gvt1.com/edgedl/release2/chrome_component/x_4.10.3112.0/oimompecagnajdejgnnjijobebaeigek_4.10.3112.0_win64_x.crx3",
          "mirrorUrls": ["https://www.google.com/dl/release2/chrome_component/x_4.10.3112.0/oimompecagnajdejgnnjijobebaeigek_4.10.3112.0_win64_x.crx3"],
          "filesize": 22976546,
          "hashValue": "HASH_WIN64"
        },
        "WINNT_x86_64-msvc-x64": {"alias": "WINNT_x86_64-msvc"},
        "WINNT_x86_64-msvc-x64-asan": {"alias": "WINNT_x86_64-msvc"}
      },
      "version": "4.10.3112.0"
    }
  }
}`)

func TestParseManifestSelectsEachPlatform(t *testing.T) {
	tests := []struct {
		goos, goarch string
		file         string // the package file name's platform part
		size         int64
		hash         string
		code         string
	}{
		{goos: "linux", goarch: "amd64", file: "_linux64_", size: 21970001, hash: strings.Repeat("c3", 64)},
		{goos: "linux", goarch: "arm64", code: CodeUnsupportedPlatform},
		{goos: "darwin", goarch: "arm64", file: "_mac_arm64_", size: 20423379, hash: strings.Repeat("a1", 64)},
		// Darwin_x86_64-gcc3 is an alias.
		{goos: "darwin", goarch: "amd64", file: "_mac64_", size: 22867646, hash: strings.Repeat("b2", 64)},
		{goos: "windows", goarch: "amd64", file: "_win64_", size: 22976546, hash: strings.Repeat("f6", 64)},
		{goos: "windows", goarch: "arm64", file: "_win_arm64_", size: 20270133, hash: strings.Repeat("d4", 64)},
	}
	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			r, err := parseManifest([]byte(realManifest), platform(t, tt.goos, tt.goarch), Mozilla)
			if tt.code != "" {
				wantCode(t, err, tt.code)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if r.Version != "4.10.3112.0" || r.Size != tt.size || hex.EncodeToString(r.SHA512) != tt.hash {
				t.Errorf("release = %s %d %x", r.Version, r.Size, r.SHA512)
			}
			if len(r.URLs) != 2 || !strings.HasPrefix(r.URLs[0], "https://edgedl.me.gvt1.com/") || !strings.HasPrefix(r.URLs[1], "https://www.google.com/") {
				t.Fatalf("URLs = %q, want Google's URL then its mirror", r.URLs)
			}
			for _, u := range r.URLs {
				if !strings.Contains(u, tt.file) {
					t.Errorf("URL %s is not the %s package", u, tt.file)
				}
			}
		})
	}
}

// entry is a valid manifest platform entry on testSource's hosts.
func entry() map[string]any {
	return map[string]any{
		"fileUrl":    "https://" + packageHost + "/p.crx3",
		"mirrorUrls": []string{"https://" + mirrorHost + "/p.crx3"},
		"filesize":   1234,
		"hashValue":  strings.Repeat("ab", 64),
	}
}

func with(e map[string]any, key string, v any) map[string]any {
	out := map[string]any{}
	for k, x := range e {
		out[k] = x
	}
	out[key] = v
	return out
}

func TestParseManifestAliases(t *testing.T) {
	p := linuxAMD64(t)
	alias := func(to string) map[string]any { return map[string]any{"alias": to} }
	tests := []struct {
		name      string
		platforms map[string]any
		code      string
	}{
		{"three hops", map[string]any{p.Mozilla: alias("a1"), "a1": alias("a2"), "a2": alias("a3"), "a3": entry()}, ""},
		{"four hops", map[string]any{p.Mozilla: alias("a1"), "a1": alias("a2"), "a2": alias("a3"), "a3": alias("a4"), "a4": entry()}, CodeManifest},
		{"loop", map[string]any{p.Mozilla: alias("a1"), "a1": alias(p.Mozilla)}, CodeManifest},
		{"self", map[string]any{p.Mozilla: alias(p.Mozilla)}, CodeManifest},
		{"dangling", map[string]any{p.Mozilla: alias("gone")}, CodeUnsupportedPlatform},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := parseManifest(mozillaJSON("1.2.3.4", tt.platforms), p, testSource())
			if tt.code != "" {
				wantCode(t, err, tt.code)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if r.Size != 1234 {
				t.Errorf("size = %d, want the aliased entry's", r.Size)
			}
		})
	}
}

func TestParseManifestRefuses(t *testing.T) {
	p := linuxAMD64(t)
	doc := func(version string, e any) []byte { return mozillaJSON(version, map[string]any{p.Mozilla: e}) }
	tests := []struct {
		name string
		body []byte
		code string
	}{
		{"not JSON", []byte("<html>"), CodeManifest},
		{"sha256", bytes.Replace(doc("1.2.3.4", entry()), []byte(`"sha512"`), []byte(`"sha256"`), 1), CodeManifest},
		{"no hash function", bytes.Replace(doc("1.2.3.4", entry()), []byte(`"hashFunction":"sha512",`), nil, 1), CodeManifest},
		{"no vendor", bytes.Replace(doc("1.2.3.4", entry()), []byte(widevineVendor), []byte("gmp-other"), 1), CodeManifest},
		{"version climbs", doc("../1", entry()), CodeManifest},
		{"version with a slash", doc("1.2/3", entry()), CodeManifest},
		{"version without dots", doc("1", entry()), CodeManifest},
		{"version too long", doc("1.2.3.4.5", entry()), CodeManifest},
		{"empty version", doc("", entry()), CodeManifest},
		{"platform not an object", doc("1.2.3.4", "Linux"), CodeManifest},
		{"hash not hex", doc("1.2.3.4", with(entry(), "hashValue", strings.Repeat("zz", 64))), CodeManifest},
		{"hash too short", doc("1.2.3.4", with(entry(), "hashValue", strings.Repeat("ab", 63))), CodeManifest},
		{"hash too long", doc("1.2.3.4", with(entry(), "hashValue", strings.Repeat("ab", 65))), CodeManifest},
		{"size zero", doc("1.2.3.4", with(entry(), "filesize", 0)), CodeTooLarge},
		{"size negative", doc("1.2.3.4", with(entry(), "filesize", -1)), CodeTooLarge},
		{"size over the cap", doc("1.2.3.4", with(entry(), "filesize", maxPackageBytes+1)), CodeTooLarge},
		{"no URL", doc("1.2.3.4", with(with(entry(), "fileUrl", ""), "mirrorUrls", nil)), CodeUntrustedURL},
		{"all URLs untrusted", doc("1.2.3.4", with(with(entry(), "fileUrl", "http://"+packageHost+"/p.crx3"), "mirrorUrls", []string{"https://evil.test/p.crx3", "https://" + packageHost + ":8443/p.crx3"})), CodeUntrustedURL},
		{"platform missing", mozillaJSON("1.2.3.4", map[string]any{"Linux_aarch64-gcc3": entry()}), CodeUnsupportedPlatform},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseManifest(tt.body, p, testSource())
			wantCode(t, err, tt.code)
		})
	}
}

func TestParseManifestKeepsOnlyTrustedURLs(t *testing.T) {
	p := linuxAMD64(t)
	e := with(with(entry(), "fileUrl", "http://"+packageHost+"/p.crx3"), "mirrorUrls", []string{
		"https://evil.test/p.crx3",
		"https://" + mirrorHost + "/p.crx3",
		"https://" + mirrorHost + "/p.crx3",
		"https://" + packageHost + "/q.crx3",
	})
	e["filesize"] = maxPackageBytes
	r, err := parseManifest(mozillaJSON("1.2.3.4", map[string]any{p.Mozilla: e}), p, testSource())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://" + mirrorHost + "/p.crx3", "https://" + packageHost + "/q.crx3"}
	if !slices.Equal(r.URLs, want) {
		t.Errorf("URLs = %q, want %q", r.URLs, want)
	}
	if r.Size != maxPackageBytes {
		t.Errorf("size = %d, want the cap itself accepted", r.Size)
	}
}

func TestAllowedURL(t *testing.T) {
	hosts := []string{"dl.test", "www.google.com"}
	tests := []struct {
		url  string
		want bool
	}{
		{"https://dl.test/p.crx3", true},
		{"https://dl.test:443/p.crx3", true},
		{"https://DL.Test/p.crx3", true},
		{"HTTPS://dl.test/p.crx3", true},
		{"https://www.google.com/dl/p.crx3", true},
		{"http://dl.test/p.crx3", false},
		{"ftp://dl.test/p.crx3", false},
		{"https://other.test/p.crx3", false},
		{"https://dl.test.evil.test/p.crx3", false},
		{"https://evil.test/dl.test", false},
		{"https://user@dl.test/p.crx3", false},
		{"https://user:pass@dl.test/p.crx3", false},
		{"https://dl.test:8443/p.crx3", false},
		{"https://dl.test:80/p.crx3", false},
		{"//dl.test/p.crx3", false},
		{"dl.test/p.crx3", false},
		{"", false},
		{"https://dl.test:bad/p", false},
		{"https://%zz/p", false},
	}
	for _, tt := range tests {
		if got := allowedURL(tt.url, hosts); got != tt.want {
			t.Errorf("allowedURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int // sign
	}{
		{"4.10.3112.0", "4.10.3112.0", 0},
		{"4.10.3112.0", "4.10.3112", 0},
		{"4.10.2891.0", "4.10.3112.0", -1},
		{"4.10.3112.0", "4.10.2891.0", 1},
		{"4.9.0.0", "4.10.0.0", -1},
		{"1.10", "1.9", 1},
		{"1.2", "1.2.0.1", -1},
		{"10.0", "9.99.99.99", 1},
		{"1.02", "1.2", 0},
	}
	sign := func(n int) int { return min(max(n, -1), 1) }
	for _, tt := range tests {
		if got := sign(compareVersions(tt.a, tt.b)); got != tt.want {
			t.Errorf("compareVersions(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestFetchReleaseFallsBackToTheNextManifest(t *testing.T) {
	f := newFakeServer(t)
	p := linuxAMD64(t)
	src := testSource()
	src.Manifests = []string{
		"https://" + manifestHost + "/down.json",
		"https://evil.test/widevinecdm.json", // not a manifest host: never asked
		"https://" + manifestHost2 + "/widevinecdm.json",
	}
	f.status(manifestHost+"/down.json", http.StatusInternalServerError)
	f.serve(manifestHost2+"/widevinecdm.json", mozillaJSON("1.2.3.4", map[string]any{p.Mozilla: entry()}))
	r, err := fetchRelease(t.Context(), f.hc, src, p)
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "1.2.3.4" {
		t.Errorf("version = %s", r.Version)
	}
	if f.count(manifestHost+"/down.json") != 1 || f.count("evil.test/widevinecdm.json") != 0 {
		t.Errorf("requests: %v", f.hits)
	}
}

func TestFetchReleaseStopsAtAManifestWithoutThePlatform(t *testing.T) {
	f := newFakeServer(t)
	p := linuxAMD64(t)
	src := testSource()
	src.Manifests = []string{"https://" + manifestHost + "/a.json", "https://" + manifestHost2 + "/b.json"}
	f.serve(manifestHost+"/a.json", mozillaJSON("1.2.3.4", map[string]any{"Darwin_aarch64-gcc3": entry()}))
	f.serve(manifestHost2+"/b.json", mozillaJSON("1.2.3.4", map[string]any{p.Mozilla: entry()}))
	_, err := fetchRelease(t.Context(), f.hc, src, p)
	wantCode(t, err, CodeUnsupportedPlatform)
	if n := f.count(manifestHost2 + "/b.json"); n != 0 {
		t.Errorf("second manifest asked %d times", n)
	}
}

func TestFetchReleaseFailures(t *testing.T) {
	p := linuxAMD64(t)
	tests := []struct {
		name  string
		setup func(f *fakeServer)
		code  string
	}{
		{"HTTP error", func(f *fakeServer) { f.status(manifestHost+"/widevinecdm.json", http.StatusServiceUnavailable) }, CodeNetwork},
		{"not found", func(*fakeServer) {}, CodeNetwork},
		{"oversize", func(f *fakeServer) {
			f.serve(manifestHost+"/widevinecdm.json", bytes.Repeat([]byte(" "), maxManifestBytes+1))
		}, CodeTooLarge},
		{"redirect to another host", func(f *fakeServer) {
			f.handle(manifestHost+"/widevinecdm.json", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "https://evil.test/widevinecdm.json", http.StatusFound)
			})
			f.serve("evil.test/widevinecdm.json", mozillaJSON("1.2.3.4", map[string]any{p.Mozilla: entry()}))
		}, CodeUntrustedURL},
		{"redirect to plain HTTP", func(f *fakeServer) {
			f.handle(manifestHost+"/widevinecdm.json", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "http://"+manifestHost2+"/widevinecdm.json", http.StatusFound)
			})
		}, CodeUntrustedURL},
		{"redirect loop", func(f *fakeServer) {
			f.handle(manifestHost+"/widevinecdm.json", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "https://"+manifestHost+"/widevinecdm.json", http.StatusFound)
			})
		}, CodeNetwork},
		{"malformed", func(f *fakeServer) { f.serve(manifestHost+"/widevinecdm.json", []byte("{")) }, CodeManifest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeServer(t)
			tt.setup(f)
			_, err := fetchRelease(t.Context(), f.hc, testSource(), p)
			wantCode(t, err, tt.code)
			if f.count("evil.test/widevinecdm.json") != 0 {
				t.Error("followed a redirect to an untrusted host")
			}
		})
	}
}

func TestFetchReleaseFollowsRedirectsBetweenManifestHosts(t *testing.T) {
	f := newFakeServer(t)
	p := linuxAMD64(t)
	f.handle(manifestHost+"/widevinecdm.json", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://"+manifestHost2+"/edge.json", http.StatusMovedPermanently)
	})
	f.serve(manifestHost2+"/edge.json", mozillaJSON("1.2.3.4", map[string]any{p.Mozilla: entry()}))
	if _, err := fetchRelease(t.Context(), f.hc, testSource(), p); err != nil {
		t.Fatal(err)
	}
}

func TestFetchReleaseRefusesUntrustedManifestURLs(t *testing.T) {
	f := newFakeServer(t)
	src := testSource()
	src.Manifests = []string{"http://" + manifestHost + "/widevinecdm.json", "https://other.test/widevinecdm.json"}
	_, err := fetchRelease(t.Context(), f.hc, src, linuxAMD64(t))
	wantCode(t, err, CodeUntrustedURL)
	if f.total() != 0 {
		t.Errorf("made %d requests", f.total())
	}
}

func TestFetchReleaseHonoursCancel(t *testing.T) {
	f := newFakeServer(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := fetchRelease(ctx, f.hc, testSource(), linuxAMD64(t))
	wantCode(t, err, CodeNetwork)
}

func TestFetchReleaseReadsTheManifestOnly(t *testing.T) {
	f := newFakeServer(t)
	p := linuxAMD64(t)
	pub := f.publish(p, "4.10.3112.0", []byte("crx"))
	r, err := FetchRelease(t.Context(), Options{HTTP: f.hc, Platform: p, Source: testSource()})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://" + pub.pkg, "https://" + pub.mirror}
	if r.Version != "4.10.3112.0" || r.Size != 3 || !slices.Equal(r.URLs, want) {
		t.Errorf("release = %s %q", r, r.URLs)
	}
	if n := f.packageRequests(); n != 0 {
		t.Errorf("%d package requests", n)
	}
}
