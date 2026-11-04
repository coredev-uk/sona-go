package theme

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEveryIconParsesInMyGo(t *testing.T) {
	names := IconNames()
	if len(names) < 90 {
		t.Fatalf("%d icons embedded", len(names))
	}
	for _, n := range names {
		s := Icon(n)
		if w, h := s.Size(); w != 24 || h != 24 {
			t.Errorf("%s is %v×%v", n, w, h)
		}
	}
}

func TestIconNamesResolveEveryAlias(t *testing.T) {
	f, err := os.Open("icons/ALIASES.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			continue
		}
		n++
		got, ok := IconName(fields[0])
		if !ok || got != fields[1] {
			t.Errorf("%s resolves to %q (%v), want %s", fields[0], got, ok, fields[1])
		}
	}
	if n < 100 {
		t.Errorf("%d aliases", n)
	}
}

func TestIconNameForms(t *testing.T) {
	for in, want := range map[string]string{
		"play": "play", "chevron-right": "chevron-right", "ChevronRight": "chevron-right",
		"ChevronRightIcon": "chevron-right", "Maximize2": "maximize-2", "Disc3": "disc-3",
		"Loader2Icon": "loader-circle", "loader-2": "loader-circle", "History": "rotate-ccw-clock",
		"Music": "eighth-notes", "ListMusic": "list-music",
	} {
		if got, ok := IconName(in); !ok || got != want {
			t.Errorf("IconName(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := IconName("not-an-icon"); ok {
		t.Error("an unknown name resolved")
	}
}

func TestIconIsCachedAndUnknownPanicsInTests(t *testing.T) {
	if Icon("play") != Icon("Play") {
		t.Error("one icon, two parses")
	}
	defer func() {
		if recover() == nil {
			t.Error("an unknown icon did not panic under test")
		}
	}()
	Icon("no-such-icon")
}

// The names kit and the pages will ask for are the ones v0.2.0 imported from
// lucide-react; spot-check what the Button and the nav use.
func TestIconsDrawInAView(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		Apply(c)
		ui.Row(c).Gap(8).Children(func() {
			ui.Icon(c, Icon("play")).FontSize(16)
			ui.Icon(c, Icon("list-music")).FontSize(16)
			ui.Text(c, "Songs")
		})
	}, 200, 40)
	tt.Frame()
	if !tt.HasText("Songs") {
		t.Error("view did not draw")
	}
}
