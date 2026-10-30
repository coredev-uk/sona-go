// Audio output listing through PipeWire's GStreamer device provider.
#ifndef SONA_DEVICES_H_
#define SONA_DEVICES_H_

typedef struct {
  char* name;         // PipeWire node.name (pipewiresink target-object)
  char* description;  // human-readable name
  int is_default;
} sona_device;

// sona_gst_audio_sinks lists PipeWire audio sinks. Without PipeWire's
// GStreamer plugin it returns 0 with *n = 0; -1 if the provider fails.
// Free *out with sona_gst_free_devices.
int sona_gst_audio_sinks(sona_device** out, int* n);
void sona_gst_free_devices(sona_device* d, int n);

#endif  // SONA_DEVICES_H_
