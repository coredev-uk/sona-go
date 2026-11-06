// Audio output listing: PipeWire's GStreamer device provider on Linux
// (devices_linux.c), Core Audio on macOS (devices_darwin.c).
#ifndef SONA_DEVICES_H_
#define SONA_DEVICES_H_

#include <stdint.h>

typedef struct {
  char* name;         // PipeWire node.name (pipewiresink target-object); Core Audio device UID (osxaudiosink unique-id)
  char* description;  // human-readable name
  int is_default;
} sona_device;

// sona_gst_audio_sinks lists the audio outputs. Linux: PipeWire's sinks;
// without PipeWire's GStreamer plugin it returns 0 with *n = 0. macOS: the
// devices that can play (output streams, alive, not hidden). -1 if listing
// fails. Free *out with sona_gst_free_devices.
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

#endif  // SONA_DEVICES_H_
