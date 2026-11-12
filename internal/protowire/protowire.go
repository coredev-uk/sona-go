// Package protowire walks the fields of a protobuf message in wire format,
// for the few small messages Sona reads (CRX3 headers, Widevine PSSH data,
// licence responses) without a generated schema.
package protowire

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Wire types Walk reports to its callback; fixed-size fields (types 1 and 5)
// are skipped.
const (
	Varint = 0
	Bytes  = 2
)

// ErrStop ends a Walk early without an error: a callback returns it once it
// has what it looks for.
var ErrStop = errors.New("protowire: stop")

// Walk calls fn with every varint (wire type 0: varint is its value, data
// nil) and length-delimited (wire type 2: data is the field's bytes, a
// sub-slice of b) field of the message b, in order. Fixed 32- and 64-bit
// fields are skipped. Walk stops at the first error fn returns (ErrStop ends
// it with nil) and reports malformed or truncated input, a field number of 0
// or above 2^29, and wire types it does not know (groups).
func Walk(b []byte, fn func(num, wire, varint uint64, data []byte) error) error {
	for len(b) > 0 {
		tag, n := binary.Uvarint(b)
		if n <= 0 {
			return errors.New("malformed protobuf tag")
		}
		b = b[n:]
		num, wire := tag>>3, tag&7
		if num == 0 || num > 1<<29 {
			return errors.New("malformed protobuf field")
		}
		var err error
		switch wire {
		case Varint:
			v, n := binary.Uvarint(b)
			if n <= 0 {
				return errors.New("malformed protobuf varint")
			}
			b = b[n:]
			err = fn(num, wire, v, nil)
		case 1: // 64-bit
			if len(b) < 8 {
				return errors.New("truncated protobuf")
			}
			b = b[8:]
		case 5: // 32-bit
			if len(b) < 4 {
				return errors.New("truncated protobuf")
			}
			b = b[4:]
		case Bytes:
			l, n := binary.Uvarint(b)
			if n <= 0 || l > uint64(len(b)-n) {
				return errors.New("truncated protobuf")
			}
			data := b[n : n+int(l)]
			b = b[n+int(l):]
			err = fn(num, wire, 0, data)
		default:
			return fmt.Errorf("protobuf wire type %d", wire)
		}
		if err != nil {
			if err == ErrStop {
				return nil
			}
			return err
		}
	}
	return nil
}
