package cdm

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/crc32"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Test keys: the component key (RSA, as Google's), the publisher key
// (ECDSA) and another RSA key no trust names. Generated once per test
// binary.
var (
	componentKey = sync.OnceValue(func() *rsa.PrivateKey { return mustRSA() })
	otherKey     = sync.OnceValue(func() *rsa.PrivateKey { return mustRSA() })
	publisherKey = sync.OnceValue(func() *ecdsa.PrivateKey {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			panic(err)
		}
		return k
	})
)

func mustRSA() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}

func publicDER(t testing.TB, pub crypto.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// testTrust is the Trust the test packages are signed for.
func testTrust(t testing.TB) Trust {
	t.Helper()
	return Trust{
		KeyHash:          sha256.Sum256(publicDER(t, &componentKey().PublicKey)),
		PublisherKeyHash: sha256.Sum256(publicDER(t, &publisherKey().PublicKey)),
	}
}

func platform(t testing.TB, goos, goarch string) Platform {
	t.Helper()
	p, err := PlatformFor(goos, goarch)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func linuxAMD64(t testing.TB) Platform { return platform(t, "linux", "amd64") }

// fakeLibrary is the smallest file p's format check and Go's debug
// packages accept as p's shared library.
func fakeLibrary(p Platform) []byte {
	switch p.GOOS + "/" + p.GOARCH {
	case "linux/amd64":
		return elfLibrary(elf.EM_X86_64)
	case "linux/arm64":
		return elfLibrary(elf.EM_AARCH64)
	case "darwin/arm64":
		return machoLibrary(macho.CpuArm64)
	case "darwin/amd64":
		return machoLibrary(macho.CpuAmd64)
	case "windows/amd64":
		return peLibrary(pe.IMAGE_FILE_MACHINE_AMD64)
	case "windows/arm64":
		return peLibrary(pe.IMAGE_FILE_MACHINE_ARM64)
	}
	panic("no fake library for " + p.String())
}

// elfLibrary is an ELF64 ET_DYN header for m, without sections.
func elfLibrary(m elf.Machine) []byte {
	h := elf.Header64{
		Type:      uint16(elf.ET_DYN),
		Machine:   uint16(m),
		Version:   uint32(elf.EV_CURRENT),
		Ehsize:    64,
		Phentsize: 56,
		Shentsize: 64,
	}
	copy(h.Ident[:], elf.ELFMAG)
	h.Ident[elf.EI_CLASS] = byte(elf.ELFCLASS64)
	h.Ident[elf.EI_DATA] = byte(elf.ELFDATA2LSB)
	h.Ident[elf.EI_VERSION] = byte(elf.EV_CURRENT)
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, h)
	b.Write(make([]byte, 192))
	return b.Bytes()
}

// machoLibrary is a 64-bit Mach-O MH_DYLIB header for cpu, without load
// commands.
func machoLibrary(cpu macho.Cpu) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, macho.FileHeader{Magic: macho.Magic64, Cpu: cpu, Type: macho.TypeDylib})
	b.Write(make([]byte, 4+224)) // the 64-bit header's reserved word, padding
	return b.Bytes()
}

// peLibrary is a PE DLL for machine: a DOS header pointing at the PE
// signature and a COFF header without an optional header or sections.
func peLibrary(machine uint16) []byte {
	b := make([]byte, 512)
	copy(b, "MZ")
	binary.LittleEndian.PutUint32(b[0x3c:], 0x80)
	copy(b[0x80:], "PE\x00\x00")
	var h bytes.Buffer
	_ = binary.Write(&h, binary.LittleEndian, pe.FileHeader{
		Machine:         machine,
		Characteristics: pe.IMAGE_FILE_DLL | pe.IMAGE_FILE_EXECUTABLE_IMAGE | pe.IMAGE_FILE_LARGE_ADDRESS_AWARE,
	})
	copy(b[0x84:], h.Bytes())
	return b
}

// zipEntry is one file of a test archive.
type zipEntry struct {
	name string
	body []byte
	// mode, when set, is the entry's mode (a symlink, say).
	mode fs.FileMode
	// declared, when set, is the uncompressed size the entry's headers
	// claim; the body is stored as is.
	declared uint64
}

func libEntry(p Platform) string { return "_platform_specific/" + p.Dir + "/" + p.Lib }

func componentManifestJSON(name, version string) []byte {
	b, _ := json.Marshal(map[string]any{"manifest_version": 2, "name": name, "version": version})
	return b
}

// packageEntries are the files of a Widevine CRX for p: Google's layout,
// with the signature file.
func packageEntries(p Platform, version string) []zipEntry {
	return []zipEntry{
		{name: manifestFile, body: componentManifestJSON(componentName, version)},
		{name: "LICENSE", body: []byte("Google Widevine terms (test)")},
		{name: "_metadata/verified_contents.json", body: []byte(`[{"description":"treehash per file"}]`)},
		{name: libEntry(p), body: fakeLibrary(p)},
		{name: libEntry(p) + ".sig", body: []byte("widevine host verification (test)")},
	}
}

// without returns entries without the one named name.
func without(entries []zipEntry, name string) []zipEntry {
	return slices.DeleteFunc(slices.Clone(entries), func(e zipEntry) bool { return e.name == name })
}

// replaced returns entries with name's body (and options) replaced by e.
func replaced(entries []zipEntry, e zipEntry) []zipEntry {
	out := slices.Clone(entries)
	for i := range out {
		if out[i].name == e.name {
			out[i] = e
		}
	}
	return out
}

func buildZip(t testing.TB, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate, Modified: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
		if e.mode != 0 {
			h.SetMode(e.mode)
		}
		var (
			w   interface{ Write([]byte) (int, error) }
			err error
		)
		if e.declared != 0 {
			h.Method = zip.Store
			h.CRC32 = crc32.ChecksumIEEE(e.body)
			h.CompressedSize64 = uint64(len(e.body))
			h.UncompressedSize64 = e.declared
			w, err = zw.CreateRaw(h)
		} else {
			w, err = zw.CreateHeader(h)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// signing says how buildCRX3 signs and frames an archive; the zero value
// is a package valid for testTrust.
type signing struct {
	// component signs instead of componentKey (whose proof is then
	// missing).
	component *rsa.PrivateKey
	// crxID is the ID the signed header declares; default the signing
	// component key's.
	crxID []byte
	// noPublisher leaves out the publisher's proof.
	noPublisher bool
	// tamperSigned changes the signed header after signing.
	tamperSigned func([]byte) []byte
	// headerExtra is appended to the header (unsigned).
	headerExtra []byte
	// flipPayload flips an archive byte after signing.
	flipPayload bool
}

// pbField encodes a length-delimited protobuf field.
func pbField(field int, v []byte) []byte {
	b := binary.AppendUvarint(nil, uint64(field)<<3|2)
	b = binary.AppendUvarint(b, uint64(len(v)))
	return append(b, v...)
}

func le32(n int) []byte { return binary.LittleEndian.AppendUint32(nil, uint32(n)) }

// buildCRX3 wraps archive in a CRX3 signed as s says.
func buildCRX3(t testing.TB, archive []byte, s signing) []byte {
	t.Helper()
	comp := componentKey()
	if s.component != nil {
		comp = s.component
	}
	compDER := publicDER(t, &comp.PublicKey)
	id := s.crxID
	if id == nil {
		sum := sha256.Sum256(compDER)
		id = sum[:16]
	}
	signed := pbField(fieldCRXID, id)

	h := sha256.New()
	h.Write(signatureContext)
	h.Write(le32(len(signed)))
	h.Write(signed)
	h.Write(archive)
	digest := h.Sum(nil)

	rsaSig, err := rsa.SignPKCS1v15(rand.Reader, comp, crypto.SHA256, digest)
	if err != nil {
		t.Fatal(err)
	}
	header := pbField(fieldSHA256WithRSA, slices.Concat(pbField(fieldPublicKey, compDER), pbField(fieldSignature, rsaSig)))
	if !s.noPublisher {
		pub := publisherKey()
		ecSig, err := ecdsa.SignASN1(rand.Reader, pub, digest)
		if err != nil {
			t.Fatal(err)
		}
		header = append(header, pbField(fieldSHA256WithECDSA, slices.Concat(pbField(fieldPublicKey, publicDER(t, &pub.PublicKey)), pbField(fieldSignature, ecSig)))...)
	}
	if s.tamperSigned != nil {
		signed = s.tamperSigned(slices.Clone(signed))
	}
	header = append(header, pbField(fieldSignedHeaderData, signed)...)
	header = append(header, s.headerExtra...)

	body := slices.Clone(archive)
	if s.flipPayload {
		body[len(body)/2] ^= 0x01
	}
	return slices.Concat(crxMagic, le32(3), le32(len(header)), header, body)
}

// makeCRX is a valid package of p at version for testTrust.
func makeCRX(t testing.TB, p Platform, version string) []byte {
	t.Helper()
	return buildCRX3(t, buildZip(t, packageEntries(p, version)), signing{})
}

// Hosts of the fake source; requests to any of them reach fakeServer.
const (
	manifestHost  = "gmp.test"
	manifestHost2 = "gmp2.test"
	packageHost   = "dl.test"
	mirrorHost    = "mirror.test"
)

func testSource() Source {
	return Source{
		Manifests:     []string{"https://" + manifestHost + "/widevinecdm.json"},
		ManifestHosts: []string{manifestHost, manifestHost2},
		PackageHosts:  []string{packageHost, mirrorHost},
	}
}

// fakeServer is the manifest host and Google's download hosts: one TLS
// server that every host name dials, routing by Host and path and
// counting requests.
type fakeServer struct {
	srv *httptest.Server
	hc  *http.Client

	mu     sync.Mutex
	routes map[string]http.HandlerFunc // host + path
	hits   map[string]int
}

func newFakeServer(t testing.TB) *fakeServer {
	t.Helper()
	f := &fakeServer{routes: map[string]http.HandlerFunc{}, hits: map[string]int{}}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(f.srv.Close)
	tr := f.srv.Client().Transport.(*http.Transport).Clone()
	tr.TLSClientConfig.ServerName = "example.com" // in httptest's certificate
	addr := f.srv.Listener.Addr().String()
	tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}
	t.Cleanup(tr.CloseIdleConnections)
	f.hc = &http.Client{Transport: tr}
	return f
}

func (f *fakeServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.Host + r.URL.Path
	f.mu.Lock()
	f.hits[key]++
	h := f.routes[key]
	f.mu.Unlock()
	if h == nil {
		http.NotFound(w, r)
		return
	}
	h(w, r)
}

// handle routes hostPath ("dl.test/x.crx3") to h.
func (f *fakeServer) handle(hostPath string, h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[hostPath] = h
}

// serve answers hostPath with body, its length declared.
func (f *fakeServer) serve(hostPath string, body []byte) {
	f.handle(hostPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	})
}

// status answers hostPath with code.
func (f *fakeServer) status(hostPath string, code int) {
	f.handle(hostPath, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) })
}

func (f *fakeServer) count(hostPath string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[hostPath]
}

// total counts every request.
func (f *fakeServer) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.hits {
		n += c
	}
	return n
}

// packageRequests counts requests to the package hosts.
func (f *fakeServer) packageRequests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for k, c := range f.hits {
		if strings.HasPrefix(k, packageHost+"/") || strings.HasPrefix(k, mirrorHost+"/") {
			n += c
		}
	}
	return n
}

// published is where publish put a release.
type published struct {
	pkg, mirror string // host + path
	entry       map[string]any
}

// publish serves crx as version for p on the package host and its
// mirror, and a manifest listing it at testSource's manifest URL.
func (f *fakeServer) publish(p Platform, version string, crx []byte) published {
	pub := published{
		pkg:    packageHost + "/" + version + "/" + p.Dir + ".crx3",
		mirror: mirrorHost + "/" + version + "/" + p.Dir + ".crx3",
	}
	sum := sha512.Sum512(crx)
	pub.entry = map[string]any{
		"fileUrl":    "https://" + pub.pkg,
		"mirrorUrls": []string{"https://" + pub.mirror},
		"filesize":   len(crx),
		"hashValue":  hex.EncodeToString(sum[:]),
	}
	f.serve(pub.pkg, crx)
	f.serve(pub.mirror, crx)
	f.serveManifest(version, map[string]any{p.Mozilla: pub.entry})
	return pub
}

// serveManifest serves a Mozilla manifest with platforms at testSource's
// manifest URL.
func (f *fakeServer) serveManifest(version string, platforms map[string]any) {
	f.serve(manifestHost+"/widevinecdm.json", mozillaJSON(version, platforms))
}

func mozillaJSON(version string, platforms map[string]any) []byte {
	b, err := json.Marshal(map[string]any{
		"hashFunction":   "sha512",
		"name":           "Widevine-" + version,
		"schema_version": 1000,
		"vendors": map[string]any{
			widevineVendor: map[string]any{"version": version, "platforms": platforms},
		},
	})
	if err != nil {
		panic(err)
	}
	return b
}

// testOptions installs linux/amd64 packages from f into dir.
func testOptions(t testing.TB, f *fakeServer, dir string) Options {
	t.Helper()
	return Options{Dir: dir, HTTP: f.hc, Platform: linuxAMD64(t), Source: testSource(), Trust: testTrust(t)}
}

// wantCode fails unless err carries code.
func wantCode(t testing.TB, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want code %q", code)
	}
	if got := CodeOf(err); got != code {
		t.Fatalf("code = %q (%v), want %q", got, err, code)
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("%v is not an *Error", err)
	}
}

// dirNames lists dir's entries, sorted.
func dirNames(t testing.TB, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// readPointer reads current.json.
func readPointer(t testing.TB, dir string) pointer {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, pointerFile))
	if err != nil {
		t.Fatal(err)
	}
	var p pointer
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFakeLibrariesAreTheirPlatformsOnly(t *testing.T) {
	for _, p := range platforms {
		lib := fakeLibrary(p)
		var err error
		switch p.GOOS {
		case "linux":
			var f *elf.File
			if f, err = elf.NewFile(bytes.NewReader(lib)); err == nil {
				_ = f.Close()
			}
		case "darwin":
			var f *macho.File
			if f, err = macho.NewFile(bytes.NewReader(lib)); err == nil {
				_ = f.Close()
			}
		case "windows":
			var f *pe.File
			if f, err = pe.NewFile(bytes.NewReader(lib)); err == nil {
				_ = f.Close()
			}
		}
		if err != nil {
			t.Errorf("%s: debug package rejects the fake library: %v", p, err)
		}
		for _, q := range platforms {
			err := q.format(bytes.NewReader(lib))
			same := q.String() == p.String()
			switch {
			case same && err != nil:
				t.Errorf("%s format check rejects its own library: %v", q, err)
			case !same && err == nil:
				t.Errorf("%s format check accepts %s's library", q, p)
			}
		}
	}
}

func TestFakeCRXVerifies(t *testing.T) {
	for _, p := range platforms {
		crx := makeCRX(t, p, "1.2.3.4")
		if _, err := verifyCRX3(bytes.NewReader(crx), int64(len(crx)), testTrust(t)); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
}
