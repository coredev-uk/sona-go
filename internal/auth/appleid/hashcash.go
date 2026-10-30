package appleid

import (
	"context"
	"crypto/sha1"
	"fmt"
	"math/bits"
	"strconv"
	"time"
)

// hashcash mints the X-APPLE-HC proof-of-work stamp Apple's widget sends
// with signin/complete (generateHashcash.js): "1:<bits>:<UTC
// yyyymmddhhmmss>:<challenge>::<counter>", where SHA-1 of the stamp starts
// with at least <bits> zero bits. challenge and bits come from the
// X-Apple-HC-Challenge and X-Apple-HC-Bits response headers.
func hashcash(ctx context.Context, challenge string, nbits int, now time.Time) (string, error) {
	if nbits < 0 || nbits > 32 {
		return "", fmt.Errorf("appleid: hashcash bits %d out of range", nbits)
	}
	prefix := fmt.Sprintf("1:%d:%s:%s::", nbits, now.UTC().Format("20060102150405"), challenge)
	buf := []byte(prefix)
	for n := 0; ; n++ {
		if n&0xffff == 0 && ctx.Err() != nil {
			return "", ctx.Err()
		}
		buf = strconv.AppendInt(buf[:len(prefix)], int64(n), 10)
		if leadingZeroBits(sha1.Sum(buf)) >= nbits {
			return string(buf), nil
		}
	}
}

func leadingZeroBits(sum [sha1.Size]byte) int {
	z := 0
	for _, b := range sum {
		if b != 0 {
			return z + bits.LeadingZeros8(b)
		}
		z += 8
	}
	return z
}
