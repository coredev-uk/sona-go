//go:build dev

package inspect

import (
	"context"
	"encoding/json"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// ActionID is the registered action's ID prefix; each kind has its own
// ("inspect.song"), as an action targets one kind.
const ActionID = "inspect."

// loadTimeout bounds the cache and database reads behind one Inspect.
const loadTimeout = 10 * time.Second

// inspector is the feature's state: the one open panel.
type inspector struct {
	env *module.Env
	// inspect is InspectService.Inspect; tests replace it.
	inspect func(ctx context.Context, q services.InspectQuery) (services.InspectReport, error)

	open    bool
	item    kit.Item
	query   services.InspectQuery
	loading bool
	err     string
	report  services.InspectReport
	text    string // the panel's text, shown selectable
	shown   string // text as the field was last given it: edits are undone
	scroll  ui.ScrollState
	// gen tells a late answer from one for an item opened since.
	gen int
}

// New is the inspector module.
func New(env *module.Env) module.Module { return newInspector(env).module() }

func newInspector(env *module.Env) *inspector {
	in := &inspector{env: env}
	if env.Core != nil {
		in.inspect = services.Service[services.InspectService](env.Core).Inspect
	}
	return in
}

func (in *inspector) module() module.Module {
	var defs []kit.ActionDef
	for _, k := range []kit.ItemKind{kit.KindSong, kit.KindAlbum, kit.KindPlaylist, kit.KindArtist, kit.KindStation, kit.KindMusicVideo} {
		defs = append(defs, in.action(k))
	}
	return module.Module{
		ID:      "inspect",
		Actions: defs,
		Slots:   module.Slots{Layer: in.panel},
	}
}

// action is the item menu's Inspect for one kind (group Developer: after every
// other group, so it is the menu's last entry).
func (in *inspector) action(kind kit.ItemKind) kit.ActionDef {
	return kit.ActionDef{
		ID:     ActionID + string(kind),
		Target: kind,
		Label:  func(*kit.ActionContext) string { return "Inspect" },
		Icon:   "square-terminal",
		Group:  kit.GroupDeveloper,
		Run:    func(a *kit.ActionContext) { in.show(a.Item()) },
	}
}

// show opens the panel on item and asks Go for its report.
func (in *inspector) show(item kit.Item) {
	in.gen++
	gen := in.gen
	in.open, in.item, in.loading, in.err, in.report = true, item, true, "", services.InspectReport{}
	in.query = queryOf(item)
	in.text = in.render()
	in.scroll = ui.ScrollState{}
	if in.inspect == nil {
		in.loading = false
		return
	}
	q := in.query
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
		defer cancel()
		rep, err := in.inspect(ctx, q)
		in.env.Post(func() {
			if gen != in.gen {
				return
			}
			in.loading, in.report = false, rep
			if err != nil {
				in.err = err.Error()
			}
			in.text = in.render()
		})
	}()
}

// queryOf is what InspectService is asked about item.
func queryOf(item kit.Item) services.InspectQuery {
	q := services.InspectQuery{Kind: string(item.Kind), Item: item.Target}
	switch t := item.Target.(type) {
	case services.Song:
		q.CatalogID, q.LibraryID = t.CatalogID, t.LibraryID
	case services.Album:
		q.CatalogID, q.LibraryID = t.CatalogID, t.LibraryID
	case services.Playlist:
		q.CatalogID, q.LibraryID = t.CatalogID, t.LibraryID
	case services.Artist:
		q.CatalogID, q.LibraryID = t.CatalogID, t.LibraryID
	case services.MusicVideo:
		q.CatalogID, q.LibraryID = t.CatalogID, t.LibraryID
	case services.Station:
		q.CatalogID = t.ID
	}
	return q
}

// artURL is the item's artwork at its largest size ("" for none).
func artURL(item kit.Item) string {
	switch t := item.Target.(type) {
	case services.Song:
		return t.Art.Large
	case services.Album:
		return t.Art.Large
	case services.Playlist:
		return t.Art.Large
	case services.Artist:
		return t.Art.Large
	case services.Station:
		return t.Art.Large
	case services.MusicVideo:
		return t.Art.Large
	}
	return ""
}

// id is the item's ID in its own source (Copy ID).
func (in *inspector) id() string { return in.item.ID() }

// render is the panel's text: every section Go has for the item.
func (in *inspector) render() string {
	var s string
	add := func(title, body string) {
		if body != "" {
			s += "── " + title + " ──\n" + body + "\n\n"
		}
	}
	s += "kind: " + string(in.item.Kind) + "\nid: " + in.id() + "\nartwork: " + artURL(in.item) + "\n\n"
	switch {
	case in.loading:
		s += "Loading…\n"
	case in.err != "":
		s += "error: " + in.err + "\n\n"
	}
	add("Mapped item (what Sona shows)", in.report.Mapped)
	if in.report.Raw != "" {
		add("Apple's resource (catalog cache "+in.report.RawKey+")", in.report.Raw)
	} else if !in.loading {
		add("Apple's resource", "not in Go's catalog cache")
	}
	add("Library row (SQLite)", in.report.Row)
	return s
}

// asJSON is the whole report as one JSON object, for Copy as JSON.
func (in *inspector) asJSON() string {
	raw := func(s string) json.RawMessage {
		if s == "" || !json.Valid([]byte(s)) {
			return nil
		}
		return json.RawMessage(s)
	}
	b, err := json.MarshalIndent(struct {
		Kind    string          `json:"kind"`
		ID      string          `json:"id"`
		Artwork string          `json:"artwork,omitempty"`
		RawKey  string          `json:"rawKey,omitempty"`
		Mapped  json.RawMessage `json:"mapped,omitempty"`
		Raw     json.RawMessage `json:"raw,omitempty"`
		Row     json.RawMessage `json:"row,omitempty"`
	}{string(in.item.Kind), in.id(), artURL(in.item), in.report.RawKey, raw(in.report.Mapped), raw(in.report.Raw), raw(in.report.Row)}, "", "  ")
	if err != nil {
		return in.text
	}
	return string(b)
}

// panel is the dialog: plain, monospaced-feeling text in a scroll, three copy
// buttons. Not designed for end users.
func (in *inspector) panel(c *ui.Context) {
	if in.text != in.shown {
		in.shown = in.text // the field edits its own copy; a new report replaces it
	}
	_, wh := c.Size()
	kit.Dialog(c.Key("inspect"), &in.open, kit.DialogOpts{
		Title: "Inspect " + in.item.Name(), Description: "Development build: what Sona mapped and what Apple sent",
		MaxWidth: 880, Top: wh * 0.06,
	}, func() {
		t := theme.Current().Type["secondary"]
		area := ui.TextAreaBase(c.Key("text"), &in.shown)
		area.FillWidth().MinHeight(200).MaxHeight(max(wh*0.7-120, 200)).Padding(8, 12).FontSize(t.Size).
			TextColor(theme.C("ink")).Background(theme.C("raised")).Radius(theme.Current().Radius["md"]).Label("Item properties")
		if in.shown != in.text {
			in.shown = in.text // read-only: any edit is undone
		}
		ui.Row(c.Key("actions")).Gap(8).Children(func() {
			btn := func(key, label string, text func() string, enabled bool) {
				if kit.Button(c.Key(key), label, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Disabled: !enabled}).Clicked() {
					in.env.Copy(text())
				}
			}
			btn("copy-json", "Copy as JSON", in.asJSON, !in.loading)
			btn("copy-id", "Copy ID", in.id, in.id() != "")
			btn("copy-art", "Copy artwork URL", func() string { return artURL(in.item) }, artURL(in.item) != "")
		})
	})
}
