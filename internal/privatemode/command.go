package privatemode

import (
	"fmt"
	"strings"
)

// Usage is the `sona private` synopsis.
const Usage = "usage: sona private [on|off|toggle|status]"

// Command runs `sona private <args>` (forwarded to the running instance)
// and returns the line to print: the state after the command. No argument
// means status.
func (m *Mode) Command(args []string) (string, error) {
	verb := "status"
	if len(args) > 0 {
		verb = args[0]
	}
	if len(args) > 1 {
		return "", fmt.Errorf("%s", Usage)
	}
	var err error
	switch verb {
	case "on":
		_, err = m.Set(true)
	case "off":
		_, err = m.Set(false)
	case "toggle":
		_, err = m.Toggle()
	case "status":
	default:
		return "", fmt.Errorf("%s", Usage)
	}
	if err != nil {
		return "", err
	}
	return m.describe(), nil
}

// describe is the CLI's one-line state: "on" or "off" first, then where it
// comes from when that is not the saved setting.
func (m *Mode) describe() string {
	st := m.State()
	var b strings.Builder
	b.WriteString("private listening: ")
	if st.On {
		b.WriteString("on")
	} else {
		b.WriteString("off")
	}
	switch st.Source {
	case SourceDevelopment:
		saved := "off"
		if m.Saved() {
			saved = "on"
		}
		fmt.Fprintf(&b, " (development build: this session only; saved setting %s)", saved)
	case SourceEnvironment:
		b.WriteString(" (forced by " + EnvVar + "=1)")
	}
	return b.String()
}
