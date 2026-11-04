package player

import (
	"fmt"

	"example.com/tree/internal/i18n/l10n"
)

// Messages, grouped.
const (
	// msgPlay is the play button.
	msgPlay     l10n.Message = "Play"
	msgPause    l10n.Message = "Pause" // the pause button
	notAMessage              = "Untyped strings are not messages"
	msgCount    l10n.Message = "%d songs"
	msgRaw      l10n.Message = `A raw "quoted" string`
	msgJoined   l10n.Message = "Joined " + "from " + "parts"
)

// MsgPublic is used from another package.
var MsgPublic l10n.Message = "Shared"

var labels = map[string]l10n.Message{
	"next": "Next",
	"prev": "Previous",
	"play": msgPlay,
}

var order = []l10n.Message{msgPause, "Stop", 3: "Shuffle"}

func show(p interface{ T(l10n.Message) string }) string {
	fmt.Println(notAMessage)
	return p.T(l10n.Message("Converted")) + p.T(l10n.Message(msgPlay))
}
