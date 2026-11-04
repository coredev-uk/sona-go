package kit

import (
	"strconv"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// ToastKind picks a toast's icon (sonner's types).
type ToastKind string

const (
	ToastPlain   ToastKind = ""
	ToastSuccess ToastKind = "success" // circle-check
	ToastInfo    ToastKind = "info"
	ToastWarning ToastKind = "warning" // triangle-alert
	ToastError   ToastKind = "error"   // octagon-x
	ToastLoading ToastKind = "loading" // a spinning loader
)

// ToastSpec is one toast (sonner's toast(title, {description, id…})).
type ToastSpec struct {
	// ID names the toast: showing one with the ID of a toast on screen
	// replaces it (a held key never stacks messages). Made up when empty.
	ID string
	// Title and Description are what it says; screen readers read both,
	// politely, as it shows.
	Title, Description string
	Kind               ToastKind
	// Duration is how long it shows: 4 s when zero, until closed when
	// negative.
	Duration time.Duration
	// Action labels a button that runs OnAction and closes the toast.
	Action   string
	OnAction func()
}

// The toast queue: Toast may be called from any goroutine; the Toaster
// moves what is queued into the window on the next frame (a goroutine
// posts a redraw, as for any state it changes).
var toasts struct {
	mu     sync.Mutex
	queue  []ui.Toast
	closed []string
	next   int
	post   store.Poster
	// shown are the titles of the toasts the window shows, by ID, as the
	// last frame of the Toaster drew them.
	shown map[string]string
}

// SetToastPoster sets how a toast raised off the main thread asks for a
// frame (the shell's Poster, which redraws the windows).
func SetToastPoster(p store.Poster) {
	toasts.mu.Lock()
	toasts.post = p
	toasts.mu.Unlock()
}

// overlaysWake asks for a frame, so a toast raised from a goroutine shows without
// waiting for an unrelated redraw.
func overlaysWake() {
	toasts.mu.Lock()
	p := toasts.post
	toasts.mu.Unlock()
	if p != nil {
		p(func() {})
	}
}

// ShowToast queues a toast and returns its ID. Safe from any goroutine; the
// Toaster shows it on the next frame (SetToastPoster makes it ask for one).
func ShowToast(t ToastSpec) string {
	toasts.mu.Lock()
	defer overlaysWake()
	defer toasts.mu.Unlock()
	if t.ID == "" {
		toasts.next++
		t.ID = "kit-toast-" + strconv.Itoa(toasts.next)
	}
	toasts.queue = append(toasts.queue, ui.Toast{
		ID: t.ID, Title: t.Title, Description: t.Description, Type: string(t.Kind), Timeout: t.Duration,
		Action: t.Action, OnAction: t.OnAction,
	})
	return t.ID
}

// ToastSuccessf and its siblings are the common shapes: a title and an
// optional description.
func ToastSuccessf(title, description string) string {
	return ShowToast(ToastSpec{Title: title, Description: description, Kind: ToastSuccess})
}

// ToastErrorf shows an error toast: what failed, and why as the description.
func ToastErrorf(title, description string) string {
	return ShowToast(ToastSpec{Title: title, Description: description, Kind: ToastError})
}

// CloseToast closes a toast by ID ("" closes every toast).
func CloseToast(id string) {
	toasts.mu.Lock()
	toasts.closed = append(toasts.closed, id)
	toasts.mu.Unlock()
	overlaysWake()
}

// Unavailable says why a command that does not apply right now did nothing
// ("Not available while listening to a live station"): one polite toast
// with a fixed ID, so a held key never stacks messages.
//
// A key held down asks again on every repeat, and MyGo announces each toast
// it is given: the reason is raised again only once the message is gone (or
// is another), so it is read once per hold.
func Unavailable(reason string) {
	const id = "command-unavailable"
	toasts.mu.Lock()
	same := toasts.shown[id] == reason
	for _, q := range toasts.queue {
		same = same || q.ID == id && q.Title == reason
	}
	toasts.mu.Unlock()
	if same {
		return
	}
	ShowToast(ToastSpec{ID: id, Title: reason})
}

// Toaster draws the window's toasts: popover colours, the theme's radius
// and the kind's icon (check, info, warning, error octagon, spinning
// loader), at the bottom right, 16 from the right and 88 from the bottom
// (above the 72 DIP player bar). Mount it once, in the shell. The region is
// labelled "Notifications"; titles and descriptions are announced politely.
func Toaster(c *ui.Context) {
	toasts.mu.Lock()
	queue, closed := toasts.queue, toasts.closed
	toasts.queue, toasts.closed = nil, nil
	toasts.mu.Unlock()
	for _, id := range closed {
		c.CloseToast(id)
	}
	for _, t := range queue {
		c.AddToast(t)
	}
	tk := theme.Current()
	ui.ToastViewportBase(c.Key("toaster"), func(viewport ui.Element, list []ui.Toast) {
		toasts.mu.Lock()
		toasts.shown = map[string]string{}
		for _, t := range list {
			toasts.shown[t.ID] = t.Title
		}
		toasts.mu.Unlock()
		viewport.Padding(0, 16, 88, 16).AlignItems(ui.End).Gap(8)
		if len(list) > 0 {
			// The region is named while it holds a toast; MyGo announces each
			// one itself, so nothing has to exist before the message.
			viewport.Role(ui.RoleGroup).Label(i18n.T("Notifications"))
		}
		for _, t := range list {
			part := ui.ToastBase(c, t)
			sh := tk.Shadow["float"]
			part.Root.Row().Width(356).Padding(16).Gap(6).AlignItems(ui.Start).Radius(tk.Radius["lg"]).
				Background(theme.C("popover")).Border(1, theme.C("border")).TextColor(theme.C("popover-foreground")).
				Shadow(sh.X, sh.Y, sh.Blur, sh.Spread, sh.Color)
			if !theme.ReducedMotion() {
				m := tk.Motion["float"]
				part.Root.Transition(ui.ElementTransition{Enter: &ui.Motion{Y: 8}, Exit: &ui.Motion{}, Duration: m.Duration, Ease: m.Ease})
			}
			part.Root.Children(func() {
				toastIcon(c, ToastKind(t.Type))
				ui.Column(c.Key("txt")).Grow(1).MinWidth(0).Gap(2).Children(func() {
					overlaysText(c.Key("title"), t.Title, "secondary").FontWeight(500).TextColor(theme.C("popover-foreground"))
					if t.Description != "" {
						overlaysText(c.Key("desc"), t.Description, "secondary").TextColor(theme.C("muted-foreground"))
					}
				})
				if t.Action != "" {
					part.ActionButton().Label(t.Action).Padding(0, 8).Height(24).Radius(tk.Radius["md"]).Background(theme.C("primary")).
						TextColor(theme.C("primary-foreground")).Center().Children(func() {
						overlaysText(c.Key("act"), t.Action, "meta").FontWeight(500)
					})
				}
			})
		}
	})
}

// toastIcon is the icon of a toast kind: 16 DIPs, in the text colour; the
// loader spins.
func toastIcon(c *ui.Context, k ToastKind) {
	name := ""
	switch k {
	case ToastSuccess:
		name = "circle-check"
	case ToastInfo:
		name = "info"
	case ToastWarning:
		name = "triangle-alert"
	case ToastError:
		name = "octagon-x"
	case ToastLoading:
		name = "loader-circle"
	}
	if name == "" {
		return
	}
	ic := ui.Icon(c.Key("icon"), theme.Icon(name)).FontSize(16).Margin(2, 0, 0, 0).Shrink(0)
	if k == ToastLoading && !theme.ReducedMotion() {
		ic.Rotate(ic.Loop("spin", time.Second, ui.Linear) * 360)
	}
}
