#include "devices.h"

#include <CoreAudio/CoreAudio.h>
#include <CoreFoundation/CoreFoundation.h>
#include <pthread.h>
#include <stdatomic.h>
#include <stdlib.h>
#include <string.h>

static AudioObjectPropertyAddress global(AudioObjectPropertySelector sel) {
  return (AudioObjectPropertyAddress){sel, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
}

static UInt32 u32_prop(AudioObjectID obj, AudioObjectPropertySelector sel, UInt32 fallback) {
  AudioObjectPropertyAddress a = global(sel);
  UInt32 v = 0, size = sizeof(v);
  return AudioObjectGetPropertyData(obj, &a, 0, NULL, &size, &v) == noErr ? v : fallback;
}

// str_prop returns a CFString property as malloc'd UTF-8, or NULL.
static char* str_prop(AudioObjectID obj, AudioObjectPropertySelector sel) {
  AudioObjectPropertyAddress a = global(sel);
  CFStringRef s = NULL;
  UInt32 size = sizeof(s);
  if (AudioObjectGetPropertyData(obj, &a, 0, NULL, &size, &s) != noErr || !s) return NULL;
  CFIndex max = CFStringGetMaximumSizeForEncoding(CFStringGetLength(s), kCFStringEncodingUTF8) + 1;
  char* out = malloc(max);
  if (out && !CFStringGetCString(s, out, max, kCFStringEncodingUTF8)) {
    free(out);
    out = NULL;
  }
  CFRelease(s);
  return out;
}

// can_play: the device has output streams, is alive and is not hidden.
static int can_play(AudioObjectID dev) {
  AudioObjectPropertyAddress a = {kAudioDevicePropertyStreams, kAudioObjectPropertyScopeOutput,
                                  kAudioObjectPropertyElementMain};
  UInt32 size = 0;
  if (AudioObjectGetPropertyDataSize(dev, &a, 0, NULL, &size) != noErr || size == 0) return 0;
  return u32_prop(dev, kAudioDevicePropertyDeviceIsAlive, 0) && !u32_prop(dev, kAudioDevicePropertyIsHidden, 0);
}

static AudioObjectID default_output(void) {
  return u32_prop(kAudioObjectSystemObject, kAudioHardwarePropertyDefaultOutputDevice, kAudioObjectUnknown);
}

// all_devices sets *ids to every audio device (malloc'd; NULL when there
// is none) and *n to their count. Returns -1 if Core Audio fails.
static int all_devices(AudioObjectID** ids, int* n) {
  *ids = NULL;
  *n = 0;
  AudioObjectPropertyAddress a = global(kAudioHardwarePropertyDevices);
  UInt32 size = 0;
  if (AudioObjectGetPropertyDataSize(kAudioObjectSystemObject, &a, 0, NULL, &size) != noErr) return -1;
  if (size == 0) return 0;
  AudioObjectID* got = malloc(size);
  if (!got) return -1;
  if (AudioObjectGetPropertyData(kAudioObjectSystemObject, &a, 0, NULL, &size, got) != noErr) {
    free(got);
    return -1;
  }
  *ids = got;
  *n = (int)(size / sizeof(AudioObjectID));
  return 0;
}

int sona_gst_audio_sinks(sona_device** out, int* n) {
  *out = NULL;
  *n = 0;
  int count = 0;
  AudioObjectID* ids = NULL;
  if (all_devices(&ids, &count) != 0) return -1;
  if (count == 0) return 0;
  sona_device* res = calloc(count, sizeof(sona_device));
  if (!res) {
    free(ids);
    return -1;
  }
  AudioObjectID def = default_output();
  int k = 0;
  for (int i = 0; i < count; i++) {
    if (!can_play(ids[i])) continue;
    char* uid = str_prop(ids[i], kAudioDevicePropertyDeviceUID);
    if (!uid) continue;
    char* desc = str_prop(ids[i], kAudioObjectPropertyName);
    res[k].name = uid;
    res[k].description = desc ? desc : strdup(uid);
    res[k].is_default = ids[i] == def;
    k++;
  }
  free(ids);
  *out = res;
  *n = k;
  return 0;
}

void sona_gst_free_devices(sona_device* d, int n) {
  for (int i = 0; i < n; i++) {
    free(d[i].name);
    free(d[i].description);
  }
  free(d);
}

static _Atomic uint64_t route_epoch;
static pthread_once_t route_once = PTHREAD_ONCE_INIT;

static OSStatus route_changed(AudioObjectID obj, UInt32 n, const AudioObjectPropertyAddress* addrs, void* data) {
  (void)obj, (void)n, (void)addrs, (void)data;
  atomic_fetch_add(&route_epoch, 1);
  return noErr;
}

static void watch_route(void) {
  // Notifications on the HAL's own thread: no run loop runs for them in the
  // host (osxaudiosink sets the same when it opens).
  AudioObjectPropertyAddress rl = global(kAudioHardwarePropertyRunLoop);
  CFRunLoopRef none = NULL;
  AudioObjectSetPropertyData(kAudioObjectSystemObject, &rl, 0, NULL, sizeof(none), &none);
  AudioObjectPropertyAddress def = global(kAudioHardwarePropertyDefaultOutputDevice);
  AudioObjectPropertyAddress devs = global(kAudioHardwarePropertyDevices);
  AudioObjectAddPropertyListener(kAudioObjectSystemObject, &def, route_changed, NULL);
  AudioObjectAddPropertyListener(kAudioObjectSystemObject, &devs, route_changed, NULL);
}

uint64_t sona_route_epoch(void) {
  pthread_once(&route_once, watch_route);
  return atomic_load(&route_epoch);
}

char* sona_output_target(const char* uid) {
  pthread_once(&route_once, watch_route);
  if (uid && *uid) {
    int count = 0;
    AudioObjectID* ids = NULL;
    all_devices(&ids, &count);  // on failure: no device, so the default
    char* found = NULL;
    for (int i = 0; i < count && !found; i++) {
      if (!can_play(ids[i])) continue;
      char* u = str_prop(ids[i], kAudioDevicePropertyDeviceUID);
      if (u && strcmp(u, uid) == 0) {
        found = u;
      } else {
        free(u);
      }
    }
    free(ids);
    if (found) return found;
  }
  AudioObjectID def = default_output();
  return def == kAudioObjectUnknown ? NULL : str_prop(def, kAudioDevicePropertyDeviceUID);
}
