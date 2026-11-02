package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
)

// helpWidth is where help text wraps.
const helpWidth = 78

// exitStatuses are the exit statuses in help and the manual page.
var exitStatuses = []struct {
	code int
	text l10n.Message
}{
	{ExitOK, msgExitOK},
	{ExitFailed, msgExitFailed},
	{ExitUsage, msgExitUsage},
	{ExitNotRunning, msgExitNotRunning},
}

// synopsis is a command's usage line, without "sona ".
func (c *command) synopsis() string {
	parts := []string{c.name}
	if c.args != "" {
		parts = append(parts, c.args)
	}
	if len(c.flags) > 0 {
		parts = append(parts, "[options]")
	}
	return strings.Join(parts, " ")
}

// flagSynopsis is "--name VALUE" (or "--name").
func (f flag) synopsis() string {
	if f.value == "" {
		return "--" + f.name
	}
	return "--" + f.name + " " + f.value
}

// text is a flag's description with its choices.
func (f flag) text(p *l10n.Printer) string {
	if len(f.choices) == 0 {
		return p.T(f.help)
	}
	return p.Sprintf(msgFieldLine, p.T(f.help), strings.Join(f.choices, ", "))
}

// writeOverview prints `sona help`: the usage lines, every command and
// the exit statuses.
func writeOverview(w io.Writer, p *l10n.Printer) error {
	var b strings.Builder
	usage := p.T(msgUsage)
	fmt.Fprintf(&b, "%s: %s\n", usage, usageLine)
	pad := strings.Repeat(" ", len([]rune(usage))+2)
	fmt.Fprintf(&b, "%s%s\n%s%s\n\n%s:\n", pad, launchLine, pad, versionLine, p.T(msgCommands))
	width := 0
	for _, c := range commands {
		width = max(width, len(c.name))
	}
	for _, c := range commands {
		fmt.Fprintf(&b, "  %-*s  %s\n", width, c.name, p.T(c.summary))
	}
	b.WriteString("\n" + wrap(p.T(msgHelpFooter), "", helpWidth) + "\n")
	writeExitStatuses(&b, p)
	_, err := io.WriteString(w, b.String())
	return err
}

// writeHelp prints one command's help.
func writeHelp(w io.Writer, p *l10n.Printer, c *command) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: sona %s\n\n%s\n", p.T(msgUsage), c.synopsis(), wrap(p.T(c.help), "", helpWidth))
	if c.args != "" {
		fmt.Fprintf(&b, "\n%s:\n  %s  %s\n", p.T(msgArguments), c.args, p.T(c.argHelp))
	}
	rows := [][2]string{}
	for _, f := range c.flags {
		rows = append(rows, [2]string{f.synopsis(), f.text(p)})
	}
	rows = append(rows, [2]string{"-h, --help", p.T(msgFlagHelp)})
	width := 0
	for _, r := range rows {
		width = max(width, len(r[0]))
	}
	fmt.Fprintf(&b, "\n%s:\n", p.T(msgOptions))
	for _, r := range rows {
		indent := strings.Repeat(" ", width+4)
		fmt.Fprintf(&b, "  %-*s  %s\n", width, r[0], strings.TrimPrefix(wrap(r[1], indent, helpWidth-width-4), indent))
	}
	if !c.local {
		writeExitStatuses(&b, p)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeExitStatuses(b *strings.Builder, p *l10n.Printer) {
	fmt.Fprintf(b, "\n%s:\n", p.T(msgExitStatus))
	for _, e := range exitStatuses {
		fmt.Fprintf(b, "  %d  %s\n", e.code, p.T(e.text))
	}
}

// wrap breaks text into lines of at most width runes after indent (the
// first line included).
func wrap(text, indent string, width int) string {
	var b strings.Builder
	n := 0
	for i, word := range strings.Fields(text) {
		l := len([]rune(word))
		switch {
		case i == 0:
			b.WriteString(indent)
		case n+1+l > width:
			b.WriteString("\n" + indent)
			n = 0
		default:
			b.WriteByte(' ')
			n++
		}
		b.WriteString(word)
		n += l
	}
	return b.String()
}
