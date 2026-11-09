// C interface to Sona's Chromium CDM host shim (shim.cc). The shim loads a
// CDM library, implements cdm::Host_10/Host_11 and forwards every host
// callback to Go as an event. All calls on one sona_cdm must come from the
// thread that created it.
//
// On Linux and macOS the shim is compiled into the Go program by cgo and
// delivers events by calling the Go function sonaCdmEvent. On Windows
// Google's CDM is built with the MSVC C++ ABI, which MinGW (cgo's compiler)
// cannot speak, so shim.cc is built with MSVC into its own DLL
// (sona_cdm_shim.dll, build/windows/build-shim.sh) that exports exactly this
// C API, and shim_windows.c, which cgo compiles, defines the same functions
// as thunks over the DLL's exports and gives the DLL sonaCdmEvent's address
// (sona_shim_set_event_handler).
#ifndef SONA_WIDEVINE_SHIM_H_
#define SONA_WIDEVINE_SHIM_H_

#include <stddef.h>
#include <stdint.h>

// SONA_API marks what the DLL exports; it is empty everywhere else.
#if defined(_WIN32) && defined(SONA_SHIM_BUILD)
#define SONA_API __declspec(dllexport)
#else
#define SONA_API
#endif

#ifdef __cplusplus
extern "C" {
#endif

// Host callback kinds delivered to Go (sonaCdmEvent).
enum {
  SONA_EV_INITIALIZED = 1,      // a = success
  SONA_EV_PROMISE_RESOLVED,     // a = promise id
  SONA_EV_NEW_SESSION,          // a = promise id, sid = session id (empty: not found)
  SONA_EV_PROMISE_REJECTED,     // a = promise id, b = cdm::Exception, c = system code, data = message
  SONA_EV_KEY_STATUS_PROMISE,   // a = promise id, b = key status
  SONA_EV_MESSAGE,              // a = cdm::MessageType, sid, data = message
  SONA_EV_KEY_STATUS,           // a = cdm::KeyStatus, b = system code, sid, data = key id
  SONA_EV_KEYS_CHANGED,         // a = has additional usable key, b = key count, sid
  SONA_EV_EXPIRATION,           // t = new expiry (seconds since epoch, 0 = none), sid
  SONA_EV_SESSION_CLOSED,       // sid
  SONA_EV_TIMER,                // t = delay in ms, ptr = context
  SONA_EV_QUERY_OUTPUT_PROTECTION,
  SONA_EV_STORAGE_ID,           // a = version
  SONA_EV_PLATFORM_CHALLENGE,
};

typedef struct sona_cdm sona_cdm;

#ifdef _WIN32
// sona_cdm_event is one host callback as the DLL delivers it (the fields
// sonaCdmEvent takes on the other platforms).
typedef struct {
  int kind;
  uint32_t a, b, c;
  double t;
  uintptr_t ptr;
  const char* sid;
  uint32_t sid_size;
  const char* data;
  uint32_t data_size;
} sona_cdm_event;
typedef void (*sona_cdm_event_fn)(uintptr_t handle, const sona_cdm_event* ev);
// sona_shim_set_event_handler registers where the DLL delivers events,
// before the first sona_cdm_create.
SONA_API void sona_shim_set_event_handler(sona_cdm_event_fn fn);
#endif

// sona_cdm_module_load dlopens the CDM at path and runs its module
// initialiser. The module stays loaded for the life of the process; loading
// a different path later fails. Returns 0 or -1 with err filled.
SONA_API int sona_cdm_module_load(const char* path, char* err, size_t errlen);
// sona_cdm_module_version returns GetCdmVersion() of the loaded module.
SONA_API const char* sona_cdm_module_version(void);

// sona_cdm_create creates a CDM instance of interface version iface for
// com.widevine.alpha. handle is passed back with every event.
SONA_API sona_cdm* sona_cdm_create(int iface, uintptr_t handle);
SONA_API int sona_cdm_interface(sona_cdm* c);
SONA_API int sona_cdm_host_interface(sona_cdm* c);
SONA_API void sona_cdm_initialize(sona_cdm* c, int allow_distinctive_identifier, int allow_persistent_state);
SONA_API void sona_cdm_set_server_certificate(sona_cdm* c, uint32_t promise, const uint8_t* data, uint32_t size);
SONA_API void sona_cdm_create_session(sona_cdm* c, uint32_t promise, uint32_t init_data_type, const uint8_t* data, uint32_t size);
SONA_API void sona_cdm_update_session(sona_cdm* c, uint32_t promise, const char* sid, uint32_t sid_size, const uint8_t* data, uint32_t size);
SONA_API void sona_cdm_close_session(sona_cdm* c, uint32_t promise, const char* sid, uint32_t sid_size);
SONA_API void sona_cdm_timer_expired(sona_cdm* c, uintptr_t context);
// Answers to host requests the CDM made through events.
SONA_API void sona_cdm_output_protection_status(sona_cdm* c);
SONA_API void sona_cdm_storage_id(sona_cdm* c, uint32_t version);
SONA_API void sona_cdm_platform_challenge_failed(sona_cdm* c);

// sona_cdm_decrypt decrypts one 'cenc' sample, or one 'cbcs' sample when
// cbcs is non-zero (with the tenc crypt/skip block pattern). subsamples
// holds nsub (clear, cipher) uint32 pairs; nsub 0 means the whole sample is
// encrypted. The clear result is copied to out (which may alias data).
// Returns the cdm::Status (0 = success), -1 if out is too small, -2 if the
// CDM produced no output.
SONA_API int sona_cdm_decrypt(sona_cdm* c, const uint8_t* kid, uint32_t kid_size, const uint8_t* iv, uint32_t iv_size,
                     const uint8_t* data, uint32_t size, const uint32_t* subsamples, uint32_t nsub, int64_t timestamp_us,
                     int cbcs, uint32_t crypt_blocks, uint32_t skip_blocks,
                     uint8_t* out, uint32_t out_cap, uint32_t* out_size);

SONA_API void sona_cdm_destroy(sona_cdm* c);

// Video: the CDM's own decoder, the only way to use keys of level
// SW_SECURE_DECODE (Apple's music video keys), which Decrypt refuses.
// sona_cdm_video_init initialises it (cdm::VideoDecoderConfig_2 fields:
// codec, profile, I420 output, coded size, extra data, cbcs or cenc) and
// returns the cdm::Status (0 = success).
SONA_API int sona_cdm_video_init(sona_cdm* c, uint32_t codec, uint32_t profile, int32_t width, int32_t height,
                        const uint8_t* extra, uint32_t extra_size, int cbcs);
// sona_cdm_video_reset drops every frame the decoder holds (a seek).
SONA_API void sona_cdm_video_reset(sona_cdm* c);
// sona_cdm_video_deinit releases the decoder.
SONA_API void sona_cdm_video_deinit(sona_cdm* c);

// sona_video_frame is a decoded picture, its planes packed as I420 (Y,
// then U, then V; chroma (width+1)/2 by (height+1)/2) in data, which
// sona_video_frame_free releases.
typedef struct {
  uint8_t* data;
  uint32_t size;
  int32_t width, height;
  int64_t timestamp_us;
} sona_video_frame;

// sona_cdm_video_decode decrypts and decodes one access unit (arguments
// as sona_cdm_decrypt; data NULL and size 0 drain the decoder at the end
// of the stream). It returns the cdm::Status: 0 with a frame in out,
// 1 (kNeedMoreData) when no frame is ready yet (or, draining, none is
// left); -3 for a frame in a format other than I420/YV12, -4 when the
// frame's memory could not be allocated.
SONA_API int sona_cdm_video_decode(sona_cdm* c, const uint8_t* kid, uint32_t kid_size, const uint8_t* iv, uint32_t iv_size,
                          const uint8_t* data, uint32_t size, const uint32_t* subsamples, uint32_t nsub,
                          int64_t timestamp_us, int cbcs, uint32_t crypt_blocks, uint32_t skip_blocks,
                          sona_video_frame* out);
SONA_API void sona_video_frame_free(sona_video_frame* f);

#ifdef __cplusplus
}
#endif

#endif  // SONA_WIDEVINE_SHIM_H_
