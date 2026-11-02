package tray

import (
	msg "example.com/tree/internal/i18n/l10n"
	"example.com/tree/player"
)

var items = []msg.Message{player.MsgPublic, "Play", msgQuit}

// msgQuit is the tray's last item.
const msgQuit msg.Message = "Quit" // keep it short
