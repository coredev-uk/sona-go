package appleid

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// SRP-6a as Apple's sign-in widget runs it (webSRPClientWorker.js): the
// RFC 5054 2048-bit group, SHA-256, an empty identity inside x, and the
// lowercased account name inside M1.

const groupN = "AC6BDB41324A9A9BF166DE5E1389582FAF72B6651987EE07FC3192943DB56050A37329CBB4A099ED8193E0757767A13DD52312AB4B03310DCD7F48A9DA04FD50E8083969EDB767B0CF6095179A163AB3661A05FBD5FAAAE82918A9962F0B93B855F97993EC975EEAA80D740ADBF4FF747359D041D5C33EA71D281E446B14773BCA97B43A23FB801676BD207A436C6481F1D2B9078717461A5B9D32E688F87748544523B524B0D57D5EA77A2775D2ECFA032CFBDBF52FB3786160279004E57AE6AF874E7303CE53299CCC041C7BC308D82A5698F3A8D0C38271AE35F8E9DBFBB694B5C803D89F7AE435DE236D525F54759B65E372FCD68EF20FA7111F9E4AFF73"

var (
	srpN, _  = new(big.Int).SetString(groupN, 16)
	srpG     = big.NewInt(2)
	srpNLen  = len(srpN.Bytes()) // 256
	srpNHash = sha(srpN.Bytes())
)

// Password key derivations Apple offers in signin/init.
const (
	protoS2K   = "s2k"    // PBKDF2 over SHA-256(password)
	protoS2KFO = "s2k_fo" // PBKDF2 over hex(SHA-256(password))
)

var srpProtocols = []string{protoS2K, protoS2KFO}

type srpClient struct {
	account string // as typed; lowercased for M1
	a       *big.Int
	A       []byte // padded to len(N)
}

// newSRPClient draws a 256-byte private value; tests pass a fixed one.
func newSRPClient(account string, private []byte) (*srpClient, error) {
	if private == nil {
		private = make([]byte, 256)
		if _, err := rand.Read(private); err != nil {
			return nil, err
		}
	}
	a := new(big.Int).SetBytes(private)
	A := pad(new(big.Int).Exp(srpG, a, srpN))
	return &srpClient{account: account, a: a, A: A}, nil
}

// evidence computes M1 and M2 for the server's challenge.
func (c *srpClient) evidence(password string, salt, serverB []byte, iterations int, protocol string) (m1, m2 []byte, err error) {
	if protocol == "" {
		protocol = protoS2K
	}
	if protocol != protoS2K && protocol != protoS2KFO {
		return nil, nil, fmt.Errorf("appleid: unsupported password protocol %q", protocol)
	}
	B := new(big.Int).SetBytes(serverB)
	if new(big.Int).Mod(B, srpN).Sign() == 0 {
		return nil, nil, errors.New("appleid: invalid server public value")
	}

	// The worker hex-encodes only on the PBKDF2 path; zero iterations use the
	// raw digest for both protocols.
	p := sha([]byte(password))
	if iterations > 0 {
		key := p
		if protocol == protoS2KFO {
			key = []byte(hex.EncodeToString(p))
		}
		if p, err = pbkdf2.Key(sha256.New, string(key), salt, iterations, 32); err != nil {
			return nil, nil, err
		}
	}

	k := new(big.Int).SetBytes(sha(srpN.Bytes(), pad(srpG)))
	x := new(big.Int).SetBytes(sha(salt, sha([]byte(":"), p)))
	u := new(big.Int).SetBytes(sha(c.A, pad(B)))

	// S = (B - k·g^x) ^ (a + u·x) mod N
	kgx := new(big.Int).Exp(srpG, x, srpN)
	kgx.Mul(kgx, k).Mod(kgx, srpN)
	base := new(big.Int).Sub(B, kgx)
	base.Mod(base, srpN)
	exp := new(big.Int).Mul(u, x)
	exp.Add(exp, c.a)
	S := new(big.Int).Exp(base, exp, srpN)
	K := sha(pad(S))

	hg := sha(pad(srpG))
	xor := make([]byte, len(srpNHash))
	for i := range xor {
		xor[i] = srpNHash[i] ^ hg[i]
	}
	// Apple's worker rebuilds H(N) xor H(g) from a BigInt, which drops
	// leading zero bytes.
	xor = new(big.Int).SetBytes(xor).Bytes()

	m1 = sha(xor, sha([]byte(strings.ToLower(c.account))), salt, c.A, serverB, K)
	m2 = sha(c.A, m1, K)
	return m1, m2, nil
}

func pad(n *big.Int) []byte { return n.FillBytes(make([]byte, srpNLen)) }

func sha(parts ...[]byte) []byte {
	h := sha256.New()
	for _, p := range parts {
		h.Write(p)
	}
	return h.Sum(nil)
}
