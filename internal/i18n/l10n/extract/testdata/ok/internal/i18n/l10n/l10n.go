package l10n

// Message is a catalogue message.
type Message string

// English is a message inside the package itself.
const msgInside Message = "Inside the package"

// fromCatalogue converts text the runtime read: not a message.
func fromCatalogue(id string) Message { return Message(id) }
