package discord

// Supported is false: Discord on Windows listens on a named pipe
// (\\.\pipe\discord-ipc-N), which the client does not dial yet, so
// every SetActivity fails with ErrUnsupported.
const Supported = false

// socketPaths lists no candidates: see Supported.
func socketPaths() []string { return nil }
