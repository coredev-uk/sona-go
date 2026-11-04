package concerts

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

const (
	bandTickets   = "Get tickets for Example Band with Opener Duo from Example Tickets, opens in your browser"
	singerTickets = "Get tickets for Sample Singer"
)

func (r *rig) opened(url string) func() bool {
	return func() bool { return slices.Contains(r.openedURLs(), url) }
}

func TestOneSellerOpensInTheBrowser(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 800, exampleBand, sampleSinger, anotherAct)
	r.shows(bandTickets)
	r.click(bandTickets)
	r.tt.Until("the seller's page opened", r.opened("https://tickets.example.com/e/1"))
}

func TestSeveralSellersAreAMenu(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 800, exampleBand, sampleSinger, anotherAct)
	r.shows(singerTickets)
	r.click(singerTickets)
	r.shows("First Seller", "Second Seller")
	r.click("Second Seller")
	r.tt.Until("the chosen seller's page opened", r.opened("https://second.example.com/2"))
	if slices.Contains(r.openedURLs(), "https://first.example.com/1") {
		t.Error("the other seller was opened too")
	}
	r.tt.Until("the menu to close", func() bool { return !r.tt.HasText("Second Seller") })
}

func TestSeveralSellersMenuOpensFromTheKeyboard(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 800, exampleBand, sampleSinger, anotherAct)
	r.shows(singerTickets)
	r.click(bandTickets)    // focus lands on the first row's tickets
	r.tt.Key(0, ui.KeyDown) // ↓ moves down the agenda...
	r.tt.Drain()
	r.tt.Drain()
	if !r.tt.Focused(singerTickets) {
		t.Fatalf("↓ did not reach the menu button; texts %q", r.tt.Texts())
	}
	if r.tt.HasText("Second Seller") {
		t.Error("↓ opened the menu")
	}
	r.tt.Key(0, ui.KeyEnter) // ...and Enter opens it
	r.tt.Drain()
	r.shows("First Seller", "Second Seller")
}

func TestNoSellerNoTicketControl(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 800, anotherAct)
	r.shows("Another Act")
	r.lacks("Get tickets")
	if len(r.openedURLs()) != 0 {
		t.Errorf("opened %q", r.openedURLs())
	}
}

func TestAgendaIsOneTabStopWithArrowsAlongAndDown(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 800, exampleBand, sampleSinger, anotherAct)
	r.shows(bandTickets)
	press := func(mods ui.Modifiers, k ui.Key) {
		r.tt.Key(mods, k)
		r.tt.Drain()
		r.tt.Drain()
	}
	at := func(what, label string) {
		t.Helper()
		if !r.tt.Focused(label) {
			t.Errorf("%s: focus is not on %q; texts %q", what, label, r.tt.Texts())
		}
	}
	r.click(bandTickets) // row 0, tickets
	at("clicked", bandTickets)
	press(0, ui.KeyLeft)
	at("left to the line-up", "Example Band with Opener Duo")
	press(0, ui.KeyLeft)
	at("left at the row's start", "Example Band with Opener Duo")
	press(0, ui.KeyDown)
	at("down keeps the column", "Sample Singer")
	press(0, ui.KeyRight)
	at("right to the tickets", singerTickets)
	press(0, ui.KeyDown)
	at("down to a row without tickets", "Another Act") // the column is clamped
	press(0, ui.KeyDown)
	at("down at the last row", "Another Act")
	press(0, ui.KeyUp)
	at("up", "Sample Singer")
	press(0, ui.KeyHome)
	at("home", "Example Band with Opener Duo")
	press(0, ui.KeyRight)
	press(0, ui.KeyEnd)
	at("end keeps the column, clamped", "Another Act")
	// A key with a modifier is not the list's.
	press(ui.Shift, ui.KeyUp)
	at("shift+up", "Another Act")

	// Tab leaves the list; coming back lands on the control focused last.
	press(0, ui.KeyUp)
	press(0, ui.KeyRight)
	at("before leaving", singerTickets)
	press(0, ui.KeyTab)
	for _, l := range []string{bandTickets, singerTickets, "Example Band with Opener Duo", "Sample Singer", "Another Act"} {
		if r.tt.Focused(l) {
			t.Errorf("Tab stayed in the agenda, on %q", l)
		}
	}
	press(ui.Shift, ui.KeyTab)
	at("back in", singerTickets)
}

func TestAuthChangeRefetchesOnlyWhenTheStateChanges(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 800, exampleBand, sampleSinger, anotherAct)
	r.shows("Your artists (2)")
	counted := &countingAPI{concertsAPI: r.s.api}
	r.s.api = counted
	asked := func() int { return counted.asked() }
	n := asked()
	emit := func(state string) {
		st := services.AuthStatus{State: state}
		store.Set(r.h.Env.Queries, store.KeyAuth, st) // the auth module keeps the cache
		r.h.Bus.Emit(services.EventAuthChanged, st)
		r.tt.Drain()
	}
	emit("signed_in") // the first event is not news
	emit("validating")
	r.tt.Drain()
	if got := asked(); got != n {
		t.Errorf("%d listings asked after events that changed nothing, %d before", got, n)
	}
	emit("signed_out") // now it is
	r.tt.Until("a new listing", func() bool { return asked() > n })
	// Signed out, the user's own artists are not offered.
	r.tt.Until("no Your artists switch", func() bool { return !r.tt.HasText("Your artists (2)") })
}

// countingAPI counts the listings asked of the service.
type countingAPI struct {
	concertsAPI
	n atomic.Int32
}

func (c *countingAPI) Concerts(ctx context.Context, req services.ConcertsRequest) (services.ConcertsPage, error) {
	c.n.Add(1)
	return c.concertsAPI.Concerts(ctx, req)
}

func (c *countingAPI) asked() int { return int(c.n.Load()) }
