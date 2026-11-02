// C interface to Sona's Chromium CDM host shim (shim.cc). The shim loads a
// CDM library, implements cdm::Host_10/Host_11 and forwards every host
// callback to Go as an event. All calls on one sona_cdm must come from the
// thread that created it.
#ifndef SONA_WIDEVINE_SHIM_H_
#define SONA_WIDEVINE_SHIM_H_

#include <stddef.h>
#include <stdint.h>

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

// sona_cdm_module_load dlopens the CDM at path and runs its module
// initialiser. The module stays loaded for the life of the process; loading
// a different path later fails. Returns 0 or -1 with err filled.
int sona_cdm_module_load(const char* path, char* err, size_t errlen);
// sona_cdm_module_version returns GetCdmVersion() of the loaded module.
const char* sona_cdm_module_version(void);

// sona_cdm_create creates a CDM instance of interface version iface for
// com.widevine.alpha. handle is passed back with every event.
sona_cdm* sona_cdm_create(int iface, uintptr_t handle);
int sona_cdm_interface(sona_cdm* c);
int sona_cdm_host_interface(sona_cdm* c);
void sona_cdm_initialize(sona_cdm* c, int allow_distinctive_identifier, int allow_persistent_state);
void sona_cdm_set_server_certificate(sona_cdm* c, uint32_t promise, const uint8_t* data, uint32_t size);
void sona_cdm_create_session(sona_cdm* c, uint32_t promise, uint32_t init_data_type, const uint8_t* data, uint32_t size);
void sona_cdm_update_session(sona_cdm* c, uint32_t promise, const char* sid, uint32_t sid_size, const uint8_t* data, uint32_t size);
void sona_cdm_close_session(sona_cdm* c, uint32_t promise, const char* sid, uint32_t sid_size);
void sona_cdm_timer_expired(sona_cdm* c, uintptr_t context);
// Answers to host requests the CDM made through events.
void sona_cdm_output_protection_status(sona_cdm* c);
void sona_cdm_storage_id(sona_cdm* c, uint32_t version);
void sona_cdm_platform_challenge_failed(sona_cdm* c);

// sona_cdm_decrypt decrypts one 'cenc' sample, or one 'cbcs' sample when
// cbcs is non-zero (with the tenc crypt/skip block pattern). subsamples
// holds nsub (clear, cipher) uint32 pairs; nsub 0 means the whole sample is
// encrypted. The clear result is copied to out (which may alias data).
// Returns the cdm::Status (0 = success), -1 if out is too small, -2 if the
// CDM produced no output.
int sona_cdm_decrypt(sona_cdm* c, const uint8_t* kid, uint32_t kid_size, const uint8_t* iv, uint32_t iv_size,
                     const uint8_t* data, uint32_t size, const uint32_t* subsamples, uint32_t nsub, int64_t timestamp_us,
                     int cbcs, uint32_t crypt_blocks, uint32_t skip_blocks,
                     uint8_t* out, uint32_t out_cap, uint32_t* out_size);

void sona_cdm_destroy(sona_cdm* c);

// Video: the CDM's own decoder, the only way to use keys of level
// SW_SECURE_DECODE (Apple's music video keys), which Decrypt refuses.
// sona_cdm_video_init initialises it (cdm::VideoDecoderConfig_2 fields:
// codec, profile, I420 output, coded size, extra data, cbcs or cenc) and
// returns the cdm::Status (0 = success).
int sona_cdm_video_init(sona_cdm* c, uint32_t codec, uint32_t profile, int32_t width, int32_t height,
                        const uint8_t* extra, uint32_t extra_size, int cbcs);
// sona_cdm_video_reset drops every frame the decoder holds (a seek).
void sona_cdm_video_reset(sona_cdm* c);
// sona_cdm_video_deinit releases the decoder.
void sona_cdm_video_deinit(sona_cdm* c);

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
int sona_cdm_video_decode(sona_cdm* c, const uint8_t* kid, uint32_t kid_size, const uint8_t* iv, uint32_t iv_size,
                          const uint8_t* data, uint32_t size, const uint32_t* subsamples, uint32_t nsub,
                          int64_t timestamp_us, int cbcs, uint32_t crypt_blocks, uint32_t skip_blocks,
                          sona_video_frame* out);
void sona_video_frame_free(sona_video_frame* f);

#ifdef __cplusplus
}
#endif

#endif  // SONA_WIDEVINE_SHIM_H_
