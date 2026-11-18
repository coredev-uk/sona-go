package inspect

import (
	"context"
	"encoding/json"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
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
	// addonText and addonShown are the same for "What an add-on gets".
	addonText, addonShown string
	scroll                ui.ScrollState
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

// module registers nothing until Init has the setting: Inspect is offered in
// a development build, and in a release build while Settings → Diagnostics →
// Developer tools is on.
func (in *inspector) module() module.Module {
	return module.Module{
		ID:    "inspect",
		Init:  in.init,
		Slots: module.Slots{Layer: in.panel},
	}
}

// init offers Inspect as the setting says now, and follows it: the setting
// applies at once, with no restart.
func (in *inspector) init() {
	on := in.env.Dev
	if in.env.Core != nil {
		on = on || services.Service[services.SystemService](in.env.Core).Settings().DeveloperTools
	}
	in.offer(on)
	store.On(in.env.Bus, services.EventSettingsChanged, func(s config.Settings) { in.offer(in.env.Dev || s.DeveloperTools) })
}

// offer puts Inspect in every item menu, or takes it out: the registry holds
// the action only while it is offered, so a kind whose only action it is gets
// no ⋯ button while it is off.
func (in *inspector) offer(on bool) {
	var defs []kit.ActionDef
	if on {
		for _, k := range kinds {
			defs = append(defs, in.action(k))
		}
	}
	in.env.Actions.ReplacePrefix(ActionID, defs...)
	if !on {
		in.open = false
	}
}

// kinds are the item kinds that get an Inspect.
var kinds = []kit.ItemKind{kit.KindSong, kit.KindAlbum, kit.KindPlaylist, kit.KindArtist, kit.KindStation, kit.KindMusicVideo}

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
	in.text, in.addonText = in.render(), in.renderAddon()
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
			in.text, in.addonText = in.render(), in.renderAddon()
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

// addonNote says why the section shows what it does: an add-on is never sent
// a private listen, but this is the user's own inspection.
const addonNote = "Exactly what an add-on is sent for this item. Add-ons never see private listens or previews; this is your own inspection, so it shows the item regardless."

// renderAddon is the "What an add-on gets" text: the protocol's JSON for this
// item, as Go builds it.
func (in *inspector) renderAddon() string {
	if in.loading {
		return "Loading…\n"
	}
	var s string
	if t := in.report.Addon.Target; t != "" {
		s += "invoke · params.target\n" + t + "\n\n"
	} else {
		s += "invoke · no add-on action is offered on a " + string(in.item.Kind) + "\n\n"
	}
	if t := in.report.Addon.Track; t != "" {
		s += "now-playing · data.track\n" + t + "\n"
	}
	return s
}

// addonJSON is the add-on's view of the item as one JSON object, for the
// section's own Copy as JSON.
func (in *inspector) addonJSON() string {
	raw := func(s string) json.RawMessage {
		if s == "" || !json.Valid([]byte(s)) {
			return nil
		}
		return json.RawMessage(s)
	}
	type invoke struct {
		Target json.RawMessage `json:"target"`
	}
	type nowPlaying struct {
		Track json.RawMessage `json:"track"`
	}
	doc := struct {
		Invoke     *invoke     `json:"invoke,omitempty"`
		NowPlaying *nowPlaying `json:"now-playing,omitempty"`
	}{}
	if t := raw(in.report.Addon.Target); t != nil {
		doc.Invoke = &invoke{t}
	}
	if t := raw(in.report.Addon.Track); t != nil {
		doc.NowPlaying = &nowPlaying{t}
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return in.addonText
	}
	return string(b)
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

// panel is the dialog: plain text in a scroll, three copy buttons, then what
// an add-on gets with its own. Not designed for end users.
func (in *inspector) panel(c *ui.Context) {
	if in.text != in.shown {
		in.shown = in.text // the field edits its own copy; a new report replaces it
	}
	if in.addonText != in.addonShown {
		in.addonShown = in.addonText
	}
	_, wh := c.Size()
	kit.Dialog(c.Key("inspect"), &in.open, kit.DialogOpts{
		Title: "Inspect " + in.item.Name(), Description: "Developer tools: what Sona mapped, what Apple sent and what an add-on gets",
		MaxWidth: 880, Top: wh * 0.04,
	}, func() {
		t := theme.Current().Type["secondary"]
		field := func(key string, shown *string, text string, label string, maxHeight float32) {
			area := ui.TextAreaBase(c.Key(key), shown)
			area.FillWidth().MinHeight(100).MaxHeight(maxHeight).Padding(8, 12).FontSize(t.Size).
				TextColor(theme.C("ink")).Background(theme.C("raised")).Radius(theme.Current().Radius["md"]).Label(label)
			if *shown != text {
				*shown = text // read-only: any edit is undone
			}
		}
		field("text", &in.shown, in.text, "Item properties", max(wh*0.28, 140))
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
		ui.Column(c.Key("addon")).FillWidth().Gap(8).Margin(8, 0, 0, 0).Children(func() {
			theme.Text(c, "What an add-on gets", "heading").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(3)
			theme.Text(c, addonNote, "secondary").TextColor(theme.C("ink-muted"))
			field("addon-text", &in.addonShown, in.addonText, "What an add-on gets", max(wh*0.22, 110))
			ui.Row(c.Key("addon-actions")).Children(func() {
				if kit.Button(c.Key("copy-addon"), "Copy as JSON", kit.ButtonOpts{
					Variant: kit.Outline, Size: kit.SizeSM, Disabled: in.loading, A11yLabel: "Copy what an add-on gets as JSON",
				}).Clicked() {
					in.env.Copy(in.addonJSON())
				}
			})
		})
	})
}
