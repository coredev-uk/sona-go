#include "devices.h"

#include <gst/gst.h>
#include <string.h>

static char* dup_prop(const GstStructure* s, const char* key) {
  const char* v = s ? gst_structure_get_string(s, key) : NULL;
  return v ? g_strdup(v) : NULL;
}

int sona_gst_audio_sinks(sona_device** out, int* n) {
  *out = NULL;
  *n = 0;
  GstDeviceProviderFactory* f = gst_device_provider_factory_find("pipewiredeviceprovider");
  if (!f) return 0;
  GstDeviceProvider* prov = gst_device_provider_factory_get(f);
  gst_object_unref(f);
  if (!prov) return -1;
  // Without start(), get_devices probes once: connect, enumerate, disconnect.
  GList* devs = gst_device_provider_get_devices(prov);
  int cap = g_list_length(devs);
  sona_device* res = cap ? g_new0(sona_device, cap) : NULL;
  int k = 0;
  for (GList* l = devs; l; l = l->next) {
    GstDevice* d = GST_DEVICE(l->data);
    if (!gst_device_has_classes(d, "Audio/Sink")) continue;
    GstStructure* props = gst_device_get_properties(d);
    char* name = dup_prop(props, "node.name");
    if (name) {
      gboolean def = FALSE;
      if (props) gst_structure_get_boolean(props, "is-default", &def);
      res[k].name = name;
      res[k].description = gst_device_get_display_name(d);
      res[k].is_default = def;
      k++;
    }
    if (props) gst_structure_free(props);
  }
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
