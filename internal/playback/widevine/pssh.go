//go:build linux || darwin

package widevine

import (
	"encoding/binary"
	"errors"
)

// SystemID is the Widevine DRM system ID (edef8ba9-79d6-4ace-a3c8-27dcd51d21ed).
var SystemID = [16]byte{0xed, 0xef, 0x8b, 0xa9, 0x79, 0xd6, 0x4a, 0xce, 0xa3, 0xc8, 0x27, 0xdc, 0xd5, 0x1d, 0x21, 0xed}

// PSSH returns a version 0 Widevine 'pssh' box for one 16-byte key ID, the
// cenc init data MusicKit generates for Apple's ISO-23001-7 streams. Its
// WidevinePsshData sets algorithm AESCTR (field 1) and key_id (field 2).
func PSSH(kid []byte) ([]byte, error) {
	if len(kid) != 16 {
		return nil, errors.New("widevine: key ID must be 16 bytes")
	}
	data := append([]byte{0x08, 0x01, 0x12, 0x10}, kid...)
	box := make([]byte, 0, 32+len(data))
	box = binary.BigEndian.AppendUint32(box, uint32(32+len(data)))
	box = append(box, "pssh"...)
	box = binary.BigEndian.AppendUint32(box, 0) // version 0, flags 0
	box = append(box, SystemID[:]...)
	box = binary.BigEndian.AppendUint32(box, uint32(len(data)))
	return append(box, data...), nil
}

// InitData returns the cenc init data for a session: a 16-byte key ID is
// wrapped with PSSH (songs); anything longer must already be a Widevine
// 'pssh' box (live radio playlists carry the whole box) and is returned
// unchanged. This mirrors MusicKit, which treats 16 bytes as a key ID.
func InitData(b []byte) ([]byte, error) {
	if len(b) == 16 {
		return PSSH(b)
	}
	if len(b) < 32 || int(binary.BigEndian.Uint32(b)) != len(b) || string(b[4:8]) != "pssh" || [16]byte(b[12:28]) != SystemID {
		return nil, errors.New("widevine: init data is neither a key ID nor a Widevine pssh box")
	}
	return b, nil
}
