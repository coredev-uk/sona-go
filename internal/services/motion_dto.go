package services

import "github.com/wailsapp/wails/v3/pkg/application"

// EventPower carries PowerState when the machine switches between battery
// and mains power.
const EventPower = "power:changed"

func init() { application.RegisterEvent[PowerState](EventPower) }

// PowerState is the machine's power source (UPower OnBattery; false where
// UPower is unavailable).
type PowerState struct {
	OnBattery bool `json:"onBattery"`
}
