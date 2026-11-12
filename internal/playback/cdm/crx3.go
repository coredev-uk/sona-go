package cdm

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/coredev-uk/sona-go/internal/protowire"
)

// Trust is what a CRX3 package must prove, as Chromium's component
// updater checks it (components/crx_file/crx_verifier.cc with
// CRX3_WITH_PUBLISHER_PROOF and the component's required key hash).
type Trust struct {
	// KeyHash is the SHA-256 of the component's public key (DER
	// SubjectPublicKeyInfo). Its first 16 bytes are the CRX ID the
	// signed header must declare, and a proof by this key is required.
	KeyHash [32]byte
	// PublisherKeyHash is the SHA-256 of the publisher key whose proof is
	// also required: Google's CRX3 publisher key for anything it ships.
	PublisherKeyHash [32]byte
}

// Widevine is Google's Widevine CDM component: kWidevineSha2Hash from
// Chromium's widevine_cdm_component_installer.cc (component ID
// oimompecagnajdejgnnjijobebaeigek) and kPublisherKeyHash from
// crx_verifier.cc.
var Widevine = Trust{
	KeyHash: [32]byte{
		0xe8, 0xce, 0xcf, 0x42, 0x06, 0xd0, 0x93, 0x49, 0x6d, 0xd9, 0x89,
		0xe1, 0x41, 0x04, 0x86, 0x4a, 0x8f, 0xbd, 0x86, 0x12, 0xb9, 0x58,
		0x9b, 0xfb, 0x4f, 0xbb, 0x1b, 0xa9, 0xd3, 0x85, 0x37, 0xef,
	},
	PublisherKeyHash: [32]byte{
		0x61, 0xf7, 0xf2, 0xa6, 0xbf, 0xcf, 0x74, 0xcd, 0x0b, 0xc1, 0xfe,
		0x24, 0x97, 0xcc, 0x9b, 0x04, 0x25, 0x4c, 0x65, 0x8f, 0x79, 0xf2,
		0x14, 0x53, 0x92, 0x86, 0x7e, 0xa8, 0x36, 0x63, 0x67, 0xcf,
	},
}

// ID is the component's extension ID: the first 16 bytes of KeyHash,
// each nibble written as a letter from 'a' to 'p'.
func (t Trust) ID() string {
	return crxID(t.KeyHash[:16])
}

func crxID(b []byte) string {
	id := make([]byte, 0, 2*len(b))
	for _, c := range b {
		id = append(id, 'a'+c>>4, 'a'+c&0xf)
	}
	return string(id)
}

// Bounds on a CRX3 header.
const (
	maxHeaderBytes = 256 << 10 // Google's is ~1 KiB
	maxProofs      = 16
)

// crxMagic starts every CRX file; signatureContext starts the signed data.
var (
	crxMagic         = []byte("Cr24")
	signatureContext = []byte("CRX3 SignedData\x00")
	// ZIP end-of-central-directory tokens: a header holding one could
	// make the archive parse differently (crx_verifier.cc refuses them).
	eocdTokens = [][]byte{{'P', 'K', 5, 6}, {'P', 'K', 6, 7}, {'P', 'K', 6, 6}}
)

// archive is where a verified CRX3's ZIP archive lies in its file.
type archive struct {
	offset, size int64
}

type proof struct {
	key, sig []byte
	ecdsa    bool
}

// verifyCRX3 checks the CRX3 in f (size bytes) against t: the CRX3 magic
// and version, a header that parses, declares t's CRX ID and carries
// proofs by t's component key and publisher key, and every proof's
// signature over the signed header and the archive. It returns where the
// archive is.
func verifyCRX3(f io.ReaderAt, size int64, t Trust) (archive, error) {
	var head [12]byte
	if _, err := f.ReadAt(head[:], 0); err != nil {
		return archive{}, fail(CodeSignature, "crx3: header: %w", err)
	}
	if !bytes.Equal(head[:4], crxMagic) {
		return archive{}, fail(CodeSignature, "crx3: not a CRX file")
	}
	if v := binary.LittleEndian.Uint32(head[4:8]); v != 3 {
		return archive{}, fail(CodeSignature, "crx3: version %d, want 3", v)
	}
	hlen := int64(binary.LittleEndian.Uint32(head[8:12]))
	if hlen > maxHeaderBytes || 12+hlen >= size {
		return archive{}, fail(CodeSignature, "crx3: header length %d", hlen)
	}
	header := make([]byte, hlen)
	if _, err := f.ReadAt(header, 12); err != nil {
		return archive{}, fail(CodeSignature, "crx3: header: %w", err)
	}
	for _, tok := range eocdTokens {
		if bytes.Contains(header, tok) {
			return archive{}, fail(CodeSignature, "crx3: header holds a ZIP directory token")
		}
	}
	proofs, signed, err := parseHeader(header)
	if err != nil {
		return archive{}, fail(CodeSignature, "crx3: %w", err)
	}
	id, err := parseSignedData(signed)
	if err != nil {
		return archive{}, fail(CodeSignature, "crx3: signed header: %w", err)
	}
	if !bytes.Equal(id, t.KeyHash[:16]) {
		return archive{}, fail(CodeSignature, "crx3: package is %s, not %s", crxID(id), t.ID())
	}

	a := archive{offset: 12 + hlen, size: size - 12 - hlen}
	h := sha256.New()
	h.Write(signatureContext)
	_ = binary.Write(h, binary.LittleEndian, uint32(len(signed)))
	h.Write(signed)
	if _, err := io.Copy(h, io.NewSectionReader(f, a.offset, a.size)); err != nil {
		return archive{}, fail(CodeSignature, "crx3: archive: %w", err)
	}
	digest := h.Sum(nil)

	var component, publisher bool
	for _, p := range proofs {
		kh := sha256.Sum256(p.key)
		component = component || kh == t.KeyHash
		publisher = publisher || kh == t.PublisherKeyHash
		if err := p.verify(digest); err != nil {
			return archive{}, fail(CodeSignature, "crx3: proof by key %x…: %w", kh[:4], err)
		}
	}
	switch {
	case !component:
		return archive{}, fail(CodeSignature, "crx3: no proof by the %s component key", t.ID())
	case !publisher:
		return archive{}, fail(CodeSignature, "crx3: no proof by the publisher key")
	}
	return a, nil
}

// verify checks the proof's signature over digest (SHA-256): RSA
// PKCS #1 v1.5 or ECDSA (ASN.1), whichever list the proof came from.
func (p proof) verify(digest []byte) error {
	pub, err := x509.ParsePKIXPublicKey(p.key)
	if err != nil {
		return err
	}
	switch k := pub.(type) {
	case *rsa.PublicKey:
		if p.ecdsa {
			return errors.New("RSA key in an ECDSA proof")
		}
		return rsa.VerifyPKCS1v15(k, crypto.SHA256, digest, p.sig)
	case *ecdsa.PublicKey:
		if !p.ecdsa {
			return errors.New("ECDSA key in an RSA proof")
		}
		if !ecdsa.VerifyASN1(k, digest, p.sig) {
			return errors.New("ECDSA signature does not verify")
		}
		return nil
	}
	return fmt.Errorf("unsupported key type %T", pub)
}

// CrxFileHeader fields (components/crx_file/crx3.proto).
const (
	fieldSHA256WithRSA    = 2
	fieldSHA256WithECDSA  = 3
	fieldSignedHeaderData = 10000
	fieldCRXID            = 1 // SignedData
	fieldPublicKey        = 1 // AsymmetricKeyProof
	fieldSignature        = 2 // AsymmetricKeyProof
)

// parseHeader decodes a CrxFileHeader: its proofs and signed_header_data.
func parseHeader(b []byte) ([]proof, []byte, error) {
	var (
		proofs []proof
		signed []byte
	)
	err := walkProto(b, func(field int, v []byte) error {
		switch field {
		case fieldSHA256WithRSA, fieldSHA256WithECDSA:
			if len(proofs) == maxProofs {
				return errors.New("too many proofs")
			}
			p := proof{ecdsa: field == fieldSHA256WithECDSA}
			if err := walkProto(v, func(f int, v []byte) error {
				switch f {
				case fieldPublicKey:
					p.key = v
				case fieldSignature:
					p.sig = v
				}
				return nil
			}); err != nil {
				return err
			}
			if len(p.key) == 0 || len(p.sig) == 0 {
				return errors.New("proof without a key or signature")
			}
			proofs = append(proofs, p)
		case fieldSignedHeaderData:
			signed = v
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if signed == nil {
		return nil, nil, errors.New("no signed header data")
	}
	return proofs, signed, nil
}

// parseSignedData decodes a SignedData: its 16-byte crx_id.
func parseSignedData(b []byte) ([]byte, error) {
	var id []byte
	err := walkProto(b, func(field int, v []byte) error {
		if field == fieldCRXID {
			id = v
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(id) != 16 {
		return nil, fmt.Errorf("crx_id is %d bytes, want 16", len(id))
	}
	return id, nil
}

// walkProto calls fn with every length-delimited field of a protobuf
// message (the only kind CRX3 headers use; the varint and fixed-size
// fields of a newer header are skipped).
func walkProto(b []byte, fn func(field int, v []byte) error) error {
	return protowire.Walk(b, func(num, wire, _ uint64, data []byte) error {
		if wire != protowire.Bytes {
			return nil
		}
		return fn(int(num), data)
	})
}
