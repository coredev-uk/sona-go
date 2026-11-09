#include <gst/gst.h>
#include <string.h>

#include "devices.h"

static char* dup_prop(const GstStructure* s, const char* key) {
  const char* v = s ? gst_structure_get_string(s, key) : NULL;
  return v ? g_strdup(v) : NULL;
}

// sona_gst_audio_sinks lists the WASAPI render endpoints through the wasapi2
// plugin's device provider. It also offers a virtual "default device" entry
// (device.default, whose device.actual-id names the endpoint it stands for):
// that entry is not listed, as the empty device already means the default,
// and the endpoint it stands for is the one marked is_default.
int sona_gst_audio_sinks(sona_device** out, int* n) {
  *out = NULL;
  *n = 0;
  GstDeviceProviderFactory* f = gst_device_provider_factory_find("wasapi2deviceprovider");
  if (!f) return 0;
  GstDeviceProvider* prov = gst_device_provider_factory_get(f);
  gst_object_unref(f);
  if (!prov) return -1;
  // Without start(), get_devices probes once.
  GList* devs = gst_device_provider_get_devices(prov);
  char* actual = NULL;
  for (GList* l = devs; l; l = l->next) {
    GstDevice* d = GST_DEVICE(l->data);
    if (!gst_device_has_classes(d, "Audio/Sink")) continue;
    GstStructure* props = gst_device_get_properties(d);
    gboolean def = FALSE;
    if (props) gst_structure_get_boolean(props, "device.default", &def);
    if (def && !actual) actual = dup_prop(props, "device.actual-id");
    if (props) gst_structure_free(props);
  }
  int cap = g_list_length(devs);
  sona_device* res = cap ? g_new0(sona_device, cap) : NULL;
  int k = 0;
  for (GList* l = devs; l; l = l->next) {
    GstDevice* d = GST_DEVICE(l->data);
    if (!gst_device_has_classes(d, "Audio/Sink")) continue;
    GstStructure* props = gst_device_get_properties(d);
    gboolean def = FALSE;
    if (props) gst_structure_get_boolean(props, "device.default", &def);
    char* id = def ? NULL : dup_prop(props, "device.id");
    if (id) {
      res[k].name = id;
      res[k].description = gst_device_get_display_name(d);
      res[k].is_default = actual && strcmp(actual, id) == 0;
      k++;
    }
    if (props) gst_structure_free(props);
  }
  g_free(actual);
  g_list_free_full(devs, gst_object_unref);
  gst_object_unref(prov);
  *out = res;
  *n = k;
  return 0;
}

void sona_gst_free_devices(sona_device* d, int n) {
  for (int i = 0; i < n; i++) {
    g_free(d[i].name);
    g_free(d[i].description);
  }
  g_free(d);
}
