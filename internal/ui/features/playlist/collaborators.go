package playlist

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// stackFaces is how many faces the header's stack shows before the count
// says the rest.
const stackFaces = 4

// collaborators is the state of the byline's dialog.
type collaborators struct {
	open bool
}

// personName is what a profile is called: its name, else @handle.
func personName(pr services.SocialProfile) string {
	if pr.Name != "" {
		return pr.Name
	}
	return "@" + pr.Handle
}

// collaboratorsLabel is the byline's words: "Owner & n others" ("You & n
// others" for the owner, "You" alone).
func collaboratorsLabel(c services.PlaylistCollaboration) string {
	others := max(0, len(c.Collaborators)-1)
	lead := ""
	for _, pr := range c.Collaborators {
		if pr.ID == c.OwnerID {
			lead = pr.Name
			if lead == "" {
				lead = pr.Handle
			}
			break
		}
	}
	if lead == "" && len(c.Collaborators) > 0 {
		lead = c.Collaborators[0].Name
		if lead == "" {
			lead = c.Collaborators[0].Handle
		}
	}
	switch {
	case c.Viewer == services.RoleOwner && others > 0:
		return i18n.T("{others, plural, one {You & # other} other {You & # others}}", "others", others)
	case c.Viewer == services.RoleOwner:
		return i18n.T("You")
	case others > 0:
		return i18n.T("{others, plural, one {{lead} & # other} other {{lead} & # others}}", "others", others, "lead", lead)
	}
	return lead
}

// roleLine is what the signed-in user is to the playlist ("" for a
// listener).
func roleLine(v services.CollaborationRole) string {
	switch v {
	case services.RoleOwner:
		return i18n.T("You started this playlist.")
	case services.RoleCollaborator:
		return i18n.T("You're a collaborator.")
	}
	return ""
}

// collaboratorsByline is a collaborative playlist's byline (Collaborators.tsx):
// the collaborators' faces and "Owner & n others", a button that opens the
// full list.
func (p *playlists) collaboratorsByline(c *ui.Context, collab services.PlaylistCollaboration) {
	st := &p.page.collab
	tk := theme.Current()
	label := collaboratorsLabel(collab)
	btn := ui.ButtonBase(c.Key("collaborators")).Row().AlignItems(ui.Center).Gap(10).Margin(0, 0, 0, -4).Padding(2, 4).
		Radius(tk.Radius["md"]).MinWidth(0).Tooltip(i18n.T("View playlist collaborators")).Description(i18n.T("View playlist collaborators"))
	hovered := btn.Hovered()
	btn.Children(func() {
		faces := ui.Row(c.Key("faces")).Shrink(0).Role(ui.RoleNone)
		faces.Children(func() {
			for i, pr := range collab.Collaborators[:min(stackFaces, len(collab.Collaborators))] {
				ring := ui.Box(c.Key(pr.ID)).Size(28, 28).Radius(14).Shrink(0).Background(theme.C("ground")).Padding(2)
				if i > 0 {
					ring.MarginStart(-6)
				}
				ring.Children(func() {
					kit.PersonAvatar(c, p.env.Art, personName(pr), pr.Art.Small, 24)
				})
			}
		})
		t := ui.Text(c, label).FontSize(tk.Type["heading"].Size).FontWeight(500).TextColor(theme.C("ink")).SingleLine().Shrink(1).MinWidth(0)
		if hovered {
			t.Underline()
		}
	})
	if btn.Clicked() {
		st.open = true
	}
	p.collaboratorsDialog(c, collab)
}

// collaboratorsDialog lists everyone who works on the playlist.
func (p *playlists) collaboratorsDialog(c *ui.Context, collab services.PlaylistCollaboration) {
	st := &p.page.collab
	desc := i18n.T("{0, plural, one {# person can add songs to this playlist.} other {# people can add songs to this playlist.}}", "0", len(collab.Collaborators))
	if role := roleLine(collab.Viewer); role != "" {
		desc += " " + role
	}
	kit.Dialog(c, &st.open, kit.DialogOpts{
		Title: i18n.T("Collaborators"), Description: desc, MaxWidth: 420, Padding: 20, Gap: 20,
	}, func() {
		p.peopleList(c, "people", collab.Collaborators, collab.OwnerID)
		if len(collab.Former) > 0 {
			ui.Column(c.Key("former")).Gap(8).Role(ui.RoleGroup).Label(i18n.T("Former collaborators")).Children(func() {
				tk := theme.Current()
				ui.Text(c, i18n.T("Former collaborators")).FontSize(tk.Type["sub"].Size).FixedLineHeight(tk.Type["sub"].LineHeight).
					FontWeight(600).TextColor(theme.C("ink-muted")).SingleLine().Role(ui.RoleHeading).Level(3)
				p.peopleList(c, "former-people", collab.Former, "")
			})
		}
	})
}

// peopleList is one scrollable list of people: round picture, name or
// @handle with the @handle under it, an "Owner" tag on the owner, each a link
// to the person's profile where Sona has the page.
func (p *playlists) peopleList(c *ui.Context, key string, people []services.SocialProfile, ownerID string) {
	tk := theme.Current()
	_, wh := c.Size()
	linked := p.hasRoute("/profile/x")
	ui.Scroll(c.Key(key)).MaxHeight(min(wh*0.6, 420)).Margin(0, -8).Children(func() {
		ui.Column(c).Children(func() {
			for _, pr := range people {
				var row ui.Element
				if linked {
					row = ui.ButtonBase(c.Key(pr.ID)).Role(ui.RoleLink).Label(personName(pr)).Cursor(ui.CursorPointer)
				} else {
					row = ui.Row(c.Key(pr.ID)).Role(ui.RoleListItem)
				}
				row.Row().AlignItems(ui.Center).Gap(12).Padding(6, 8).Radius(tk.Radius["md"]).FillWidth()
				if linked && row.Hovered() {
					row.Background(theme.C("hover"))
				}
				row.Children(func() {
					kit.PersonAvatar(c, p.env.Art, personName(pr), pr.Art.Small, 40)
					ui.Column(c.Key("names")).Grow(1).Basis(0).MinWidth(0).Children(func() {
						ui.Text(c, personName(pr)).FontSize(tk.Type["ui"].Size).FixedLineHeight(tk.Type["ui"].LineHeight).
							TextColor(theme.C("ink")).SingleLine()
						if pr.Name != "" && pr.Handle != "" {
							ui.Text(c, "@"+pr.Handle).FontSize(tk.Type["sub"].Size).FixedLineHeight(tk.Type["sub"].LineHeight).
								TextColor(theme.C("ink-muted")).SingleLine()
						}
					})
					if pr.ID == ownerID {
						ui.Text(c, i18n.T("Owner")).FontSize(tk.Type["meta"].Size).FixedLineHeight(tk.Type["meta"].LineHeight).
							TextColor(theme.C("ink-faint")).SingleLine().Shrink(0)
					}
				})
				if linked && row.Clicked() {
					p.page.collab.open = false
					p.env.Nav.Push("/profile/" + pr.ID)
				}
			}
		})
	})
}
