package cdm

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"slices"
	"testing"
)

func TestWidevineTrustIsTheWidevineComponent(t *testing.T) {
	if got := Widevine.ID(); got != "oimompecagnajdejgnnjijobebaeigek" {
		t.Errorf("Widevine.ID() = %s", got)
	}
	tr := testTrust(t)
	if got, want := tr.ID(), crxID(tr.KeyHash[:16]); got != want || len(got) != 32 {
		t.Errorf("ID() = %s, want %s", got, want)
	}
}

func TestVerifyCRX3ReturnsTheArchive(t *testing.T) {
	zip := buildZip(t, packageEntries(linuxAMD64(t), "1.2.3.4"))
	crx := buildCRX3(t, zip, signing{})
	a, err := verifyCRX3(bytes.NewReader(crx), int64(len(crx)), testTrust(t))
	if err != nil {
		t.Fatal(err)
	}
	hlen := int64(binary.LittleEndian.Uint32(crx[8:12]))
	if a.offset != 12+hlen || a.size != int64(len(zip)) {
		t.Fatalf("archive = %+v, want offset %d size %d", a, 12+hlen, len(zip))
	}
	if !bytes.Equal(crx[a.offset:a.offset+a.size], zip) {
		t.Error("archive bytes differ")
	}
}

// crxParts splits a valid CRX into its proofs, signed header and archive.
func crxParts(t *testing.T, crx []byte) ([]proof, []byte, []byte) {
	t.Helper()
	hlen := binary.LittleEndian.Uint32(crx[8:12])
	proofs, signed, err := parseHeader(crx[12 : 12+hlen])
	if err != nil {
		t.Fatal(err)
	}
	return proofs, signed, crx[12+hlen:]
}

// encodeProof encodes p under field.
func encodeProof(field int, p proof) []byte {
	return pbField(field, slices.Concat(pbField(fieldPublicKey, p.key), pbField(fieldSignature, p.sig)))
}

// frame is a CRX3 file with header and archive.
func frame(header, archive []byte) []byte {
	return slices.Concat(crxMagic, le32(3), le32(len(header)), header, archive)
}

func TestVerifyCRX3RefusesForgeries(t *testing.T) {
	p := linuxAMD64(t)
	zip := buildZip(t, packageEntries(p, "1.2.3.4"))
	valid := buildCRX3(t, zip, signing{})
	tr := testTrust(t)
	trusted := tr.KeyHash[:16]
	proofs, signed, archive := crxParts(t, valid)
	rsaProof, ecProof := proofs[0], proofs[1]
	signedField := pbField(fieldSignedHeaderData, signed)

	tests := []struct {
		name string
		crx  []byte
	}{
		{"crx_id changed after signing", buildCRX3(t, zip, signing{tamperSigned: func([]byte) []byte {
			id := slices.Clone(trusted)
			id[0] ^= 1
			return pbField(fieldCRXID, id)
		}})},
		{"signed header extended after signing", buildCRX3(t, zip, signing{tamperSigned: func(b []byte) []byte {
			return append(b, pbField(7, []byte("x"))...)
		}})},
		{"payload byte flipped", buildCRX3(t, zip, signing{flipPayload: true})},
		{"no publisher proof", buildCRX3(t, zip, signing{noPublisher: true})},
		{"another component key", buildCRX3(t, zip, signing{component: otherKey()})},
		{"another key claiming the component's ID", buildCRX3(t, zip, signing{component: otherKey(), crxID: trusted})},
		{"no component proof", frame(slices.Concat(encodeProof(fieldSHA256WithECDSA, ecProof), signedField), archive)},
		{"RSA proof in the ECDSA list", frame(slices.Concat(encodeProof(fieldSHA256WithECDSA, rsaProof), encodeProof(fieldSHA256WithECDSA, ecProof), signedField), archive)},
		{"ECDSA proof in the RSA list", frame(slices.Concat(encodeProof(fieldSHA256WithRSA, rsaProof), encodeProof(fieldSHA256WithRSA, ecProof), signedField), archive)},
		{"a bad extra proof", frame(slices.Concat(encodeProof(fieldSHA256WithRSA, rsaProof), encodeProof(fieldSHA256WithECDSA, ecProof),
			encodeProof(fieldSHA256WithRSA, proof{key: rsaProof.key, sig: bytes.Repeat([]byte{1}, len(rsaProof.sig))}), signedField), archive)},
		{"proof key not a key", frame(slices.Concat(encodeProof(fieldSHA256WithRSA, proof{key: []byte("key"), sig: rsaProof.sig}), encodeProof(fieldSHA256WithRSA, rsaProof), encodeProof(fieldSHA256WithECDSA, ecProof), signedField), archive)},
		{"proof without a signature", frame(slices.Concat(pbField(fieldSHA256WithRSA, pbField(fieldPublicKey, rsaProof.key)), encodeProof(fieldSHA256WithECDSA, ecProof), signedField), archive)},
		{"too many proofs", frame(slices.Concat(bytes.Repeat(encodeProof(fieldSHA256WithRSA, rsaProof), maxProofs+1), encodeProof(fieldSHA256WithECDSA, ecProof), signedField), archive)},
		{"no signed header", frame(slices.Concat(encodeProof(fieldSHA256WithRSA, rsaProof), encodeProof(fieldSHA256WithECDSA, ecProof)), archive)},
		{"crx_id too short", buildCRX3(t, zip, signing{crxID: trusted[:15]})},
		{"malformed header", frame([]byte{0x12, 0xff, 0xff, 0xff, 0x0f}, archive)},
		{"unknown wire type", frame(slices.Concat([]byte{0x0b}, signedField), archive)},
	}
	for _, tok := range eocdTokens {
		tests = append(tests, struct {
			name string
			crx  []byte
		}{"ZIP token " + hex.EncodeToString(tok) + " in the header", buildCRX3(t, zip, signing{headerExtra: pbField(9, slices.Concat([]byte("pad"), tok))})})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := verifyCRX3(bytes.NewReader(tt.crx), int64(len(tt.crx)), testTrust(t))
			wantCode(t, err, CodeSignature)
		})
	}

	t.Run("valid with an unknown header field", func(t *testing.T) {
		crx := buildCRX3(t, zip, signing{headerExtra: slices.Concat(pbField(9, []byte("future")), []byte{0x50, 0x01}, []byte{0x59, 1, 2, 3, 4, 5, 6, 7, 8}, []byte{0x5d, 1, 2, 3, 4})})
		if _, err := verifyCRX3(bytes.NewReader(crx), int64(len(crx)), testTrust(t)); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("valid for another trust", func(t *testing.T) {
		other := testTrust(t)
		other.PublisherKeyHash = sha256.Sum256([]byte("someone else"))
		_, err := verifyCRX3(bytes.NewReader(valid), int64(len(valid)), other)
		wantCode(t, err, CodeSignature)
		_, err = verifyCRX3(bytes.NewReader(valid), int64(len(valid)), Widevine)
		wantCode(t, err, CodeSignature)
	})
}

func TestVerifyCRX3RefusesBadFraming(t *testing.T) {
	valid := makeCRX(t, linuxAMD64(t), "1.2.3.4")
	edit := func(f func(b []byte) []byte) []byte { return f(slices.Clone(valid)) }
	tests := []struct {
		name string
		crx  []byte
	}{
		{"empty", nil},
		{"short", valid[:11]},
		{"wrong magic", edit(func(b []byte) []byte { copy(b, "Cr25"); return b })},
		{"a ZIP", edit(func(b []byte) []byte { copy(b, "PK\x03\x04"); return b })},
		{"version 2", edit(func(b []byte) []byte { binary.LittleEndian.PutUint32(b[4:], 2); return b })},
		{"version 4", edit(func(b []byte) []byte { binary.LittleEndian.PutUint32(b[4:], 4); return b })},
		{"header over the limit", edit(func(b []byte) []byte { binary.LittleEndian.PutUint32(b[8:], maxHeaderBytes+1); return b })},
		{"header length 4 GiB", edit(func(b []byte) []byte { binary.LittleEndian.PutUint32(b[8:], 0xffffffff); return b })},
		{"header to the end", edit(func(b []byte) []byte { binary.LittleEndian.PutUint32(b[8:], uint32(len(b)-12)); return b })},
		{"header past the end", edit(func(b []byte) []byte {
			hlen := binary.LittleEndian.Uint32(b[8:])
			return b[:12+hlen-1]
		})},
		{"header shortened", edit(func(b []byte) []byte {
			binary.LittleEndian.PutUint32(b[8:], binary.LittleEndian.Uint32(b[8:])-1)
			return b
		})},
		{"no archive", edit(func(b []byte) []byte { return b[:12+binary.LittleEndian.Uint32(b[8:])] })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := verifyCRX3(bytes.NewReader(tt.crx), int64(len(tt.crx)), testTrust(t))
			wantCode(t, err, CodeSignature)
		})
	}
}

func TestWalkProtoRefusesMalformedInput(t *testing.T) {
	inputs := [][]byte{
		{0x80},             // unterminated tag
		{0x00},             // field 0
		{0x12},             // length missing
		{0x12, 0x05, 1, 2}, // length past the end
		{0x12, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}, // length 2^64-1
		{0x08},                         // varint missing
		{0x08, 0x80},                   // unterminated varint
		{0x09, 1, 2, 3},                // truncated fixed64
		{0x0d, 1, 2},                   // truncated fixed32
		{0x0b},                         // group start
		{0x0e},                         // wire type 6
		{0xf8, 0xff, 0xff, 0xff, 0x7f}, // field over 2^29
	}
	for _, in := range inputs {
		if err := walkProto(in, func(int, []byte) error { return nil }); err == nil {
			t.Errorf("walkProto(%x) = nil", in)
		}
	}
	var got []int
	err := walkProto(slices.Concat(pbField(1, []byte("a")), []byte{0x10, 0x96, 0x01}, pbField(10000, nil)), func(f int, v []byte) error {
		got = append(got, f)
		return nil
	})
	if err != nil || !slices.Equal(got, []int{1, 10000}) {
		t.Errorf("walkProto = %v, fields %v", err, got)
	}
}

func FuzzWalkProto(f *testing.F) {
	crx := makeCRX(f, linuxAMD64(f), "1.2.3.4")
	f.Add(crx[12 : 12+binary.LittleEndian.Uint32(crx[8:12])])
	f.Add([]byte{0x12, 0x02, 0x0a, 0x00})
	f.Fuzz(func(t *testing.T, b []byte) {
		_ = walkProto(b, func(_ int, v []byte) error {
			if len(v) > len(b) {
				t.Fatalf("field of %d bytes in %d", len(v), len(b))
			}
			return walkProto(v, func(int, []byte) error { return nil })
		})
		_, _, _ = parseHeader(b)
		_, _ = parseSignedData(b)
	})
}

func FuzzVerifyCRX3(f *testing.F) {
	crx := makeCRX(f, linuxAMD64(f), "1.2.3.4")
	f.Add(crx)
	f.Add(crx[:200])
	f.Add([]byte("Cr24\x03\x00\x00\x00\x04\x00\x00\x00\x12\x00\x00\x00x"))
	trust := testTrust(f)
	f.Fuzz(func(t *testing.T, b []byte) {
		a, err := verifyCRX3(bytes.NewReader(b), int64(len(b)), trust)
		if err == nil && (a.offset < 12 || a.offset+a.size != int64(len(b))) {
			t.Fatalf("archive %+v outside %d bytes", a, len(b))
		}
		if err != nil && CodeOf(err) != CodeSignature {
			t.Fatalf("code %s: %v", CodeOf(err), err)
		}
	})
}
