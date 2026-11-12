package settings

import (
	"context"
	"math"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// keyStorageUsage is the storage query (keys.storage).
var keyStorageUsage = store.Key{"storage"}

// artChoices are the artwork cache limits offered, in MiB.
var artChoices = []int{256, 512, 1024, 2048, 4096}

// storageAPI is what the section asks of StorageService.
type storageAPI interface {
	Usage(ctx context.Context) (services.StorageUsage, error)
	Clear(ctx context.Context) (services.ClearResult, error)
	Compact(ctx context.Context) (int64, error)
}

// storageSvc returns the StorageService, or the fake a test put in.
func (s *settings) storageSvc() storageAPI {
	if o := stateOf[storageAPI](s, "seam:storage"); *o != nil {
		return *o
	}
	return services.Service[services.StorageService](s.env.Core)
}

// storageState is what outlives a frame: the calls in flight, and whether
// the follow of settings:changed is set up.
type storageState struct {
	clearing, compacting bool
	following            bool
}

func (s *settings) usage() store.Result[services.StorageUsage] {
	return store.Use(s.env.Queries, keyStorageUsage, func(ctx context.Context) (services.StorageUsage, error) {
		return s.storageSvc().Usage(ctx)
	})
}

// storage is Settings → Storage (Storage.tsx): what Sona keeps on this
// computer first, as figures (the artwork cache against its limit, the
// catalogue cache, the database), then the limit, clearing, compacting and
// the settings file.
func (s *settings) storage(c *ui.Context) {
	st := stateOf[storageState](s, "storage")
	if !st.following {
		st.following = true
		// Go evicts down to a lower limit before the save returns, and its
		// settings:changed confirms the save: the usage is new then.
		store.On(s.env.Bus, services.EventSettingsChanged, func(config.Settings) {
			s.env.Queries.Invalidate(keyStorageUsage)
		})
	}
	cfg, usage := s.cfg(), s.usage()
	if err := storageErr(cfg.Err, usage.Err); err != nil {
		kit.ErrorBlock(c, i18n.T("Couldn't read Sona's storage"), err, kit.ErrorBlockOpts{
			OnRetry:  func() { s.env.Queries.Invalidate(store.KeySettings, keyStorageUsage) },
			Retrying: cfg.Fetching || usage.Fetching,
		})
		return
	}
	if !cfg.HasData || !usage.HasData || !s.caps().HasData {
		s.loading(c, 4)
		return
	}
	limit, u := cfg.Data.Storage.ArtCacheMiB, usage.Data
	choices := artChoices
	if !storageHas(choices, limit) {
		choices = append(append([]int(nil), artChoices...), limit)
		storageSort(choices)
	}
	items := make([]kit.SelectItem, len(choices))
	for i, n := range choices {
		items[i] = kit.SelectOption(strconv.Itoa(n), i18n.T("Up to {size}", "size", mibText(n)))
	}
	artUsed, artLimit := i18n.FormatBytes(u.ArtworkBytes), mibText(limit)
	share := math.Min(1, float64(u.ArtworkBytes)/float64(int64(limit)<<20))

	s.stack(c, func() {
		s.group(c, "storage-usage", i18n.T("On this computer"), "", func() {
			s.line(c)
			ui.Row(c.Key("figures")).FillWidth().Gap(24).Padding(rowPadY, 0, rowPadY, 0).AlignItems(ui.Start).Children(func() {
				s.figure(c, "art", i18n.T("Artwork cache"), artUsed, i18n.T("of {artworkLimit}", "artworkLimit", artLimit), func() {
					kit.ProgressBar(c, share, kit.ProgressOpts{
						A11yLabel: i18n.T("Artwork cache"), Width: 144,
						ValueText: i18n.T("{artworkUsed} of {artworkLimit}", "artworkUsed", artUsed, "artworkLimit", artLimit),
					}).Margin(6, 0, 0, 0)
				})
				s.figure(c, "catalog", i18n.T("Catalogue cache"), i18n.FormatBytes(u.CatalogBytes), "", func() {
					text(c, i18n.T("{catalogEntries, plural, one {# page} other {# pages}}", "catalogEntries", u.CatalogEntries), "meta").
						TextColor(theme.C("ink-muted")).FontFeatures("tnum")
				})
				s.figure(c, "db", i18n.T("Library database"), i18n.FormatBytes(u.DatabaseBytes), "", func() {
					if u.DatabaseFreeBytes > 0 {
						text(c, i18n.T("{databaseFree} of it free space", "databaseFree", i18n.FormatBytes(u.DatabaseFreeBytes)), "meta").
							TextColor(theme.C("ink-muted")).FontFeatures("tnum")
					}
				})
			})
		})
		s.group(c, "storage-keeping", i18n.T("Limit and cleanup"), "", func() {
			s.selectRow(c, rowOpts{
				ID: "art-cache", Title: i18n.T("Artwork cache"),
				Desc: i18n.T("Covers and backdrops kept on this computer so pages open without downloading them again. Above the limit, the artwork shown least recently is removed first."),
			}, strconv.Itoa(limit), items, false, func(v string) {
				n := atoiOr(v)
				s.save(func(x *config.Settings) { x.Storage.ArtCacheMiB = n })
			})
			s.clearRow(c, st)
			s.compactRow(c, st, u)
			s.settingsFile(c)
		})
	})
}

// figure is one figure of the usage line: what it is, its size (and of
// what), and a line under it.
func (s *settings) figure(c *ui.Context, id, label, value, of string, under func()) {
	k := c.Key("fig:" + id)
	ui.Column(k).Grow(1).Basis(0).MinWidth(0).Gap(4).Children(func() {
		text(k, label, "meta").TextColor(theme.C("ink-muted"))
		ui.Row(k.Key("v")).AlignItems(ui.End).Gap(4).Children(func() {
			text(k, value, "title").TextColor(theme.C("ink")).FontFeatures("tnum")
			if of != "" {
				text(k, of, "secondary").TextColor(theme.C("ink-muted")).FontFeatures("tnum")
			}
		})
		under()
	})
}

func (s *settings) clearRow(c *ui.Context, st *storageState) {
	s.row(c, rowOpts{
		ID: "clear-cache", Title: i18n.T("Catalogue cache"),
		Desc: i18n.T("Apple Music pages kept so they open at once and while offline; expired pages are removed every few hours. Clearing it and the artwork cache keeps your library and settings."),
	}, func() ui.Element {
		label := i18n.T("Clear caches")
		if st.clearing {
			label = i18n.T("Clearing…")
		}
		b := kit.Button(c.Key("clear"), label, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Disabled: st.clearing})
		if b.Clicked() {
			st.clearing = true
			svc := s.storageSvc()
			go func() {
				r, err := svc.Clear(context.Background())
				s.env.Post(func() {
					st.clearing = false
					s.env.Queries.Invalidate(keyStorageUsage)
					if err != nil {
						kit.ToastErrorf(i18n.T("Couldn't clear the cache"), i18n.ErrorMessage(err))
						return
					}
					kit.ShowToast(kit.ToastSpec{Title: i18n.T(
						"Cleared {artwork} of artwork and {pages, plural, one {# cached page} other {# cached pages}}",
						"artwork", i18n.FormatBytes(r.ArtworkBytes), "pages", r.CatalogEntries)})
				})
			}()
		}
		return b
	})
}

func (s *settings) compactRow(c *ui.Context, st *storageState, u services.StorageUsage) {
	desc := i18n.T("Your synced library, history and the catalogue cache. Sona optimises it when it is idle and nothing is playing.")
	if u.LastMaintenance > 0 {
		desc = i18n.T("Your synced library, history and the catalogue cache. Sona optimises it when it is idle and nothing is playing (last {lastMaintenance}).",
			"lastMaintenance", i18n.RelativeTime(stampTime(u.LastMaintenance), s.now()))
	}
	s.row(c, rowOpts{ID: "compact", Title: i18n.T("Library database"), Desc: desc}, func() ui.Element {
		label := i18n.T("Compact now")
		if st.compacting {
			label = i18n.T("Compacting…")
		}
		b := kit.Button(c.Key("compact"), label, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Disabled: st.compacting})
		if b.Clicked() {
			st.compacting = true
			svc := s.storageSvc()
			go func() {
				freed, err := svc.Compact(context.Background())
				s.env.Post(func() {
					st.compacting = false
					s.env.Queries.Invalidate(keyStorageUsage)
					switch {
					case err != nil:
						kit.ToastErrorf(i18n.T("Couldn't compact the database"), i18n.ErrorMessage(err))
					case freed > 0:
						kit.ShowToast(kit.ToastSpec{Title: i18n.T("Gave back {size} to the disk", "size", i18n.FormatBytes(freed))})
					default:
						kit.ShowToast(kit.ToastSpec{Title: i18n.T("The database was already compact")})
					}
				})
			}()
		}
		return b
	})
}

// mibText is a size in MiB as "512 MB" or "2 GB".
func mibText(n int) string {
	if n >= 1024 {
		return i18n.T("{gb} GB", "gb", float64(n)/1024)
	}
	return i18n.T("{n} MB", "n", n)
}

func storageErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func storageHas(l []int, n int) bool {
	for _, x := range l {
		if x == n {
			return true
		}
	}
	return false
}

func storageSort(l []int) {
	for i := 1; i < len(l); i++ {
		for j := i; j > 0 && l[j] < l[j-1]; j-- {
			l[j], l[j-1] = l[j-1], l[j]
		}
	}
}

func atoiOr(v string) int { n, _ := strconv.Atoi(v); return n }
