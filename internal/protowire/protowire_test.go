package protowire

import (
	"errors"
	"slices"
	"testing"
)

type seen struct {
	num, wire, varint uint64
	data              string
}

func walk(t *testing.T, b []byte) ([]seen, error) {
	t.Helper()
	var got []seen
	err := Walk(b, func(num, wire, v uint64, data []byte) error {
		got = append(got, seen{num, wire, v, string(data)})
		return nil
	})
	return got, err
}

func TestWalk(t *testing.T) {
	msg := []byte{
		0x08, 0x96, 0x01, // 1: varint 150
		0x15, 1, 2, 3, 4, // 2: fixed32, skipped
		0x19, 1, 2, 3, 4, 5, 6, 7, 8, // 3: fixed64, skipped
		0x12, 0x03, 'a', 'b', 'c', // 2: bytes "abc"
	}
	got, err := walk(t, msg)
	want := []seen{{1, 0, 150, ""}, {2, 2, 0, "abc"}}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("Walk = %v, %v; want %v", got, err, want)
	}
}

func TestWalkRejectsMalformed(t *testing.T) {
	for name, b := range map[string][]byte{
		"tag":            {0x80},
		"field zero":     {0x00, 0x01},
		"varint":         {0x08, 0x80},
		"fixed32":        {0x15, 1, 2},
		"fixed64":        {0x19, 1},
		"length":         {0x12, 0x05, 'a'},
		"length varint":  {0x12},
		"group":          {0x0b},
		"unknown wire 6": {0x0e},
	} {
		if _, err := walk(t, b); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestWalkStops(t *testing.T) {
	boom := errors.New("boom")
	b := []byte{0x08, 1, 0x08, 2}
	n := 0
	if err := Walk(b, func(_, _, _ uint64, _ []byte) error { n++; return ErrStop }); err != nil || n != 1 {
		t.Errorf("ErrStop: n=%d err=%v, want 1, nil", n, err)
	}
	if err := Walk(b, func(_, _, _ uint64, _ []byte) error { return boom }); err != boom {
		t.Errorf("callback error = %v, want boom", err)
	}
}
