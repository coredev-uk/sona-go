#include "gst.h"

// Core Audio has no sound server that relinks streams: no error is one.
int sona_relink_error(GstMessage* m) {
  (void)m;
  return 0;
}
