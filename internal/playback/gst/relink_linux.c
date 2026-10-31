#include <string.h>

#include "gst.h"

int sona_relink_error(GstMessage* m) {
  if (GST_MESSAGE_TYPE(m) != GST_MESSAGE_ERROR) return 0;
  GstObject* src = GST_MESSAGE_SRC(m);
  if (!GST_IS_ELEMENT(src)) return 0;
  GstElementFactory* f = gst_element_get_factory(GST_ELEMENT(src));
  if (!f || g_strcmp0(GST_OBJECT_NAME(f), "pipewiresink") != 0) return 0;
  GError* e = NULL;
  gst_message_parse_error(m, &e, NULL);
  int match = e && g_error_matches(e, GST_RESOURCE_ERROR, GST_RESOURCE_ERROR_NOT_FOUND) && e->message &&
              strstr(e->message, "all buffers have been removed");
  g_clear_error(&e);
  return match;
}
