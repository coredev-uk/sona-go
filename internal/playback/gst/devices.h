// Audio output listing: PipeWire's GStreamer device provider on Linux
// (devices_linux.c), Core Audio on macOS (devices_darwin.c), Core Audio's
// endpoint enumerator on Windows (devices_windows.c, without GStreamer, with
// the endpoint watch in route_windows.c).
#ifndef SONA_DEVICES_H_
#define SONA_DEVICES_H_

#include <stdint.h>

typedef struct {
  char* name;         // PipeWire node.name (pipewiresink target-object); Core Audio device UID (osxaudiosink unique-id); WASAPI endpoint ID (wasapi2sink device)
  char* description;  // human-readable name
  int is_default;
} sona_device;

// sona_gst_audio_sinks lists the audio outputs. Linux: PipeWire's sinks;
// without PipeWire's GStreamer plugin it returns 0 with *n = 0. macOS: the
// devices that can play (output streams, alive, not hidden). Windows: the
// active render endpoints. -1 if listing fails. Free *out with
// sona_gst_free_devices.
int sona_gst_audio_sinks(sona_device** out, int* n);
void sona_gst_free_devices(sona_device* d, int n);

#ifdef __APPLE__
// sona_output_target returns the UID of the device to play to for the
// chosen uid (NULL or "": the system default): uid while that device can
// play, else the system default output's. malloc'd; NULL without any
// output. The first call starts the watch sona_route_epoch counts.
char* sona_output_target(const char* uid);
// sona_route_epoch counts changes of the default output device and of the
// device list (one plugged in or removed) since the watch started.
uint64_t sona_route_epoch(void);
#endif

#ifdef _WIN32
enum { SONA_ENDPOINT_UNKNOWN = -1, SONA_ENDPOINT_GONE = 0, SONA_ENDPOINT_ACTIVE = 1 };
// sona_endpoint_state says whether the WASAPI endpoint id can play now
// (DEVICE_STATE_ACTIVE): SONA_ENDPOINT_ACTIVE, SONA_ENDPOINT_GONE (unknown
// to Windows, unplugged, disabled), or SONA_ENDPOINT_UNKNOWN when the
// endpoint API itself fails. The first call starts the watch
// sona_route_epoch counts.
int sona_endpoint_state(const char* id);
// sona_route_epoch counts endpoints added, removed or changing state
// (plugged in, unplugged, enabled, disabled) since the watch started.
uint64_t sona_route_epoch(void);
#endif

#endif  // SONA_DEVICES_H_
