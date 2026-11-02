package applestrings

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"

	sonai18n "github.com/coredev-uk/sona-go/frontend/src/shared/i18n"
	"github.com/coredev-uk/sona-go/internal/i18n/po"
)

// Mapping is one reviewed pair: a Sona message (its English msgid and
// context, which Lingui hashes into the runtime ID) and the Apple key that
// translates it. A plural mapping names the key base; its `.one`,
// `.other`, ... forms are read. Placeholders maps Apple's names to Sona's.
// apple-keys.json holds these and no Apple text.
type Mapping struct {
	Message      string            `json:"message"`
	Context      string            `json:"context,omitempty"`
	Key          string            `json:"key"`
	Plural       bool              `json:"plural,omitempty"`
	Placeholders map[string]string `json:"placeholders,omitempty"`
}

// MsgKey is the Sona message's identity in the catalogues.
func (m Mapping) MsgKey() po.Key { return po.Key{ID: m.Message, Context: m.Context} }

// ID is the Sona message's Lingui runtime ID.
func (m Mapping) ID() string { return MessageID(m.Message, m.Context) }

// MessageID is Lingui's runtime ID for a message (@lingui/message-utils
// generateMessageId): the first six characters of the URL-safe base64
// SHA-256 of the msgid, a unit separator and the context.
func MessageID(msg, context string) string {
	sum := sha256.Sum256([]byte(msg + "\x1f" + context))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:6]
}

// DecodeMappings reads apple-keys.json strictly: an unknown field, such
// as Apple text pasted into the mapping, is an error.
func DecodeMappings(b []byte) ([]Mapping, error) {
	var ms []Mapping
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ms); err != nil {
		return nil, err
	}
	return ms, nil
}

// ValidateMappings checks the reviewed mapping against Sona's catalogue:
// each message exists once, parses, and its placeholders rename onto its
// own arguments.
func ValidateMappings(ms []Mapping, sona map[po.Key]Message) error {
	var errs []error
	seen := map[po.Key]bool{}
	for _, m := range ms {
		k := m.MsgKey()
		if seen[k] {
			errs = append(errs, fmt.Errorf("mapped twice: %q", m.Message))
		}
		seen[k] = true
		msg, ok := sona[k]
		if !ok {
			errs = append(errs, fmt.Errorf("mapped message is not in en.po (changed or removed? re-run match): %q", m.Message))
			continue
		}
		if msg.IsPlural() != m.Plural {
			errs = append(errs, fmt.Errorf("plural flag does not fit the message: %q", m.Message))
		}
		names := msg.Placeholders()
		for an, sn := range m.Placeholders {
			if !slices.Contains(names, sn) {
				errs = append(errs, fmt.Errorf("placeholder %s → {%s} is not an argument of %q", an, sn, m.Message))
			}
		}
	}
	return errors.Join(errs...)
}

// ErrMissing is a mapped key the bundle lacks (a plural key without its
// `.other` form).
var ErrMissing = errors.New("key not in Apple's bundle")

// maxMessageBytes caps one converted message; Apple's longest mapped
// strings are a few hundred bytes.
const maxMessageBytes = 4 << 10

// Translate converts the mapped key's text in b into the Lingui (ICU)
// message for the Sona message: Apple's `@@name@@` placeholders become
// Sona's `{name}` through the renames, a plural key's forms an ICU plural
// whose count is `#`, and ICU syntax in Apple's text is quoted. It refuses
// a message whose placeholders do not agree with Sona's (ToICU) or that is
// over the size cap.
func (m Mapping) Translate(b Bundle) (string, error) {
	sona, ok := ParseSona(m.Message)
	if !ok {
		return "", errors.New("the Sona message cannot map to one key")
	}
	if sona.IsPlural() != m.Plural {
		return "", errors.New("plural flag does not fit the message")
	}
	apple, ok := AppleMessage(b, m.Key, m.Plural)
	if !ok {
		return "", ErrMissing
	}
	s, err := ToICU(apple, sona, m.Placeholders)
	if err != nil {
		return "", err
	}
	if len(s) > maxMessageBytes {
		return "", fmt.Errorf("%d bytes, over %d", len(s), maxMessageBytes)
	}
	return s, nil
}

// Converted is a bundle's translations of the mapped messages.
type Converted struct {
	// Messages are keyed by Sona's Lingui message ID; only mapped keys
	// that passed Translate's checks are here.
	Messages map[string]string
	Missing  []string // mapped keys the bundle lacks
	Invalid  []string // "key: reason" for refused ones
}

// Convert translates every mapping from b.
func Convert(b Bundle, ms []Mapping) Converted {
	c := Converted{Messages: make(map[string]string, len(ms))}
	for _, m := range ms {
		s, err := m.Translate(b)
		switch {
		case errors.Is(err, ErrMissing):
			c.Missing = append(c.Missing, m.Key)
		case err != nil:
			c.Invalid = append(c.Invalid, m.Key+": "+err.Error())
		default:
			c.Messages[m.ID()] = s
		}
	}
	return c
}

// Catalogue is what conversion and coverage read from Sona's own files.
type Catalogue struct {
	Mappings []Mapping
	// Messages is how many messages Sona has (en.po), the denominator of
	// a locale's coverage.
	Messages int
	// Version identifies the mapping: converted messages cached under
	// another version are re-converted.
	Version string
}

// Embedded is the catalogue built into Sona (frontend/src/shared/i18n). A
// mapped message that is not in en.po is left out (a Go test keeps the
// committed files consistent).
var Embedded = sync.OnceValues(func() (Catalogue, error) {
	return NewCatalogue(sonai18n.AppleKeys, sonai18n.SourceCatalogue)
})

// NewCatalogue reads a mapping and Sona's English catalogue.
func NewCatalogue(mapping, source []byte) (Catalogue, error) {
	f, err := po.Parse(source)
	if err != nil {
		return Catalogue{}, fmt.Errorf("en.po: %w", err)
	}
	ms, err := DecodeMappings(mapping)
	if err != nil {
		return Catalogue{}, fmt.Errorf("apple-keys.json: %w", err)
	}
	keys := make(map[po.Key]bool, len(f.Entries))
	c := Catalogue{}
	for _, e := range f.Entries {
		if !e.Obsolete {
			keys[e.Key()] = true
			c.Messages++
		}
	}
	for _, m := range ms {
		if keys[m.MsgKey()] {
			c.Mappings = append(c.Mappings, m)
		}
	}
	sum := sha256.Sum256(mapping)
	c.Version = hex.EncodeToString(sum[:8])
	return c, nil
}
