// The Windows side of the CDM shim (shim.h): cgo compiles this file with
// MinGW, and the shim itself (shim.cc) is built with MSVC into
// sona_cdm_shim.dll, because Google's CDM speaks the MSVC C++ ABI. Every
// sona_cdm_* function here forwards to the DLL's export of the same name; the
// DLL is found next to the executable (the installer's layout), or at the
// path SONA_CDM_SHIM names (a development or test build, where the executable
// is a temporary file). It is loaded only when a CDM is, which happens in the
// CDM host child.

#include <stdio.h>
#include <string.h>
#include <windows.h>

#include "shim.h"

// Implemented in Go (cdm.go).
extern void sonaCdmEvent(uintptr_t handle, int kind, uint32_t a, uint32_t b, uint32_t c, double t, uintptr_t ptr,
                         const char* sid, uint32_t sid_size, const char* data, uint32_t data_size);

// onEvent adapts the DLL's event struct to sonaCdmEvent.
static void onEvent(uintptr_t handle, const sona_cdm_event* e) {
  sonaCdmEvent(handle, e->kind, e->a, e->b, e->c, e->t, e->ptr, e->sid, e->sid_size, e->data, e->data_size);
}

static struct {
  int (*module_load)(const char*, char*, size_t);
  const char* (*module_version)(void);
  void (*set_event_handler)(sona_cdm_event_fn);
  sona_cdm* (*create)(int, uintptr_t);
  int (*interface_version)(sona_cdm*);
  int (*host_interface)(sona_cdm*);
  void (*initialize)(sona_cdm*, int, int);
  void (*set_server_certificate)(sona_cdm*, uint32_t, const uint8_t*, uint32_t);
  void (*create_session)(sona_cdm*, uint32_t, uint32_t, const uint8_t*, uint32_t);
  void (*update_session)(sona_cdm*, uint32_t, const char*, uint32_t, const uint8_t*, uint32_t);
  void (*close_session)(sona_cdm*, uint32_t, const char*, uint32_t);
  void (*timer_expired)(sona_cdm*, uintptr_t);
  void (*output_protection_status)(sona_cdm*);
  void (*storage_id)(sona_cdm*, uint32_t);
  void (*platform_challenge_failed)(sona_cdm*);
  int (*decrypt)(sona_cdm*, const uint8_t*, uint32_t, const uint8_t*, uint32_t, const uint8_t*, uint32_t, const uint32_t*,
                 uint32_t, int64_t, int, uint32_t, uint32_t, uint8_t*, uint32_t, uint32_t*);
  void (*destroy)(sona_cdm*);
  int (*video_init)(sona_cdm*, uint32_t, uint32_t, int32_t, int32_t, const uint8_t*, uint32_t, int);
  void (*video_reset)(sona_cdm*);
  void (*video_deinit)(sona_cdm*);
  int (*video_decode)(sona_cdm*, const uint8_t*, uint32_t, const uint8_t*, uint32_t, const uint8_t*, uint32_t,
                      const uint32_t*, uint32_t, int64_t, int, uint32_t, uint32_t, sona_video_frame*);
  void (*video_frame_free)(sona_video_frame*);
} fn;

static HMODULE g_shim;
static INIT_ONCE g_once = INIT_ONCE_STATIC_INIT;
static char g_err[512];

#define RESOLVE(field, name)                                       \
  do {                                                             \
    FARPROC p_ = GetProcAddress(g_shim, name);                     \
    if (!p_) {                                                     \
      snprintf(g_err, sizeof g_err, "%s lacks %s", g_path, name);  \
      return FALSE;                                                \
    }                                                              \
    memcpy(&fn.field, &p_, sizeof p_);                             \
  } while (0)

static char g_path[MAX_PATH * 3];

// shimPath fills g_path: $SONA_CDM_SHIM, else sona_cdm_shim.dll beside the
// running executable.
static BOOL shimPath(void) {
  WCHAR wide[MAX_PATH];
  DWORD n = GetEnvironmentVariableW(L"SONA_CDM_SHIM", wide, MAX_PATH);
  if (n == 0 || n >= MAX_PATH) {
    n = GetModuleFileNameW(NULL, wide, MAX_PATH);
    if (n == 0 || n >= MAX_PATH) {
      snprintf(g_err, sizeof g_err, "cannot find the running executable (error %lu)", (unsigned long)GetLastError());
      return FALSE;
    }
    WCHAR* slash = wcsrchr(wide, L'\\');
    if (!slash) {
      snprintf(g_err, sizeof g_err, "the executable's path has no directory");
      return FALSE;
    }
    slash[1] = 0;
    if (wcslen(wide) + wcslen(L"sona_cdm_shim.dll") >= MAX_PATH) {
      snprintf(g_err, sizeof g_err, "the executable's path is too long");
      return FALSE;
    }
    wcscat(wide, L"sona_cdm_shim.dll");
  }
  if (WideCharToMultiByte(CP_UTF8, 0, wide, -1, g_path, sizeof g_path, NULL, NULL) == 0) {
    snprintf(g_err, sizeof g_err, "cannot convert the shim's path");
    return FALSE;
  }
  g_shim = LoadLibraryExW(wide, NULL, LOAD_WITH_ALTERED_SEARCH_PATH);
  if (!g_shim) {
    snprintf(g_err, sizeof g_err, "cannot load %s (error %lu)", g_path, (unsigned long)GetLastError());
    return FALSE;
  }
  return TRUE;
}

static BOOL CALLBACK openShim(PINIT_ONCE once, PVOID param, PVOID* ctx) {
  (void)once;
  (void)param;
  (void)ctx;
  if (!shimPath()) return FALSE;
  RESOLVE(module_load, "sona_cdm_module_load");
  RESOLVE(module_version, "sona_cdm_module_version");
  RESOLVE(set_event_handler, "sona_shim_set_event_handler");
  RESOLVE(create, "sona_cdm_create");
  RESOLVE(interface_version, "sona_cdm_interface");
  RESOLVE(host_interface, "sona_cdm_host_interface");
  RESOLVE(initialize, "sona_cdm_initialize");
  RESOLVE(set_server_certificate, "sona_cdm_set_server_certificate");
  RESOLVE(create_session, "sona_cdm_create_session");
  RESOLVE(update_session, "sona_cdm_update_session");
  RESOLVE(close_session, "sona_cdm_close_session");
  RESOLVE(timer_expired, "sona_cdm_timer_expired");
  RESOLVE(output_protection_status, "sona_cdm_output_protection_status");
  RESOLVE(storage_id, "sona_cdm_storage_id");
  RESOLVE(platform_challenge_failed, "sona_cdm_platform_challenge_failed");
  RESOLVE(decrypt, "sona_cdm_decrypt");
  RESOLVE(destroy, "sona_cdm_destroy");
  RESOLVE(video_init, "sona_cdm_video_init");
  RESOLVE(video_reset, "sona_cdm_video_reset");
  RESOLVE(video_deinit, "sona_cdm_video_deinit");
  RESOLVE(video_decode, "sona_cdm_video_decode");
  RESOLVE(video_frame_free, "sona_video_frame_free");
  fn.set_event_handler(onEvent);
  return TRUE;
}

int sona_cdm_module_load(const char* path, char* err, size_t errlen) {
  if (!InitOnceExecuteOnce(&g_once, openShim, NULL, NULL)) {
    snprintf(err, errlen, "the CDM shim is unavailable: %s", g_err);
    return -1;
  }
  return fn.module_load(path, err, errlen);
}

const char* sona_cdm_module_version(void) { return fn.module_version ? fn.module_version() : ""; }
sona_cdm* sona_cdm_create(int iface, uintptr_t handle) { return fn.create ? fn.create(iface, handle) : NULL; }
int sona_cdm_interface(sona_cdm* c) { return fn.interface_version(c); }
int sona_cdm_host_interface(sona_cdm* c) { return fn.host_interface(c); }
void sona_cdm_initialize(sona_cdm* c, int allow_distinctive_identifier, int allow_persistent_state) {
  fn.initialize(c, allow_distinctive_identifier, allow_persistent_state);
}
void sona_cdm_set_server_certificate(sona_cdm* c, uint32_t promise, const uint8_t* data, uint32_t size) {
  fn.set_server_certificate(c, promise, data, size);
}
void sona_cdm_create_session(sona_cdm* c, uint32_t promise, uint32_t init_data_type, const uint8_t* data, uint32_t size) {
  fn.create_session(c, promise, init_data_type, data, size);
}
void sona_cdm_update_session(sona_cdm* c, uint32_t promise, const char* sid, uint32_t sid_size, const uint8_t* data,
                             uint32_t size) {
  fn.update_session(c, promise, sid, sid_size, data, size);
}
void sona_cdm_close_session(sona_cdm* c, uint32_t promise, const char* sid, uint32_t sid_size) {
  fn.close_session(c, promise, sid, sid_size);
}
void sona_cdm_timer_expired(sona_cdm* c, uintptr_t context) { fn.timer_expired(c, context); }
void sona_cdm_output_protection_status(sona_cdm* c) { fn.output_protection_status(c); }
void sona_cdm_storage_id(sona_cdm* c, uint32_t version) { fn.storage_id(c, version); }
void sona_cdm_platform_challenge_failed(sona_cdm* c) { fn.platform_challenge_failed(c); }

int sona_cdm_decrypt(sona_cdm* c, const uint8_t* kid, uint32_t kid_size, const uint8_t* iv, uint32_t iv_size,
                     const uint8_t* data, uint32_t size, const uint32_t* subsamples, uint32_t nsub, int64_t timestamp_us,
                     int cbcs, uint32_t crypt_blocks, uint32_t skip_blocks, uint8_t* out, uint32_t out_cap,
                     uint32_t* out_size) {
  return fn.decrypt(c, kid, kid_size, iv, iv_size, data, size, subsamples, nsub, timestamp_us, cbcs, crypt_blocks,
                    skip_blocks, out, out_cap, out_size);
}

void sona_cdm_destroy(sona_cdm* c) { fn.destroy(c); }

int sona_cdm_video_init(sona_cdm* c, uint32_t codec, uint32_t profile, int32_t width, int32_t height, const uint8_t* extra,
                        uint32_t extra_size, int cbcs) {
  return fn.video_init(c, codec, profile, width, height, extra, extra_size, cbcs);
}
void sona_cdm_video_reset(sona_cdm* c) { fn.video_reset(c); }
void sona_cdm_video_deinit(sona_cdm* c) { fn.video_deinit(c); }

int sona_cdm_video_decode(sona_cdm* c, const uint8_t* kid, uint32_t kid_size, const uint8_t* iv, uint32_t iv_size,
                          const uint8_t* data, uint32_t size, const uint32_t* subsamples, uint32_t nsub,
                          int64_t timestamp_us, int cbcs, uint32_t crypt_blocks, uint32_t skip_blocks,
                          sona_video_frame* out) {
  return fn.video_decode(c, kid, kid_size, iv, iv_size, data, size, subsamples, nsub, timestamp_us, cbcs, crypt_blocks,
                         skip_blocks, out);
}
void sona_video_frame_free(sona_video_frame* f) { fn.video_frame_free(f); }
