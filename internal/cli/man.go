package cli

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
)

// code is `text` in a message: a command or file name, set in bold.
var code = regexp.MustCompile("`([^`]+)`")

// roff escapes text for a roff line: backslashes and hyphens, `code`
// in bold, and a leading dot or quote that would read as a request.
func roff(text string) string {
	s := strings.NewReplacer(`\`, `\e`, "-", `\-`).Replace(text)
	s = code.ReplaceAllString(s, `\fB$1\fR`)
	if strings.HasPrefix(s, ".") || strings.HasPrefix(s, "'") {
		s = `\&` + s
	}
	return s
}

// writeMan prints the manual page, sona(1), in roff, for Sona version.
func writeMan(w io.Writer, p *l10n.Printer, version string) error {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	text := func(m l10n.Message) { line("%s", roff(p.T(m))) }
	section := func(title l10n.Message) { line(".SH %s", strings.ToUpper(roff(p.T(title)))) }

	line(`.TH SONA 1 "" "Sona %s" "User Commands"`, roff(version))
	section(msgManNameSection)
	line(`sona \- %s`, roff(p.T(msgManName)))
	section(msgManSynopsis)
	line(".nf")
	for _, s := range []string{launchLine, usageLine, versionLine} {
		line("%s", roff(s))
	}
	line(".fi")
	section(msgManDescriptionSection)
	text(msgManDescription)
	section(msgManLaunch)
	text(msgManLaunchText)

	section(msgCommands)
	for _, c := range commands {
		line(".TP")
		line(`.B "%s"`, roff("sona "+c.synopsis()))
		text(c.help)
		if c.args == "" && len(c.flags) == 0 {
			continue
		}
		line(".RS")
		if c.args != "" {
			line(".TP")
			line(`.I "%s"`, roff(c.args))
			text(c.argHelp)
		}
		for _, f := range c.flags {
			line(".TP")
			line(`.B "%s"`, roff(f.synopsis()))
			line("%s", roff(f.text(p)))
		}
		line(".RE")
	}
	line(".PP")
	line("%s", roff(`-h, --help`)+" "+roff(p.T(msgFlagHelp)))

	section(msgManJSON)
	text(msgManJSONText)
	section(msgManFormat)
	text(msgManFormatText)
	section(msgExitStatus)
	for _, e := range exitStatuses {
		line(".TP")
		line(".B %d", e.code)
		text(e.text)
	}
	section(msgManFiles)
	line("%s", roff(p.Sprintf(msgManFilesText, stateDirMac, stateDirWindows)))
	section(msgManSeeAlso)
	line("%s", roff(seeAlsoURL))
	_, err := io.WriteString(w, b.String())
	return err
}
