package media

// A test binary beside a package's GStreamer (build/windows/smoke.sh copies
// media.test.exe into the unpacked package) runs on the package's trimmed
// plugins, with the environment sona.exe sets. Anywhere else, the installed
// GStreamer is used and the import does nothing.
import _ "github.com/coredev-uk/sona-go/internal/playback/gstbundle"
