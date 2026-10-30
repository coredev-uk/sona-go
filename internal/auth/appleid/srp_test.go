package appleid

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
)

// testdata/srp_vectors.json was produced by running Apple's own
// webSRPClientWorker.js (appleid.cdn-apple.com/appleauth/static/jsj/
// 378907636/webSRPClientWorker.js) under Node with crypto.getRandomValues
// stubbed to return the bytes in "a". Matching it proves the Go client
// sends the evidence Apple's widget would. The first vector's salt starts
// with a zero byte and its account name is mixed case.
func TestSRPMatchesAppleWorker(t *testing.T) {
	raw, err := os.ReadFile("testdata/srp_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vecs []struct {
		Protocol   string `json:"protocol"`
		Account    string `json:"account"`
		Password   string `json:"password"`
		Salt       string `json:"salt"`
		Iterations int    `json:"iterations"`
		Private    string `json:"a"`
		Public     string `json:"A"`
		B          string `json:"B"`
		M1         string `json:"m1"`
		M2         string `json:"m2"`
	}
	if err := json.Unmarshal(raw, &vecs); err != nil || len(vecs) == 0 {
		t.Fatalf("vectors: %v", err)
	}
	b64 := base64.StdEncoding
	for _, v := range vecs {
		t.Run(v.Protocol, func(t *testing.T) {
			priv, _ := b64.DecodeString(v.Private)
			c, err := newSRPClient(v.Account, priv)
			if err != nil {
				t.Fatal(err)
			}
			if b64.EncodeToString(c.A) != v.Public {
				t.Fatal("A mismatch")
			}
			salt, _ := b64.DecodeString(v.Salt)
			B, _ := b64.DecodeString(v.B)
			m1, m2, err := c.evidence(v.Password, salt, B, v.Iterations, v.Protocol)
			if err != nil {
				t.Fatal(err)
			}
			want1, _ := b64.DecodeString(v.M1)
			want2, _ := b64.DecodeString(v.M2)
			if !bytes.Equal(m1, want1) || !bytes.Equal(m2, want2) {
				t.Fatalf("evidence mismatch: m1=%s m2=%s", b64.EncodeToString(m1), b64.EncodeToString(m2))
			}
		})
	}
}

func TestSRPRejectsBadChallenge(t *testing.T) {
	c, err := newSRPClient("a@b.c", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.evidence("pw", []byte{1}, srpN.Bytes(), 1, protoS2K); err == nil {
		t.Fatal("B = N accepted")
	}
	if _, _, err := c.evidence("pw", []byte{1}, []byte{2}, 1, "s2k_new"); err == nil {
		t.Fatal("unknown protocol accepted")
	}
}
