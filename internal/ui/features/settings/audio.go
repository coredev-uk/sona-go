package settings

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// systemDefault is the select value standing for "follow the system".
const systemDefault = "system-default"

var keyOutputs = store.Key{"audio", "outputs"}

// outputDevices lists the output devices (AudioService.OutputDevices); tests
// override it (seam:outputs).
func (s *settings) outputDevices() ([]services.OutputDevice, error) {
	if f := *stateOf[func() ([]services.OutputDevice, error)](s, "seam:outputs"); f != nil {
		return f()
	}
	return services.Service[services.AudioService](s.env.Core).OutputDevices()
}

// hasRoute reports whether a page exists; tests override it
// (seam:hasRoute). The Env hook (HasRoute) replaces this once it is merged.
func (s *settings) hasRoute(path string) bool {
	if f := *stateOf[func(string) bool](s, "seam:hasRoute"); f != nil {
		return f(path)
	}
	return s.env.HasRoute != nil && s.env.HasRoute(path)
}

// focusState remembers whether the output select had the focus, to refetch
// the list when it gets it.
type outputState struct {
	focused bool
	blurred bool // the window lost the keyboard focus: coming back refetches the devices
}

// outputRow is the Output select: system default, then each device; the
// list is refetched when the select is used and when the window
// gets the keyboard focus back (a device plugged in meanwhile).
func (s *settings) outputRow(c *ui.Context, st config.Settings) {
	title := i18n.T("Output")
	r := store.Use(s.env.Queries, keyOutputs, func(context.Context) ([]services.OutputDevice, error) {
		return s.outputDevices()
	})
	list := r.Data
	chosen := st.Audio.OutputDevice
	var fallback *services.OutputDevice
	missing := chosen != ""
	for i := range list {
		if list[i].Default && fallback == nil {
			fallback = &list[i]
		}
		if list[i].Name == chosen {
			missing = false
		}
	}
	desc := i18n.T("Where Sona plays. Switching moves the music that's playing without a pause.")
	if missing {
		desc = i18n.T("The output Sona was set to isn't connected; it plays to the system default until it's back.")
	}
	def := i18n.T("System default")
	if fallback != nil {
		def = i18n.T("System default · {fallbackName}", "fallbackName", fallback.Description)
	}
	items := []kit.SelectItem{kit.SelectOption(systemDefault, def)}
	for _, d := range list {
		items = append(items, kit.SelectOption(d.Name, d.Description))
	}
	if missing {
		items = append(items, kit.SelectItem{Value: chosen, Label: i18n.T("{chosen} · not connected", "chosen", chosen), Disabled: true})
	}
	value := chosen
	if value == "" {
		value = systemDefault
	}
	os := stateOf[outputState](s, "output")
	if win := s.env.Window; win != nil {
		if !win.Focused() {
			os.blurred = true
		} else if os.blurred {
			os.blurred = false
			s.env.Queries.Invalidate(keyOutputs)
		}
	}
	s.row(c, rowOpts{ID: "output", Title: title, Desc: desc}, func() ui.Element {
		v := value
		sel := kit.Select(c, &v, items, kit.SelectOpts{Small: true, A11yLabel: title, Width: selectWidthFor(items, value)})
		if f := sel.Focused(); f != os.focused || sel.Clicked() {
			os.focused = f
			if f {
				s.env.Queries.Invalidate(keyOutputs)
			}
		}
		if sel.Changed() && v != value {
			dev := v
			if dev == systemDefault {
				dev = ""
			}
			s.save(func(x *config.Settings) { x.Audio.OutputDevice = dev })
		}
		return sel
	})
}

// soundCheckRow is the Sound Check switch.
func (s *settings) soundCheckRow(c *ui.Context, st config.Settings, caps services.Capabilities) {
	title := i18n.T("Sound Check")
	if !caps.Playback.SoundCheck {
		s.unavailableRow(c, "sound-check", title, "")
		return
	}
	s.switchRow(c, rowOpts{
		ID:    "sound-check",
		Title: title,
		Desc:  i18n.T("Plays every song at about the same loudness, using the loudness Apple measured for it. Uploaded songs use their own Sound Check tag, if iTunes or Music wrote one."),
	}, st.Audio.SoundCheck, false, func(on bool) {
		s.save(func(x *config.Settings) { x.Audio.SoundCheck = on })
	})
}

// equalizerRow is the way to the equaliser: a link where its page exists,
// otherwise just the state.
func (s *settings) equalizerRow(c *ui.Context, st config.Settings, caps services.Capabilities) {
	title := i18n.T("Equaliser")
	if !caps.Playback.Equalizer {
		s.unavailableRow(c, "equalizer", title, "")
		return
	}
	desc := i18n.T("Ten bands, presets and a preamp, heard as you move them. Off now.")
	if st.Audio.Equalizer.Enabled {
		desc = i18n.T("Ten bands, presets and a preamp, heard as you move them. On now.")
	}
	s.row(c, rowOpts{ID: "equalizer", Title: title, Desc: desc}, func() ui.Element {
		if !s.hasRoute("/equalizer") {
			return ui.Element{}
		}
		b := kit.Button(c.Key("open"), i18n.T("Open equaliser"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM}).Role(ui.RoleLink)
		if b.Clicked() {
			s.env.Nav.Push("/equalizer")
		}
		return b
	})
}

// audioRows are Output, Sound Check and Equaliser, each where the
// platform's playback supports it.
func (s *settings) audioRows(c *ui.Context, st config.Settings, caps services.Capabilities) {
	if caps.Playback.OutputDevice {
		s.outputRow(c, st)
	} else {
		s.unavailableRow(c, "output", i18n.T("Output"), "")
	}
	s.soundCheckRow(c, st, caps)
	s.equalizerRow(c, st, caps)
}
