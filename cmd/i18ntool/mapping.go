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
)

// mapping is one reviewed pair: a Sona message (its English msgid and
// context, which Lingui hashes into the runtime ID) and the Apple key that
// translates it. A plural mapping names the key base; the import reads its
// `.one`, `.other`, ... forms. Placeholders maps Apple's names to Sona's.
// The file holds no Apple text.
type mapping struct {
	Message      string            `json:"message"`
	Context      string            `json:"context,omitempty"`
	Key          string            `json:"key"`
	Plural       bool              `json:"plural,omitempty"`
	Placeholders map[string]string `json:"placeholders,omitempty"`
}

func (m mapping) msgKey() msgKey { return msgKey{m.Message, m.Context} }

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
	mapping
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

func sortMappings(ms []mapping) {
	slices.SortFunc(ms, func(a, b mapping) int {
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

// validateMappings checks the reviewed file against Sona's catalogue: each
// message exists once, parses, and its placeholders rename onto its own
// arguments.
func validateMappings(ms []mapping, sona map[msgKey]message) error {
	var errs []error
	seen := map[msgKey]bool{}
	for _, m := range ms {
		k := m.msgKey()
		if seen[k] {
			errs = append(errs, fmt.Errorf("mapped twice: %q", m.Message))
		}
		seen[k] = true
		msg, ok := sona[k]
		if !ok {
			errs = append(errs, fmt.Errorf("mapped message is not in en.po (changed or removed? re-run match): %q", m.Message))
			continue
		}
		if msg.isPlural() != m.Plural {
			errs = append(errs, fmt.Errorf("plural flag does not fit the message: %q", m.Message))
		}
		names := msg.placeholders()
		for an, sn := range m.Placeholders {
			if !slices.Contains(names, sn) {
				errs = append(errs, fmt.Errorf("placeholder %s → {%s} is not an argument of %q", an, sn, m.Message))
			}
		}
	}
	return errors.Join(errs...)
}
