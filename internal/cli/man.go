package cli

import (
	"fmt"
	"io"
	"regexp"
	"strings"
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
func writeMan(w io.Writer, version string) error {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	section := func(title string) { line(".SH %s", strings.ToUpper(roff(title))) }

	line(`.TH SONA 1 "" "Sona %s" "User Commands"`, roff(version))
	section("Name")
	line(`sona \- %s`, roff(msgManName))
	section("Synopsis")
	line(".nf")
	for _, s := range []string{msgLaunchLine, msgUsageLine, msgVersionLine} {
		line("%s", roff(s))
	}
	line(".fi")
	section("Description")
	line("%s", roff(msgManDescription))
	section(msgManLaunch)
	line("%s", roff(msgManLaunchText))

	section(msgCommands)
	for _, c := range commands {
		line(".TP")
		line(`.B "%s"`, roff("sona "+c.synopsis()))
		line("%s", roff(c.help))
		if c.args == "" && len(c.flags) == 0 {
			continue
		}
		line(".RS")
		if c.args != "" {
			line(".TP")
			line(`.I "%s"`, roff(c.args))
			line("%s", roff(c.argHelp))
		}
		for _, f := range c.flags {
			line(".TP")
			line(`.B "%s"`, roff(f.synopsis()))
			line("%s", roff(f.text()))
		}
		line(".RE")
	}
	line(".PP")
	line("%s", roff(`-h, --help`)+" "+roff(msgFlagHelp))

	section(msgManJSON)
	line("%s", roff(msgManJSONText))
	section(msgManFormat)
	line("%s", roff(msgManFormatText))
	section(msgExitStatus)
	for _, e := range exitStatuses {
		line(".TP")
		line(".B %d", e.code)
		line("%s", roff(e.text))
	}
	section(msgManFiles)
	line("%s", roff(msgManFilesText))
	section(msgManSeeAlso)
	line("%s", roff(msgManSeeAlsoText))
	_, err := io.WriteString(w, b.String())
	return err
}
