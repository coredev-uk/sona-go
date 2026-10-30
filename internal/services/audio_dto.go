package services

// EqualizerInfo describes the equaliser: its bands, limits and presets.
type EqualizerInfo struct {
	// Frequencies are the band centres in Hz, low to high.
	Frequencies []float64 `json:"frequencies"`
	// MinGain and MaxGain bound a band's gain, in dB.
	MinGain float64 `json:"minGain"`
	MaxGain float64 `json:"maxGain"`
	// MinPreamp and MaxPreamp bound the preamp trim, in dB.
	MinPreamp float64           `json:"minPreamp"`
	MaxPreamp float64           `json:"maxPreamp"`
	Presets   []EqualizerPreset `json:"presets"`
}

// EqualizerPreset is a built-in curve.
type EqualizerPreset struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Gains []float64 `json:"gains"` // dB per band
}

// OutputDevice is an audio output (a PipeWire sink).
type OutputDevice struct {
	// Name is the PipeWire node.name, the value of the outputDevice
	// setting.
	Name        string `json:"name"`
	Description string `json:"description"`
	// Default marks the system's default output.
	Default bool `json:"default"`
}
