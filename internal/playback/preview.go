package playback

import (
	"fmt"
	"strconv"
)

// EnvPreview is the switch for preview playback: when on, the engine plays
// every song as Apple's 30-second preview (Status.Preview), which needs
// only the developer token: no Music User Token, no Widevine CDM and no
// licence. Tests, CI and smoke runs that need audio use it; the full DRM
// path is checked deliberately with a Music User Token (cmd/playspike
// without -preview, the live workflow).
const EnvPreview = "SONA_PREVIEW"

// PreviewEnv reads a value of EnvPreview: "" is off, otherwise a boolean
// as strconv.ParseBool reads it ("1", "true", "0", …). A value it can't
// read is an error, and off.
func PreviewEnv(v string) (bool, error) {
	if v == "" {
		return false, nil
	}
	on, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("playback: %s=%q is not a boolean; previews are off", EnvPreview, v)
	}
	return on, nil
}
