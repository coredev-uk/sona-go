// The chosen output's state and the endpoint watch on Windows (Core
// Audio's MMDevice API): what streamProps and routeEpoch read in
// output_windows.go.
#define _WIN32_WINNT 0x0A00
#define COBJMACROS
#include <initguid.h>
#include <windows.h>
#include <mmdeviceapi.h>
#include <stdatomic.h>
#include <stdint.h>
#include <stdlib.h>

#include "devices.h"

static _Atomic uint64_t route_epoch;
static INIT_ONCE watch_once = INIT_ONCE_STATIC_INIT;

static void bump(void) { atomic_fetch_add(&route_epoch, 1); }

// The notification client: one static object, never freed, so its
// reference count is moot.
static HRESULT STDMETHODCALLTYPE nc_query(IMMNotificationClient* self, REFIID riid, void** out) {
  if (IsEqualIID(riid, &IID_IUnknown) || IsEqualIID(riid, &IID_IMMNotificationClient)) {
    *out = self;
    return S_OK;
  }
  *out = NULL;
  return E_NOINTERFACE;
}
static ULONG STDMETHODCALLTYPE nc_addref(IMMNotificationClient* self) {
  (void)self;
  return 1;
}
static ULONG STDMETHODCALLTYPE nc_release(IMMNotificationClient* self) {
  (void)self;
  return 1;
}
static HRESULT STDMETHODCALLTYPE nc_state(IMMNotificationClient* self, LPCWSTR id, DWORD state) {
  (void)self, (void)id, (void)state;
  bump();
  return S_OK;
}
static HRESULT STDMETHODCALLTYPE nc_added(IMMNotificationClient* self, LPCWSTR id) {
  (void)self, (void)id;
  bump();
  return S_OK;
}
static HRESULT STDMETHODCALLTYPE nc_removed(IMMNotificationClient* self, LPCWSTR id) {
  (void)self, (void)id;
  bump();
  return S_OK;
}
// A default change needs no move: wasapi2sink on the default follows it
// itself (automatic stream routing).
static HRESULT STDMETHODCALLTYPE nc_default(IMMNotificationClient* self, EDataFlow flow, ERole role, LPCWSTR id) {
  (void)self, (void)flow, (void)role, (void)id;
  return S_OK;
}
static HRESULT STDMETHODCALLTYPE nc_property(IMMNotificationClient* self, LPCWSTR id, const PROPERTYKEY key) {
  (void)self, (void)id, (void)key;
  return S_OK;
}

static IMMNotificationClientVtbl notify_vtbl = {nc_query,   nc_addref,  nc_release, nc_state,
                                                nc_added,   nc_removed, nc_default, nc_property};
static IMMNotificationClient notify = {&notify_vtbl};

// watch_thread registers the notification client in the multithreaded
// apartment, which CoIncrementMTAUsage keeps alive for the process's life
// with the enumerator that holds the registration.
static DWORD WINAPI watch_thread(LPVOID arg) {
  (void)arg;
  CO_MTA_USAGE_COOKIE cookie;
  if (FAILED(CoIncrementMTAUsage(&cookie))) return 0;
  HRESULT hr = CoInitializeEx(NULL, COINIT_MULTITHREADED);
  IMMDeviceEnumerator* e = NULL;
  if (SUCCEEDED(CoCreateInstance(&CLSID_MMDeviceEnumerator, NULL, CLSCTX_ALL, &IID_IMMDeviceEnumerator, (void**)&e)) &&
      FAILED(IMMDeviceEnumerator_RegisterEndpointNotificationCallback(e, &notify))) {
    IMMDeviceEnumerator_Release(e);
  }
  if (SUCCEEDED(hr)) CoUninitialize();
  return 0;
}

static BOOL CALLBACK start_watch(PINIT_ONCE once, PVOID param, PVOID* ctx) {
  (void)once, (void)param, (void)ctx;
  // On a thread of its own: the caller's may be in a single-threaded
  // apartment, which would hold the registration.
  HANDLE t = CreateThread(NULL, 0, watch_thread, NULL, 0, NULL);
  if (t) {
    WaitForSingleObject(t, INFINITE);
    CloseHandle(t);
  }
  return TRUE;
}

uint64_t sona_route_epoch(void) {
  InitOnceExecuteOnce(&watch_once, start_watch, NULL, NULL);
  return atomic_load(&route_epoch);
}

int sona_endpoint_state(const char* id) {
  InitOnceExecuteOnce(&watch_once, start_watch, NULL, NULL);
  int n = MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, id, -1, NULL, 0);
  if (n <= 0) return SONA_ENDPOINT_GONE;  // not an ID any endpoint has
  WCHAR* wid = malloc(n * sizeof(WCHAR));
  if (!wid) return SONA_ENDPOINT_UNKNOWN;
  MultiByteToWideChar(CP_UTF8, 0, id, -1, wid, n);
  HRESULT init = CoInitializeEx(NULL, COINIT_MULTITHREADED);
  int res = SONA_ENDPOINT_UNKNOWN;
  IMMDeviceEnumerator* e = NULL;
  if (SUCCEEDED(CoCreateInstance(&CLSID_MMDeviceEnumerator, NULL, CLSCTX_ALL, &IID_IMMDeviceEnumerator, (void**)&e))) {
    IMMDevice* d = NULL;
    DWORD state = 0;
    res = SONA_ENDPOINT_GONE;
    if (SUCCEEDED(IMMDeviceEnumerator_GetDevice(e, wid, &d))) {
      if (SUCCEEDED(IMMDevice_GetState(d, &state)) && state == DEVICE_STATE_ACTIVE) res = SONA_ENDPOINT_ACTIVE;
      IMMDevice_Release(d);
    }
    IMMDeviceEnumerator_Release(e);
  }
  if (SUCCEEDED(init)) CoUninitialize();
  free(wid);
  return res;
}
