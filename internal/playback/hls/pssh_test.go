package hls

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func psshBox(data []byte) []byte {
	b := make([]byte, 32, 32+len(data))
	binary.BigEndian.PutUint32(b[0:], uint32(32+len(data)))
	copy(b[4:], "pssh") // version 0: no KID list
	binary.BigEndian.PutUint32(b[28:], uint32(len(data)))
	return append(b, data...)
}

// WidevinePsshData's key_id (field 2) is found after fixed-size fields.
func TestPsshKeyIDSkipsFixedSizeFields(t *testing.T) {
	kid := bytes.Repeat([]byte{0xAB}, 16)
	data := []byte{0x0D, 1, 2, 3, 4}                           // field 1, fixed32
	data = append(data, 0x19, 1, 2, 3, 4, 5, 6, 7, 8)          // field 3, fixed64
	data = append(data, 0x08, 0x01)                            // field 1, varint
	data = append(data, 0x12, 0x03, 'a', 'b', 'c')             // field 2, not 16 bytes
	data = append(data, append([]byte{0x12, 0x10}, kid...)...) // field 2, the key ID
	if got := psshKeyID(psshBox(data)); !bytes.Equal(got, kid) {
		t.Errorf("psshKeyID = %x, want %x", got, kid)
	}
	if got := psshKeyID(psshBox([]byte{0x0D, 1, 2})); got != nil {
		t.Errorf("truncated fixed32: psshKeyID = %x, want nil", got)
	}
}
