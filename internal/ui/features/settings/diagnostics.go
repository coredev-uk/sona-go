package settings

import (
	"context"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Query keys: the checks (staleTime 0: they run on every open) and the
// motion artwork status, refetched every 5 s while the page is open.
var (
	keyDiagnostics = store.Key{"diagnostics"}
	keyMotion      = store.Key{"diagnostics", "motion"}
)

const motionRefetch = 5 * time.Second

// What the section asks of the services. Tests replace them (diagSeams).
type (
	diagAPI interface {
		Diagnostics(ctx context.Context) (services.Diagnostics, error)
		DiagnosticsBundle(ctx context.Context) (services.DiagBundle, error)
		OpenLogFolder() error
	}
	motionAPI interface {
		Status() services.MotionStatus
		Report(services.MotionReport)
	}
	inspectorAPI interface{ OpenDeveloperTools() error }
)

// diagSeamState is the section's replaceable collaborators; a nil field is the
// real one.
type diagSeamState struct {
	sys     diagAPI
	mot     motionAPI
	desk    inspectorAPI
	native  func() (services.MotionReport, bool)
	nextRun time.Time // when the motion status is refetched next
}

func (s *settings) diagSeams() *diagSeamsView {
	return &diagSeamsView{stateOf[diagSeamState](s, "seam:diagnostics")}
}

// diagSeamsView resolves the diagSeamState to the real services where unset.
type diagSeamsView struct{ *diagSeamState }

func (v *diagSeamsView) system(s *settings) diagAPI {
	if v.sys != nil {
		return v.sys
	}
	return services.Service[services.SystemService](s.env.Core)
}

func (v *diagSeamsView) motion(s *settings) motionAPI {
	if v.mot != nil {
		return v.mot
	}
	return services.Service[services.MotionService](s.env.Core)
}

func (v *diagSeamsView) desktop(s *settings) inspectorAPI {
	if v.desk != nil {
		return v.desk
	}
	return services.Service[services.DesktopService](s.env.Core)
}

// nativeMotion is the native motion policy's report (env.Motion); false
// without one: motion artwork was not probed.
func (v *diagSeamsView) nativeMotion(s *settings) (services.MotionReport, bool) {
	if v.native != nil {
		return v.native()
	}
	// No motion group until the codec probe has answered: Report's blank
	// codec fields would read as "nothing decodes".
	if s.env.Motion == nil || !s.env.Motion.Probed() {
		return services.MotionReport{}, false
	}
	return s.env.Motion.Report(), true
}

// diagnostics is Settings → Diagnostics (Diagnostics.tsx DiagnosticsSection).
func (s *settings) diagnostics(c *ui.Context) {
	v := s.diagSeams()
	api := v.system(s)
	q := store.Use(s.env.Queries, keyDiagnostics, func(ctx context.Context) (services.Diagnostics, error) {
		return api.Diagnostics(ctx)
	})
	report, probed := v.nativeMotion(s)
	var status *services.MotionStatus
	if probed {
		mot := v.motion(s)
		m := store.Use(s.env.Queries, keyMotion, func(context.Context) (services.MotionStatus, error) {
			return mot.Status(), nil
		})
		if m.HasData {
			status = &m.Data
		}
		// Downloads happen while the page is open (another window's covers).
		now := s.now()
		if v.nextRun.IsZero() || now.Before(v.nextRun.Add(-motionRefetch)) {
			v.nextRun = now.Add(motionRefetch)
		}
		if !now.Before(v.nextRun) {
			s.env.Queries.Invalidate(keyMotion)
			v.nextRun = now.Add(motionRefetch)
		}
		c.After(v.nextRun.Sub(now))
	}
	var live *services.CDMStatus
	if r := s.cdmStatus(); r.HasData {
		live = &r.Data
	}
	var groups []diagGroup
	if q.HasData {
		groups = diagnosticsGroups(q.Data, live, s.now())
		if probed {
			groups = append(groups[:1], append([]diagGroup{motionGroup(report, status, s.now())}, groups[1:]...)...)
		}
	}
	problems, warnings := tally(groups)
	caps := s.caps()

	ui.Column(c.Key("diag")).FillWidth().Children(func() {
		s.diagHeader(c, q, problems, warnings)
		if q.Err != nil {
			kit.ErrorBlock(c.Key("err"), i18n.T("Couldn't run the checks"), q.Err, kit.ErrorBlockOpts{
				OnRetry: func() { s.env.Queries.Invalidate(keyDiagnostics) }, Retrying: q.Fetching,
			})
		}
		if !q.HasData && q.Err == nil {
			s.diagLoading(c)
		}
		ui.Column(c.Key("groups")).FillWidth().Gap(40).Children(func() {
			for _, g := range groups {
				s.diagGroup(c, g)
			}
			if caps.HasData {
				s.developerTools(c, caps.Data.DeveloperTools)
			}
		})
	})
}

// diagVerdict is the header's sentence, as it is read out.
func diagVerdict(q store.Result[services.Diagnostics], problems, warnings int) string {
	var out string
	switch {
	case q.Fetching:
		return i18n.T("Checking the playback host, database and keyring…")
	case q.HasData && problems > 0:
		out = i18n.T("{problems, plural, one {# problem found} other {# problems found}}", "problems", problems)
	case q.HasData:
		out = i18n.T("Everything Sona needs is working")
	default:
		return ""
	}
	if warnings > 0 {
		out += ". " + i18n.T("{warnings, plural, one {# warning to look at below} other {# warnings to look at below}}", "warnings", warnings)
	}
	return out
}

// diagHeader is the verdict (announced politely) and the three buttons.
func (s *settings) diagHeader(c *ui.Context, q store.Result[services.Diagnostics], problems, warnings int) {
	c = c.Key("head")
	s.tell(c, "diag-verdict", diagVerdict(q, problems, warnings)) // aria-live="polite"
	ui.Box(c.Key("line")).FillWidth().Height(1).Shrink(0).Background(theme.C("line"))
	ui.Row(c).FillWidth().Wrap().GapX(24).GapY(12).Padding(16, 0, 0, 0).Margin(0, 0, 32, 0).Children(func() {
		ui.Row(c.Key("verdict")).Grow(1).MinHeight(32).AlignItems(ui.Start).Gap(10).Role(ui.RoleStatus).Children(func() {
			if q.HasData && !q.Fetching {
				st := statusOK
				if problems > 0 {
					st = statusError
				}
				statusIcon(c.Key("icon"), st).Margin(2, 0, 0, 0)
			}
			ui.Column(c.Key("text")).Children(func() {
				switch {
				case q.Fetching:
					text(c, i18n.T("Checking the playback host, database and keyring…"), "heading").TextColor(theme.C("ink"))
				case q.HasData && problems > 0:
					text(c, i18n.T("{problems, plural, one {# problem found} other {# problems found}}", "problems", problems), "heading").TextColor(theme.C("ink"))
				case q.HasData:
					text(c, i18n.T("Everything Sona needs is working"), "heading").TextColor(theme.C("ink"))
				}
				if q.HasData && !q.Fetching && warnings > 0 {
					text(c, i18n.T("{warnings, plural, one {# warning to look at below} other {# warnings to look at below}}", "warnings", warnings), "secondary").TextColor(theme.C("ink-muted"))
				}
			})
		})
		ui.Row(c.Key("buttons")).Wrap().Gap(8).Children(func() {
			if kit.Button(c.Key("again"), i18n.T("Check again"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "rotate-cw", Disabled: q.Fetching}).Clicked() {
				s.env.Queries.Invalidate(keyDiagnostics)
			}
			pending := stateOf[bundleState](s, "diag:bundle").pending
			label := i18n.T("Copy diagnostics")
			if pending {
				label = i18n.T("Collecting…")
			}
			copyBtn := kit.Button(c.Key("copy"), label, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "copy", Disabled: pending})
			s.seen["diagnostics-copy"] = true
			if copyBtn.Clicked() {
				s.copyBundle()
			}
			logs := kit.Button(c.Key("logs"), i18n.T("Open log folder"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "folder-open"})
			s.seen["diagnostics-logs"] = true
			if logs.Clicked() {
				s.openLogFolder()
			}
			s.land(c, "diagnostics-copy", copyBtn, copyBtn)
			s.land(c, "diagnostics-logs", logs, logs)
		})
	})
}

// statusIcon is a row's verdict icon: OK, Warning or Problem for assistive
// technology; nothing visible for none (a 16 DIP gap keeps the values aligned).
func statusIcon(c *ui.Context, st diagStatus) ui.Element {
	var name, label string
	var color ui.Color
	switch st {
	case statusOK:
		name, label, color = "circle-check", i18n.T("OK"), theme.C("positive")
	case statusWarning:
		name, label, color = "triangle-alert", i18n.T("Warning"), theme.C("ink")
	case statusError:
		name, label, color = "circle-alert", i18n.T("Problem"), theme.C("danger")
	default:
		return ui.Box(c).Size(16, 16).Shrink(0)
	}
	return ui.Icon(c, theme.Icon(name)).FontSize(16).TextColor(color).Shrink(0).Role(ui.RoleImage).Label(label)
}

// diagGroup is an h3 and its label/value rows.
func (s *settings) diagGroup(c *ui.Context, g diagGroup) {
	c = c.Key("g:" + g.id)
	ui.Column(c).FillWidth().Role(ui.RoleGroup).Label(g.title).Children(func() {
		text(c, g.title, "heading").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(3).Margin(0, 0, 12, 0)
		for i, r := range g.rows {
			k := c.Key("row" + strconv.Itoa(i))
			ui.Box(c.Key("line" + strconv.Itoa(i))).FillWidth().Height(1).Shrink(0).Background(theme.C("line"))
			ui.Row(k).FillWidth().Gap(24).Padding(10, 0).AlignItems(ui.Start).Children(func() {
				text(k, r.label, "secondary").TextColor(theme.C("ink-muted")).Width(200).Shrink(0)
				ui.Column(k.Key("v")).Grow(1).Shrink(1).MinWidth(0).Children(func() {
					ui.Row(k.Key("val")).Gap(8).AlignItems(ui.Start).Children(func() {
						statusIcon(k.Key("s"), r.status)
						t := text(k, r.value, "secondary").TextColor(theme.C("ink")).Shrink(1).MinWidth(0)
						if r.status == statusError {
							t.FontWeight(500)
						}
					})
					if r.detail != "" {
						text(k, r.detail, "secondary").TextColor(theme.C("ink-muted")).Margin(4, 0, 0, 24).MaxWidth(576)
					}
				})
			})
		}
	})
}

// diagLoading is the 7-row label/value skeleton, announced busy.
func (s *settings) diagLoading(c *ui.Context) {
	ui.Column(c.Key("loading")).FillWidth().Role(ui.RoleStatus).Label(i18n.T("Loading")).Children(func() {
		ui.Box(c.Key("h")).Width(112).Height(16).Radius(theme.Current().Radius["sm"]).Background(theme.C("hover")).Margin(0, 0, 12, 0)
		for i := range 7 {
			k := c.Key("row" + strconv.Itoa(i))
			ui.Box(c.Key("line" + strconv.Itoa(i))).FillWidth().Height(1).Shrink(0).Background(theme.C("line"))
			ui.Row(k).FillWidth().Height(40).AlignItems(ui.Center).Gap(24).Children(func() {
				ui.Box(k.Key("l")).Width(128).Height(12).Radius(theme.Current().Radius["sm"]).Background(theme.C("hover"))
				ui.Box(k.Key("v")).Width(float32(160 + (i*13)%35*6)).Height(12).Radius(theme.Current().Radius["sm"]).Background(theme.C("hover"))
			})
		}
	})
}
