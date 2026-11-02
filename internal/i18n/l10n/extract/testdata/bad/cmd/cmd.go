package cmd

import (
	"fmt"

	"example.com/bad/internal/i18n/l10n"
)

const untyped = "untyped"

const msgOK l10n.Message = "Fine"

var msgCall l10n.Message = l10n.Message(fmt.Sprint("built"))

const msgJoined l10n.Message = "Joined " + untyped

var list = []l10n.Message{untyped, msgOK}

var converted = l10n.Message(fmt.Sprint(1))
