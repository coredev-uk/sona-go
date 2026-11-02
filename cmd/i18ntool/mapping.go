package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/coredev-uk/sona-go/internal/i18n/applestrings"
)

// rejection is a reviewed match that does not hold in Sona's context; the
// matcher never proposes the same message and key again.
type rejection struct {
	Message string `json:"message"`
	Context string `json:"context,omitempty"`
	Key     string `json:"key"`
	Reason  string `json:"reason"`
}

// proposal is the matcher's output for review. Decision is filled in by the
// reviewer ("accept" or "reject", with Reason for a rejection) and applied
// with `match -apply`, which copies only the message, key and placeholders.
type proposal struct {
	applestrings.Mapping
	Match        string   `json:"match"`
	Apple        string   `json:"apple"`
	Alternatives []string `json:"alternatives,omitempty"`
	Decision     string   `json:"decision"`
	Reason       string   `json:"reason,omitempty"`
}

// readJSON decodes path strictly (an unknown field, such as Apple text
// pasted into the mapping, is an error). A missing file is empty.
func readJSON[T any](path string) ([]T, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var v []T
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return v, nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func sortMappings(ms []applestrings.Mapping) {
	slices.SortFunc(ms, func(a, b applestrings.Mapping) int {
		if c := strings.Compare(a.Message, b.Message); c != 0 {
			return c
		}
		return strings.Compare(a.Context, b.Context)
	})
}

func sortRejections(rs []rejection) {
	slices.SortFunc(rs, func(a, b rejection) int {
		if c := strings.Compare(a.Message, b.Message); c != 0 {
			return c
		}
		if c := strings.Compare(a.Context, b.Context); c != 0 {
			return c
		}
		return strings.Compare(a.Key, b.Key)
	})
}
