#define COBJMACROS
#include <windows.h>
#include <mmdeviceapi.h>
#include <propidl.h>
#include <stdlib.h>
#include <string.h>

#include "devices.h"

// The listing runs in the UI process, which must not load GStreamer's
// plugins: a plugin DLL loaded from the install's lib\ keeps that folder from
// being renamed, which an update and a restore must do (internal/update). So
// it asks Core Audio directly for what wasapi2's device provider would list:
// the active render endpoints, each with its endpoint ID (wasapi2's device.id,
// which wasapi2sink takes as its device) and friendly name, and the default
// console endpoint, which is the one wasapi2's virtual "default device" entry
// stands for (device.actual-id).

static const CLSID clsid_enumerator = {0xBCDE0395, 0xE52F, 0x467C, {0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}};
static const IID iid_enumerator = {0xA95664D2, 0x9614, 0x4F35, {0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}};
// PKEY_Device_FriendlyName.
static const PROPERTYKEY key_friendly_name = {{0xA45C254E, 0xDF1C, 0x4EFD, {0x80, 0x20, 0x67, 0xD1, 0x46, 0xA8, 0x50, 0xE0}}, 14};

// utf8 converts w to a malloc'd UTF-8 string; NULL for NULL or on failure.
static char* utf8(const wchar_t* w) {
  if (!w) return NULL;
  int n = WideCharToMultiByte(CP_UTF8, 0, w, -1, NULL, 0, NULL, NULL);
  if (n <= 0) return NULL;
  char* s = malloc(n);
  if (s && WideCharToMultiByte(CP_UTF8, 0, w, -1, s, n, NULL, NULL) != n) {
    free(s);
    s = NULL;
  }
  return s;
}

static char* friendly_name(IMMDevice* d) {
  IPropertyStore* props = NULL;
  if (FAILED(IMMDevice_OpenPropertyStore(d, STGM_READ, &props))) return NULL;
  PROPVARIANT v;
  PropVariantInit(&v);
  char* name = NULL;
  if (SUCCEEDED(IPropertyStore_GetValue(props, &key_friendly_name, &v)) && v.vt == VT_LPWSTR) name = utf8(v.pwszVal);
  PropVariantClear(&v);
  IPropertyStore_Release(props);
  return name;
}

static int list(IMMDeviceEnumerator* en, sona_device** out, int* n) {
  char* def = NULL;
  IMMDevice* d = NULL;
  // No default (no endpoint at all) is not a failure.
  if (SUCCEEDED(IMMDeviceEnumerator_GetDefaultAudioEndpoint(en, eRender, eConsole, &d))) {
    LPWSTR id = NULL;
    if (SUCCEEDED(IMMDevice_GetId(d, &id))) {
      def = utf8(id);
      CoTaskMemFree(id);
    }
    IMMDevice_Release(d);
  }
  IMMDeviceCollection* all = NULL;
  if (FAILED(IMMDeviceEnumerator_EnumAudioEndpoints(en, eRender, DEVICE_STATE_ACTIVE, &all))) {
    free(def);
    return -1;
  }
  UINT count = 0;
  IMMDeviceCollection_GetCount(all, &count);
  sona_device* res = count ? calloc(count, sizeof(sona_device)) : NULL;
  int k = 0;
  for (UINT i = 0; res && i < count; i++) {
    if (FAILED(IMMDeviceCollection_Item(all, i, &d))) continue;
    LPWSTR wid = NULL;
    char* id = NULL;
    if (SUCCEEDED(IMMDevice_GetId(d, &wid))) {
      id = utf8(wid);
      CoTaskMemFree(wid);
    }
    if (id) {
      res[k].name = id;
      res[k].description = friendly_name(d);
      if (!res[k].description) res[k].description = _strdup(id);
      res[k].is_default = def && strcmp(def, id) == 0;
      k++;
    }
    IMMDevice_Release(d);
  }
  IMMDeviceCollection_Release(all);
  free(def);
  *out = res;
  *n = k;
  return 0;
}

// sona_gst_audio_sinks lists the active WASAPI render endpoints (see the top
// of this file). The calling thread's COM apartment is joined, or used as it
// is when the thread already has one.
int sona_gst_audio_sinks(sona_device** out, int* n) {
  *out = NULL;
  *n = 0;
  HRESULT init = CoInitializeEx(NULL, COINIT_MULTITHREADED);
  if (FAILED(init) && init != RPC_E_CHANGED_MODE) return -1;
  IMMDeviceEnumerator* en = NULL;
  int rc = -1;
  if (SUCCEEDED(CoCreateInstance(&clsid_enumerator, NULL, CLSCTX_ALL, &iid_enumerator, (void**)&en))) {
    rc = list(en, out, n);
    IMMDeviceEnumerator_Release(en);
  }
  if (SUCCEEDED(init)) CoUninitialize();
  return rc;
}

void sona_gst_free_devices(sona_device* d, int n) {
  for (int i = 0; i < n; i++) {
    free(d[i].name);
    free(d[i].description);
  }
  free(d);
}
