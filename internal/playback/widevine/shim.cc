//go:build !windows

// Sona's Chromium CDM host. It implements the host side of the CDM API from
// the vendored content_decryption_module.h (chromium/cdm d6c4e1ea, BSD-3,
// see LICENSE.chromium) and forwards every callback to Go.
//
// Linux and macOS: cgo compiles this file into the Go program. Windows: cgo
// must not (the line above), because Google's CDM uses the MSVC C++ ABI;
// build/windows/build-shim.sh builds it with MSVC into sona_cdm_shim.dll (see
// shim.h).

#include "shim.h"

#ifdef _WIN32
#define WIN32_LEAN_AND_MEAN
#define NOMINMAX
#include <windows.h>
#else
#include <dlfcn.h>
#include <sys/time.h>
#endif

#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <mutex>
#include <string>

#include "content_decryption_module.h"

#ifndef _WIN32
// Implemented in Go (cdm.go).
extern "C" void sonaCdmEvent(uintptr_t handle, int kind, uint32_t a, uint32_t b, uint32_t c, double t, uintptr_t ptr,
                             const char* sid, uint32_t sid_size, const char* data, uint32_t data_size);
#endif

static_assert(sizeof(cdm::SubsampleEntry) == 2 * sizeof(uint32_t), "SubsampleEntry layout");

namespace {

const char kKeySystem[] = "com.widevine.alpha";

using CreateCdmFunc = void* (*)(int, const char*, uint32_t, GetCdmHostFunc, void*);
using VersionFunc = const char* (*)();
using InitializeFunc = void (*)();

std::mutex g_module_mu;
void* g_module = nullptr;
std::string g_module_path;
CreateCdmFunc g_create = nullptr;
VersionFunc g_version = nullptr;

#ifdef _WIN32
sona_cdm_event_fn g_event_fn = nullptr;
#endif

// deliver hands one host callback to Go.
void deliver(uintptr_t handle, int kind, uint32_t a, uint32_t b, uint32_t c, double t, uintptr_t ptr, const char* sid,
             uint32_t sid_size, const char* data, uint32_t data_size) {
#ifdef _WIN32
  if (!g_event_fn) return;
  sona_cdm_event ev = {kind, a, b, c, t, ptr, sid, sid_size, data, data_size};
  g_event_fn(handle, &ev);
#else
  sonaCdmEvent(handle, kind, a, b, c, t, ptr, sid, sid_size, data, data_size);
#endif
}

// openLibrary loads the library at path (UTF-8), or returns null with the
// system's reason in err.
void* openLibrary(const char* path, char* err, size_t errlen) {
#ifdef _WIN32
  int n = MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, path, -1, nullptr, 0);
  if (n <= 0) {
    std::snprintf(err, errlen, "%s is not valid UTF-8", path);
    return nullptr;
  }
  std::wstring wide(static_cast<size_t>(n), L'\0');
  MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, path, -1, &wide[0], n);
  // The CDM's own directory is searched for its dependencies.
  HMODULE lib = LoadLibraryExW(wide.c_str(), nullptr, LOAD_WITH_ALTERED_SEARCH_PATH);
  if (!lib) {
    DWORD code = GetLastError();
    char* msg = nullptr;
    FormatMessageA(FORMAT_MESSAGE_ALLOCATE_BUFFER | FORMAT_MESSAGE_FROM_SYSTEM | FORMAT_MESSAGE_IGNORE_INSERTS, nullptr,
                   code, 0, reinterpret_cast<char*>(&msg), 0, nullptr);
    std::string text = msg ? msg : "";
    if (msg) LocalFree(msg);
    while (!text.empty() && (text.back() == '\r' || text.back() == '\n' || text.back() == ' ')) text.pop_back();
    std::snprintf(err, errlen, "LoadLibrary failed (error %lu): %s", static_cast<unsigned long>(code), text.c_str());
  }
  return lib;
#else
  void* lib = dlopen(path, RTLD_NOW | RTLD_LOCAL);
  if (!lib) std::snprintf(err, errlen, "%s", dlerror());
  return lib;
#endif
}

void* librarySymbol(void* lib, const char* name) {
#ifdef _WIN32
  return reinterpret_cast<void*>(GetProcAddress(static_cast<HMODULE>(lib), name));
#else
  return dlsym(lib, name);
#endif
}

void closeLibrary(void* lib) {
#ifdef _WIN32
  FreeLibrary(static_cast<HMODULE>(lib));
#else
  dlclose(lib);
#endif
}

class HeapBuffer final : public cdm::Buffer {
 public:
  explicit HeapBuffer(uint32_t capacity)
      : data_(static_cast<uint8_t*>(std::malloc(capacity ? capacity : 1))), capacity_(capacity) {}
  void Destroy() override { delete this; }
  uint32_t Capacity() const override { return capacity_; }
  uint8_t* Data() override { return data_; }
  void SetSize(uint32_t size) override { size_ = size <= capacity_ ? size : capacity_; }
  uint32_t Size() const override { return size_; }
  bool ok() const { return data_ != nullptr; }

 private:
  ~HeapBuffer() override { std::free(data_); }
  uint8_t* data_;
  uint32_t capacity_;
  uint32_t size_ = 0;
};

class Block final : public cdm::DecryptedBlock {
 public:
  ~Block() override {
    if (buffer_) buffer_->Destroy();
  }
  void SetDecryptedBuffer(cdm::Buffer* buffer) override {
    if (buffer_) buffer_->Destroy();
    buffer_ = buffer;
  }
  cdm::Buffer* DecryptedBuffer() override { return buffer_; }
  void SetTimestamp(int64_t ts) override { ts_ = ts; }
  int64_t Timestamp() const override { return ts_; }

 private:
  cdm::Buffer* buffer_ = nullptr;
  int64_t ts_ = 0;
};

// Frame receives one decoded picture from DecryptAndDecodeFrame.
class Frame final : public cdm::VideoFrame {
 public:
  ~Frame() override {
    if (buffer_) buffer_->Destroy();
  }
  void SetFormat(cdm::VideoFormat format) override { format_ = format; }
  cdm::VideoFormat Format() const override { return format_; }
  void SetSize(cdm::Size size) override { size_ = size; }
  cdm::Size Size() const override { return size_; }
  void SetFrameBuffer(cdm::Buffer* buffer) override {
    if (buffer_ && buffer_ != buffer) buffer_->Destroy();
    buffer_ = buffer;
  }
  cdm::Buffer* FrameBuffer() override { return buffer_; }
  void SetPlaneOffset(cdm::VideoPlane plane, uint32_t offset) override {
    if (plane < cdm::kMaxPlanes) offset_[plane] = offset;
  }
  uint32_t PlaneOffset(cdm::VideoPlane plane) override { return plane < cdm::kMaxPlanes ? offset_[plane] : 0; }
  void SetStride(cdm::VideoPlane plane, uint32_t stride) override {
    if (plane < cdm::kMaxPlanes) stride_[plane] = stride;
  }
  uint32_t Stride(cdm::VideoPlane plane) override { return plane < cdm::kMaxPlanes ? stride_[plane] : 0; }
  void SetTimestamp(int64_t ts) override { ts_ = ts; }
  int64_t Timestamp() const override { return ts_; }

 private:
  cdm::VideoFormat format_ = cdm::kUnknownVideoFormat;
  cdm::Size size_ = {0, 0};
  cdm::Buffer* buffer_ = nullptr;
  uint32_t offset_[cdm::kMaxPlanes] = {};
  uint32_t stride_[cdm::kMaxPlanes] = {};
  int64_t ts_ = 0;
};

// Host implements both host interface versions this CDM generation may ask
// for; Host_11 only appends ReportMetrics to Host_10.
class Host final : public cdm::Host_10, public cdm::Host_11 {
 public:
  explicit Host(uintptr_t handle) : handle_(handle) {}
  ~Host() override = default;

  int requested_version = 0;

  cdm::Buffer* Allocate(uint32_t capacity) override {
    auto* b = new HeapBuffer(capacity);
    if (!b->ok()) {
      b->Destroy();
      return nullptr;
    }
    return b;
  }
  void SetTimer(int64_t delay_ms, void* context) override {
    emit(SONA_EV_TIMER, 0, 0, 0, static_cast<double>(delay_ms), reinterpret_cast<uintptr_t>(context));
  }
  cdm::Time GetCurrentWallTime() override {
#ifdef _WIN32
    // 100 ns ticks since 1601-01-01; the CDM wants seconds since 1970.
    FILETIME ft;
    GetSystemTimePreciseAsFileTime(&ft);
    ULARGE_INTEGER ticks;
    ticks.LowPart = ft.dwLowDateTime;
    ticks.HighPart = ft.dwHighDateTime;
    return static_cast<double>(ticks.QuadPart - 116444736000000000ULL) / 1e7;
#else
    timeval tv;
    gettimeofday(&tv, nullptr);
    return static_cast<double>(tv.tv_sec) + static_cast<double>(tv.tv_usec) / 1e6;
#endif
  }
  void OnInitialized(bool success) override { emit(SONA_EV_INITIALIZED, success ? 1 : 0); }
  void OnResolveKeyStatusPromise(uint32_t promise_id, cdm::KeyStatus status) override {
    emit(SONA_EV_KEY_STATUS_PROMISE, promise_id, status);
  }
  void OnResolveNewSessionPromise(uint32_t promise_id, const char* session_id, uint32_t session_id_size) override {
    emit(SONA_EV_NEW_SESSION, promise_id, 0, 0, 0, 0, session_id, session_id_size);
  }
  void OnResolvePromise(uint32_t promise_id) override { emit(SONA_EV_PROMISE_RESOLVED, promise_id); }
  void OnRejectPromise(uint32_t promise_id, cdm::Exception exception, uint32_t system_code, const char* message,
                       uint32_t message_size) override {
    emit(SONA_EV_PROMISE_REJECTED, promise_id, exception, system_code, 0, 0, nullptr, 0, message, message_size);
  }
  void OnSessionMessage(const char* session_id, uint32_t session_id_size, cdm::MessageType type, const char* message,
                        uint32_t message_size) override {
    emit(SONA_EV_MESSAGE, type, 0, 0, 0, 0, session_id, session_id_size, message, message_size);
  }
  void OnSessionKeysChange(const char* session_id, uint32_t session_id_size, bool has_additional_usable_key,
                           const cdm::KeyInformation* keys, uint32_t count) override {
    for (uint32_t i = 0; i < count; i++) {
      emit(SONA_EV_KEY_STATUS, keys[i].status, keys[i].system_code, 0, 0, 0, session_id, session_id_size,
           reinterpret_cast<const char*>(keys[i].key_id), keys[i].key_id_size);
    }
    emit(SONA_EV_KEYS_CHANGED, has_additional_usable_key ? 1 : 0, count, 0, 0, 0, session_id, session_id_size);
  }
  void OnExpirationChange(const char* session_id, uint32_t session_id_size, cdm::Time expiry) override {
    emit(SONA_EV_EXPIRATION, 0, 0, 0, expiry, 0, session_id, session_id_size);
  }
  void OnSessionClosed(const char* session_id, uint32_t session_id_size) override {
    emit(SONA_EV_SESSION_CLOSED, 0, 0, 0, 0, 0, session_id, session_id_size);
  }
  void SendPlatformChallenge(const char*, uint32_t, const char*, uint32_t) override {
    emit(SONA_EV_PLATFORM_CHALLENGE);
  }
  void EnableOutputProtection(uint32_t) override {}
  void QueryOutputProtectionStatus() override { emit(SONA_EV_QUERY_OUTPUT_PROTECTION); }
  void OnDeferredInitializationDone(cdm::StreamType, cdm::Status) override {}
  // Persistent state is disabled at Initialize, so the CDM gets no storage.
  cdm::FileIO* CreateFileIO(cdm::FileIOClient*) override { return nullptr; }
  void RequestStorageId(uint32_t version) override { emit(SONA_EV_STORAGE_ID, version); }
  void ReportMetrics(cdm::MetricName, uint64_t) override {}

 private:
  void emit(int kind, uint32_t a = 0, uint32_t b = 0, uint32_t c = 0, double t = 0, uintptr_t ptr = 0,
            const char* sid = nullptr, uint32_t sid_size = 0, const char* data = nullptr, uint32_t data_size = 0) {
    deliver(handle_, kind, a, b, c, t, ptr, sid, sid_size, data, data_size);
  }
  uintptr_t handle_;
};

void* GetHost(int version, void* user_data) {
  auto* host = static_cast<Host*>(user_data);
  switch (version) {
    case cdm::Host_10::kVersion:
      host->requested_version = version;
      return static_cast<cdm::Host_10*>(host);
    case cdm::Host_11::kVersion:
      host->requested_version = version;
      return static_cast<cdm::Host_11*>(host);
    default:
      return nullptr;
  }
}

}  // namespace

struct sona_cdm {
  explicit sona_cdm(uintptr_t handle) : host(handle) {}
  Host host;
  int iface = 0;
  // CDM_10 and CDM_11 have identical vtables; exactly one is set.
  cdm::ContentDecryptionModule_10* cdm10 = nullptr;
  cdm::ContentDecryptionModule_11* cdm11 = nullptr;

  template <typename F>
  auto call(F f) {
    return cdm11 ? f(cdm11) : f(cdm10);
  }
};

extern "C" {

#ifdef _WIN32
void sona_shim_set_event_handler(sona_cdm_event_fn fn) { g_event_fn = fn; }

#endif
int sona_cdm_module_load(const char* path, char* err, size_t errlen) {
  std::lock_guard<std::mutex> lock(g_module_mu);
  if (g_module) {
    if (g_module_path == path) return 0;
    std::snprintf(err, errlen, "a different CDM is already loaded: %s", g_module_path.c_str());
    return -1;
  }
  void* lib = openLibrary(path, err, errlen);
  if (!lib) return -1;
  auto create = reinterpret_cast<CreateCdmFunc>(librarySymbol(lib, "CreateCdmInstance"));
  auto version = reinterpret_cast<VersionFunc>(librarySymbol(lib, "GetCdmVersion"));
  auto init = reinterpret_cast<InitializeFunc>(librarySymbol(lib, "InitializeCdmModule_4"));
  if (!create || !version) {
    std::snprintf(err, errlen, "%s does not export CreateCdmInstance/GetCdmVersion", path);
    closeLibrary(lib);
    return -1;
  }
  if (init) init();
  g_module = lib;
  g_module_path = path;
  g_create = create;
  g_version = version;
  return 0;
}

const char* sona_cdm_module_version(void) {
  std::lock_guard<std::mutex> lock(g_module_mu);
  return g_version ? g_version() : "";
}

sona_cdm* sona_cdm_create(int iface, uintptr_t handle) {
  CreateCdmFunc create;
  {
    std::lock_guard<std::mutex> lock(g_module_mu);
    create = g_create;
  }
  if (!create) return nullptr;
  auto* c = new sona_cdm(handle);
  void* raw = create(iface, kKeySystem, sizeof(kKeySystem) - 1, GetHost, &c->host);
  if (!raw) {
    delete c;
    return nullptr;
  }
  c->iface = iface;
  if (iface == cdm::ContentDecryptionModule_11::kVersion) {
    c->cdm11 = static_cast<cdm::ContentDecryptionModule_11*>(raw);
  } else if (iface == cdm::ContentDecryptionModule_10::kVersion) {
    c->cdm10 = static_cast<cdm::ContentDecryptionModule_10*>(raw);
  } else {
    // Unknown interface: we cannot even Destroy() it safely.
    delete c;
    return nullptr;
  }
  return c;
}

int sona_cdm_interface(sona_cdm* c) { return c->iface; }
int sona_cdm_host_interface(sona_cdm* c) { return c->host.requested_version; }

void sona_cdm_initialize(sona_cdm* c, int allow_distinctive_identifier, int allow_persistent_state) {
  c->call([&](auto* m) {
    m->Initialize(allow_distinctive_identifier != 0, allow_persistent_state != 0, false);
    return 0;
  });
}

void sona_cdm_set_server_certificate(sona_cdm* c, uint32_t promise, const uint8_t* data, uint32_t size) {
  c->call([&](auto* m) {
    m->SetServerCertificate(promise, data, size);
    return 0;
  });
}

void sona_cdm_create_session(sona_cdm* c, uint32_t promise, uint32_t init_data_type, const uint8_t* data,
                             uint32_t size) {
  c->call([&](auto* m) {
    m->CreateSessionAndGenerateRequest(promise, cdm::kTemporary, static_cast<cdm::InitDataType>(init_data_type), data,
                                       size);
    return 0;
  });
}

void sona_cdm_update_session(sona_cdm* c, uint32_t promise, const char* sid, uint32_t sid_size, const uint8_t* data,
                             uint32_t size) {
  c->call([&](auto* m) {
    m->UpdateSession(promise, sid, sid_size, data, size);
    return 0;
  });
}

void sona_cdm_close_session(sona_cdm* c, uint32_t promise, const char* sid, uint32_t sid_size) {
  c->call([&](auto* m) {
    m->CloseSession(promise, sid, sid_size);
    return 0;
  });
}

void sona_cdm_timer_expired(sona_cdm* c, uintptr_t context) {
  c->call([&](auto* m) {
    m->TimerExpired(reinterpret_cast<void*>(context));
    return 0;
  });
}

void sona_cdm_output_protection_status(sona_cdm* c) {
  // Sona inspects no display links and claims no protection on them: the
  // only video keys a software CDM is issued (SD) require none, and HD
  // keys, which need HDCP, stay output-restricted and are never played.
  c->call([&](auto* m) {
    m->OnQueryOutputProtectionStatus(cdm::kQuerySucceeded, cdm::kLinkTypeNone, cdm::kProtectionNone);
    return 0;
  });
}

void sona_cdm_storage_id(sona_cdm* c, uint32_t version) {
  c->call([&](auto* m) {
    m->OnStorageId(version, nullptr, 0);
    return 0;
  });
}

void sona_cdm_platform_challenge_failed(sona_cdm* c) {
  cdm::PlatformChallengeResponse empty = {};
  c->call([&](auto* m) {
    m->OnPlatformChallengeResponse(empty);
    return 0;
  });
}

int sona_cdm_decrypt(sona_cdm* c, const uint8_t* kid, uint32_t kid_size, const uint8_t* iv, uint32_t iv_size,
                     const uint8_t* data, uint32_t size, const uint32_t* subsamples, uint32_t nsub, int64_t timestamp_us,
                     int cbcs, uint32_t crypt_blocks, uint32_t skip_blocks,
                     uint8_t* out, uint32_t out_cap, uint32_t* out_size) {
  cdm::SubsampleEntry whole = {0, size};
  cdm::InputBuffer_2 in = {};
  in.data = data;
  in.data_size = size;
  in.encryption_scheme = cbcs ? cdm::EncryptionScheme::kCbcs : cdm::EncryptionScheme::kCenc;
  if (cbcs) in.pattern = {crypt_blocks, skip_blocks};
  in.key_id = kid;
  in.key_id_size = kid_size;
  in.iv = iv;
  in.iv_size = iv_size;
  in.subsamples = nsub ? reinterpret_cast<const cdm::SubsampleEntry*>(subsamples) : &whole;
  in.num_subsamples = nsub ? nsub : 1;
  in.timestamp = timestamp_us;

  Block block;
  cdm::Status status = c->call([&](auto* m) { return m->Decrypt(in, &block); });
  if (status != cdm::kSuccess) return static_cast<int>(status);
  cdm::Buffer* buf = block.DecryptedBuffer();
  if (!buf) return -2;
  if (buf->Size() > out_cap) return -1;
  std::memmove(out, buf->Data(), buf->Size());
  *out_size = buf->Size();
  return 0;
}

int sona_cdm_video_init(sona_cdm* c, uint32_t codec, uint32_t profile, int32_t width, int32_t height,
                        const uint8_t* extra, uint32_t extra_size, int cbcs) {
  cdm::VideoDecoderConfig_2 cfg = {};
  cfg.codec = static_cast<cdm::VideoCodec>(codec);
  cfg.profile = static_cast<cdm::VideoCodecProfile>(profile);
  cfg.format = cdm::kI420;
  cfg.coded_size = {width, height};
  cfg.extra_data = const_cast<uint8_t*>(extra);
  cfg.extra_data_size = extra_size;
  cfg.encryption_scheme = cbcs ? cdm::EncryptionScheme::kCbcs : cdm::EncryptionScheme::kCenc;
  cdm::Status status = c->call([&](auto* m) { return m->InitializeVideoDecoder(cfg); });
  return static_cast<int>(status);
}

void sona_cdm_video_reset(sona_cdm* c) {
  c->call([&](auto* m) {
    m->ResetDecoder(cdm::kStreamTypeVideo);
    return 0;
  });
}

void sona_cdm_video_deinit(sona_cdm* c) {
  c->call([&](auto* m) {
    m->DeinitializeDecoder(cdm::kStreamTypeVideo);
    return 0;
  });
}

// copyPlane copies rows x cols bytes of a plane at offset (stride apart)
// to dst, failing if the plane runs past the buffer.
static bool copyPlane(cdm::Buffer* buf, uint32_t offset, uint32_t stride, int32_t cols, int32_t rows, uint8_t* dst) {
  if (rows <= 0 || cols <= 0) return true;
  uint64_t end = uint64_t(offset) + uint64_t(stride) * uint64_t(rows - 1) + uint64_t(cols);
  if (stride < uint32_t(cols) || end > buf->Size()) return false;
  const uint8_t* src = buf->Data() + offset;
  for (int32_t y = 0; y < rows; y++) {
    std::memcpy(dst, src, size_t(cols));
    dst += cols;
    src += stride;
  }
  return true;
}

int sona_cdm_video_decode(sona_cdm* c, const uint8_t* kid, uint32_t kid_size, const uint8_t* iv, uint32_t iv_size,
                          const uint8_t* data, uint32_t size, const uint32_t* subsamples, uint32_t nsub,
                          int64_t timestamp_us, int cbcs, uint32_t crypt_blocks, uint32_t skip_blocks,
                          sona_video_frame* out) {
  *out = {};
  cdm::InputBuffer_2 in = {};
  cdm::SubsampleEntry whole = {0, size};
  if (data && size) {
    in.data = data;
    in.data_size = size;
    in.timestamp = timestamp_us;
    if (kid && kid_size) {
      in.encryption_scheme = cbcs ? cdm::EncryptionScheme::kCbcs : cdm::EncryptionScheme::kCenc;
      if (cbcs) in.pattern = {crypt_blocks, skip_blocks};
      in.key_id = kid;
      in.key_id_size = kid_size;
      in.iv = iv;
      in.iv_size = iv_size;
      in.subsamples = nsub ? reinterpret_cast<const cdm::SubsampleEntry*>(subsamples) : &whole;
      in.num_subsamples = nsub ? nsub : 1;
    }
  }

  Frame frame;
  cdm::Status status = c->call([&](auto* m) { return m->DecryptAndDecodeFrame(in, &frame); });
  if (status != cdm::kSuccess) return static_cast<int>(status);
  cdm::Buffer* buf = frame.FrameBuffer();
  cdm::Size sz = frame.Size();
  if (!buf || sz.width <= 0 || sz.height <= 0) return -2;
  if (frame.Format() != cdm::kI420 && frame.Format() != cdm::kYv12) return -3;
  int32_t cw = (sz.width + 1) / 2, ch = (sz.height + 1) / 2;
  size_t ysize = size_t(sz.width) * size_t(sz.height), csize = size_t(cw) * size_t(ch);
  auto* dst = static_cast<uint8_t*>(std::malloc(ysize + 2 * csize));
  if (!dst) return -4;
  // The U and V plane offsets name the planes whatever their order in
  // memory (YV12 stores V first).
  if (!copyPlane(buf, frame.PlaneOffset(cdm::kYPlane), frame.Stride(cdm::kYPlane), sz.width, sz.height, dst) ||
      !copyPlane(buf, frame.PlaneOffset(cdm::kUPlane), frame.Stride(cdm::kUPlane), cw, ch, dst + ysize) ||
      !copyPlane(buf, frame.PlaneOffset(cdm::kVPlane), frame.Stride(cdm::kVPlane), cw, ch, dst + ysize + csize)) {
    std::free(dst);
    return -2;
  }
  out->data = dst;
  out->size = uint32_t(ysize + 2 * csize);
  out->width = sz.width;
  out->height = sz.height;
  out->timestamp_us = frame.Timestamp();
  return 0;
}

void sona_video_frame_free(sona_video_frame* f) {
  if (!f) return;
  std::free(f->data);
  f->data = nullptr;
}

void sona_cdm_destroy(sona_cdm* c) {
  if (!c) return;
  c->call([&](auto* m) {
    m->Destroy();
    return 0;
  });
  // The library stays mapped: the CDM's threads and TLS destructors may still
  // reference its code until process exit.
  delete c;
}

}  // extern "C"
