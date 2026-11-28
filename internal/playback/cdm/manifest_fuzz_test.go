package cdm

import (
	"bytes"
	"reflect"
	"slices"
	"testing"
)

// FuzzParseManifest feeds parseManifest the Widevine manifest of Mozilla's
// repository, the document Sona reads over HTTPS before it downloads a
// package from Google, for each platform it installs on
// (testdata/fuzz/FuzzParseManifest: the manifest as published, with its
// aliases; an alias chain that loops; URLs on other hosts, over HTTP, with a
// port or user info; sizes at and over the cap; versions that climb a
// directory). The second argument picks the platform.
//
// It checks that parseManifest does not panic or change its input, gives
// the same answer twice, and that a release it returns, on success, is one
// the installer may act on without looking again:
//   - the version is dotted decimal numbers and nothing else, since it names
//     the install directory;
//   - the size is from 1 byte to maxPackageBytes, which is what the
//     download is bounded by;
//   - the hash is 64 bytes, a SHA-512;
//   - there is a package URL, and every URL is HTTPS on one of Google's
//     package hosts, with no user info, on the default port, once each, and
//     no more of them than the document has room for;
//   - Release.String names the first.
//
// An error, on failure, carries one of the manifest's codes.
func FuzzParseManifest(f *testing.F) {
	f.Fuzz(func(t *testing.T, body []byte, sel uint8) {
		p := platforms[int(sel)%len(platforms)]
		in := bytes.Clone(body)
		r, err := parseManifest(in, p, Mozilla)
		if !bytes.Equal(in, body) {
			t.Fatal("parseManifest changed its input")
		}
		again, err2 := parseManifest(in, p, Mozilla)
		if CodeOf(err) != CodeOf(err2) || !reflect.DeepEqual(r, again) {
			t.Fatalf("parseManifest differs between runs: %+v, %v / %+v, %v", r, err, again, err2)
		}
		if err != nil {
			switch c := CodeOf(err); c {
			case CodeManifest, CodeUnsupportedPlatform, CodeTooLarge, CodeUntrustedURL:
			default:
				t.Fatalf("code %q: %v", c, err)
			}
			if r.Version != "" || r.URLs != nil || r.SHA512 != nil {
				t.Fatalf("release %+v with %v", r, err)
			}
			return
		}
		if !versionPattern.MatchString(r.Version) {
			t.Fatalf("version %q", r.Version)
		}
		if r.Size < 1 || r.Size > maxPackageBytes {
			t.Fatalf("size %d", r.Size)
		}
		if len(r.SHA512) != 64 {
			t.Fatalf("hash of %d bytes", len(r.SHA512))
		}
		if len(r.URLs) == 0 || len(r.URLs)*len("https://") > len(body) {
			t.Fatalf("%d URLs from %d bytes", len(r.URLs), len(body))
		}
		for i, u := range r.URLs {
			if !allowedURL(u, Mozilla.PackageHosts) {
				t.Fatalf("URL %q", u)
			}
			if slices.Index(r.URLs, u) != i {
				t.Fatalf("URL %q twice", u)
			}
		}
		if s := r.String(); !bytes.Contains([]byte(s), []byte(r.URLs[0])) {
			t.Fatalf("String = %q", s)
		}
		if compareVersions(r.Version, r.Version) != 0 {
			t.Fatalf("version %q is not equal to itself", r.Version)
		}
	})
}
