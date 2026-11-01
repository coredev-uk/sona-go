package config

// Onboarding is the first-run setup (frontend features/onboarding): the
// main window opens on it until it is finished or skipped, unless an Apple
// Music account is already on this computer (services.StartRoute). Every
// value is valid, so there is nothing to normalise.
type Onboarding struct {
	// Completed: the setup was finished or skipped. Settings → General
	// opens it again without changing this.
	Completed bool `json:"completed"`
}
