package main

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

var (
	variantNames = []string{"Default", "Ghost", "Outline", "Play", "Destructive"}
	sizeNames    = []string{"MD", "SM", "XS"}
	iconNames    = []string{"", "play", "pause", "plus", "ellipsis", "shuffle", "search", "check", "x", "disc-3"}
)

// buttonKnobs is the Button playground's state.
var buttonKnobs = struct {
	label    string
	variant  int
	size     int
	icon     string
	toggle   bool
	pressed  bool
	disabled bool
	clicks   int
}{label: "Play", icon: "play"}

func init() {
	register(section{name: "Button", build: buttons})
}

func buttons(c *ui.Context, _ *gallery) {
	k := &buttonKnobs
	note(c, "kit.Button: four variants and the destructive outline, three sizes; with no label it is a square showing its icon.")

	heading(c, "Playground")
	panel(c, func() {
		knob(c, "Label", func() { ui.TextInput(c, &k.label).Label("Button label").Width(200) })
		knob(c, "Variant", func() { ui.Segmented(c, &k.variant, variantNames...).Label("Variant") })
		knob(c, "Size", func() { ui.Segmented(c, &k.size, sizeNames...).Label("Size") })
		knob(c, "Icon", func() {
			sel := k.icon
			if sel == "" {
				sel = "none"
			}
			options := slices.Clone(iconNames)
			options[0] = "none"
			if ui.Select(c, &sel, options).Label("Icon").Width(160).Changed() {
				k.icon = sel
				if sel == "none" {
					k.icon = ""
				}
			}
		})
		knob(c, "State", func() {
			ui.Checkbox(c, &k.toggle, "Toggle")
			ui.Checkbox(c, &k.pressed, "Pressed").Disabled(!k.toggle)
			ui.Checkbox(c, &k.disabled, "Disabled")
		})
		ui.Row(c).Gap(16).AlignItems(ui.Center).Padding(8, 0, 0).Children(func() {
			o := kit.ButtonOpts{Variant: kit.ButtonVariant(k.variant), Size: kit.ButtonSize(k.size), Icon: k.icon, Disabled: k.disabled, A11yLabel: "Playground button"}
			if k.toggle {
				o.Pressed = &k.pressed
			}
			if kit.Button(c.Key("playground"), k.label, o).Clicked() {
				k.clicks++
				if k.toggle {
					k.pressed = !k.pressed
				}
			}
			text(c, fmt.Sprintf("%d clicks", k.clicks), "meta").TextColor(theme.C("ink-faint"))
		})
	})

	heading(c, "Variants, sizes and states")
	note(c, "Hover and press a button to see its other faces; Tab to see the focus ring.")
	on := true
	for vi, vn := range variantNames {
		v := kit.ButtonVariant(vi)
		labelled(c, vn, func() {
			for si, sn := range sizeNames {
				s := kit.ButtonSize(si)
				key := func(state string) *ui.Context { return c.Key(fmt.Sprintf("%s/%s/%s", vn, sn, state)) }
				ui.Row(c).Gap(12).AlignItems(ui.Center).Wrap().Children(func() {
					text(c, sn, "meta").TextColor(theme.C("ink-faint")).Width(24)
					kit.Button(key("label"), "Label", kit.ButtonOpts{Variant: v, Size: s})
					kit.Button(key("icon-label"), "Label", kit.ButtonOpts{Variant: v, Size: s, Icon: "plus"})
					kit.Button(key("icon"), "", kit.ButtonOpts{Variant: v, Size: s, Icon: "play", A11yLabel: "Play"})
					kit.Button(key("pressed"), "Pressed", kit.ButtonOpts{Variant: v, Size: s, Pressed: &on})
					kit.Button(key("disabled"), "Disabled", kit.ButtonOpts{Variant: v, Size: s, Disabled: true})
					kit.Button(key("icon-disabled"), "", kit.ButtonOpts{Variant: v, Size: s, Icon: "play", Disabled: true, A11yLabel: "Play"})
				})
			}
		})
	}
}
