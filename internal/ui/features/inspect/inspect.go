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
)

// ActionID is the registered action's ID prefix; each kind has its own
// ("inspect.song"), as an action targets one kind.
const ActionID = "inspect."

// loadTimeout bounds the cache and database reads behind one Inspect.
const loadTimeout = 10 * time.Second

// The panel's tabs.
const (
	tabOverview = iota
	tabApple
	tabLibrary
	tabAddon
	tabCount
)

// inspector is the feature's state: the one open panel.
type inspector struct {
	env *module.Env
	// inspect is InspectService.Inspect; tests replace it.
	inspect func(ctx context.Context, q services.InspectQuery) (services.InspectReport, error)

	open    bool
	item    kit.Item
	query   services.InspectQuery
	loading bool
	// err is why the report could not be read; the panel words it with a
	// Retry.
	err    error
	report services.InspectReport
	// data is the report as the tabs show it, built off the main thread.
	data *panelData
	// tab is the chosen tab; view says whether Overview is the ledger (0)
	// or the JSON of what Sona mapped (1).
	tab, view int
	// lists are the ledgers' places and choices (Overview, Library).
	lists [2]ledgerState
	// wantFilter asks the filter of the tab shown for the focus (Ctrl+F).
	wantFilter bool
	// gen tells a late answer from one for an item opened since.
	gen int
}

// ledgerState is a ledger's list state and the row chosen.
type ledgerState struct {
	list   ui.ListState
	chosen int
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
	in.open, in.item, in.query = true, item, queryOf(item)
	in.tab, in.view, in.wantFilter = tabOverview, 0, false
	in.lists = [2]ledgerState{{chosen: -1}, {chosen: -1}}
	in.load()
}

// load reads the report for the item shown: the panel shows its skeleton
// until it lands, or says why it could not and offers Retry.
func (in *inspector) load() {
	in.gen++
	gen := in.gen
	in.loading, in.err, in.report, in.data = true, nil, services.InspectReport{}, nil
	if in.inspect == nil {
		in.loading = false
		return
	}
	q := in.query
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
		defer cancel()
		rep, err := in.inspect(ctx, q)
		var data *panelData
		if err == nil {
			data = buildData(rep)
		}
		in.env.Post(func() {
			if gen != in.gen {
				return
			}
			in.loading, in.report, in.err, in.data = false, rep, err, data
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

// id is the item's ID in its own source.
func (in *inspector) id() string { return in.item.ID() }

// raw is s as JSON to embed, nil when it is not.
func raw(s string) json.RawMessage {
	if s == "" || !json.Valid([]byte(s)) {
		return nil
	}
	return json.RawMessage(s)
}

// addonJSON is the add-on's view of the item as one JSON document: the
// invoke target and, for a song, the now-playing track.
func (in *inspector) addonJSON() string { return addonDoc(in.report.Addon) }

// addonDoc is what an add-on is sent for an item as one JSON document, "" when
// it is sent nothing.
func addonDoc(a services.InspectAddon) string {
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
	if t := raw(a.Target); t != nil {
		doc.Invoke = &invoke{t}
	}
	if t := raw(a.Track); t != nil {
		doc.NowPlaying = &nowPlaying{t}
	}
	if doc.Invoke == nil && doc.NowPlaying == nil {
		return ""
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

// asJSON is the whole report as one JSON object, for Copy as JSON.
func (in *inspector) asJSON() string {
	var addon json.RawMessage
	if d := in.addonJSON(); d != "" {
		addon = json.RawMessage(d)
	}
	b, err := json.MarshalIndent(struct {
		Kind   string          `json:"kind"`
		ID     string          `json:"id"`
		URL    string          `json:"url,omitempty"`
		Art    json.RawMessage `json:"artwork,omitempty"`
		RawKey string          `json:"rawKey,omitempty"`
		Mapped json.RawMessage `json:"mapped,omitempty"`
		Raw    json.RawMessage `json:"raw,omitempty"`
		Row    json.RawMessage `json:"row,omitempty"`
		Addon  json.RawMessage `json:"addon,omitempty"`
	}{
		string(in.item.Kind), in.id(), in.report.URL, artJSON(in.report.Art), in.report.RawKey,
		raw(in.report.Mapped), raw(in.report.Raw), raw(in.report.Row), addon,
	}, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

// artJSON is the artwork as JSON, nil without a template.
func artJSON(a services.InspectArt) json.RawMessage {
	if a.Template == "" {
		return nil
	}
	b, err := json.Marshal(a)
	if err != nil {
		return nil
	}
	return b
}
