#include "gst.h"

#include <gst/app/gstappsrc.h>
#include <gst/gst.h>
#include <stdio.h>
#include <string.h>

struct sona_gst {
  GstElement* pipeline;
  GstElement* src;
  GstElement* volume;
  GstBus* bus;
  GMutex mu;
  uint64_t gen;  // atomic; also written under mu
  gboolean need_segment;
  GstSegment segment;
  GstCaps* caps;
};

int sona_gst_init(char* err, size_t errlen) {
  GError* e = NULL;
  if (!gst_init_check(NULL, NULL, &e)) {
    snprintf(err, errlen, "%s", e ? e->message : "gst_init failed");
    g_clear_error(&e);
    return -1;
  }
  return 0;
}

char* sona_gst_version(void) { return gst_version_string(); }

int sona_gst_have(const char* factory) {
  GstElementFactory* f = gst_element_factory_find(factory);
  if (!f) return 0;
  gst_object_unref(f);
  return 1;
}

static gboolean has_prop(gpointer obj, const char* name) {
  return g_object_class_find_property(G_OBJECT_GET_CLASS(obj), name) != NULL;
}

// drop_stale discards buffers pushed for a generation a flush superseded.
static GstPadProbeReturn drop_stale(GstPad* pad, GstPadProbeInfo* info, gpointer user) {
  sona_gst* p = user;
  GstBuffer* b = GST_PAD_PROBE_INFO_BUFFER(info);
  if (b && GST_BUFFER_OFFSET(b) != __atomic_load_n(&p->gen, __ATOMIC_ACQUIRE)) return GST_PAD_PROBE_DROP;
  return GST_PAD_PROBE_OK;
}

sona_gst* sona_gst_new(const char* decoder, const char* sink, const char* client_name, const char* props,
                       uint64_t max_queue_ns, char* err, size_t errlen) {
  const char* names[] = {"appsrc", decoder, "audioconvert", "audioresample", "volume", sink};
  GstElement* el[6] = {0};
  for (int i = 0; i < 6; i++) {
    el[i] = gst_element_factory_make(names[i], NULL);
    if (!el[i]) {
      snprintf(err, errlen, "GStreamer element %s is not installed", names[i]);
      for (int j = 0; j < i; j++) gst_object_unref(el[j]);
      return NULL;
    }
  }
  GstElement* pipe = gst_pipeline_new("sona");
  g_object_set(el[0], "format", GST_FORMAT_TIME, "stream-type", GST_APP_STREAM_TYPE_STREAM, "is-live", FALSE,
               "block", TRUE, "max-bytes", (guint64)0, "max-buffers", (guint64)0, "max-time", (guint64)max_queue_ns,
               "handle-segment-change", TRUE, "emit-signals", FALSE, NULL);
  GstElement* snk = el[5];
  if (has_prop(snk, "sync")) g_object_set(snk, "sync", TRUE, NULL);
  if (client_name && has_prop(snk, "client-name")) g_object_set(snk, "client-name", client_name, NULL);
  if (props && *props && has_prop(snk, "stream-properties")) {
    GstStructure* s = gst_structure_from_string(props, NULL);
    if (s) {
      g_object_set(snk, "stream-properties", s, NULL);
      gst_structure_free(s);
    }
  }
  gst_bin_add_many(GST_BIN(pipe), el[0], el[1], el[2], el[3], el[4], el[5], NULL);
  if (!gst_element_link_many(el[0], el[1], el[2], el[3], el[4], el[5], NULL)) {
    snprintf(err, errlen, "cannot link appsrc ! %s ! audioconvert ! audioresample ! volume ! %s", decoder, sink);
    gst_object_unref(pipe);
    return NULL;
  }
  sona_gst* p = g_new0(sona_gst, 1);
  p->pipeline = pipe;
  p->src = el[0];
  p->volume = el[4];
  p->bus = gst_element_get_bus(pipe);
  g_mutex_init(&p->mu);
  gst_segment_init(&p->segment, GST_FORMAT_TIME);
  GstPad* pad = gst_element_get_static_pad(p->src, "src");
  gst_pad_add_probe(pad, GST_PAD_PROBE_TYPE_BUFFER, drop_stale, p, NULL);
  gst_object_unref(pad);
  return p;
}

void sona_gst_free_pipeline(sona_gst* p) {
  if (!p) return;
  gst_element_set_state(p->pipeline, GST_STATE_NULL);
  gst_object_unref(p->bus);
  gst_object_unref(p->pipeline);
  if (p->caps) gst_caps_unref(p->caps);
  g_mutex_clear(&p->mu);
  g_free(p);
}

int sona_gst_set_state(sona_gst* p, int state) { return gst_element_set_state(p->pipeline, (GstState)state); }

int sona_gst_get_state(sona_gst* p, int64_t timeout_ns, int* state, int* pending) {
  GstState s = GST_STATE_VOID_PENDING, pend = GST_STATE_VOID_PENDING;
  GstStateChangeReturn r = gst_element_get_state(p->pipeline, &s, &pend, (GstClockTime)timeout_ns);
  *state = s;
  *pending = pend;
  return r;
}

void sona_gst_flush(sona_gst* p, uint64_t gen, int64_t start_ns, const uint8_t* asc, size_t asc_len, int rate,
                    int channels) {
  g_mutex_lock(&p->mu);
  __atomic_store_n(&p->gen, gen, __ATOMIC_RELEASE);
  gst_segment_init(&p->segment, GST_FORMAT_TIME);
  p->segment.start = start_ns;
  p->segment.time = start_ns;
  p->segment.position = start_ns;
  if (asc && asc_len) {
    GstBuffer* cd = gst_buffer_new_memdup(asc, asc_len);
    GstCaps* caps = gst_caps_new_simple("audio/mpeg", "mpegversion", G_TYPE_INT, 4, "stream-format", G_TYPE_STRING,
                                        "raw", "framed", G_TYPE_BOOLEAN, TRUE, "codec_data", GST_TYPE_BUFFER, cd, NULL);
    if (rate > 0) gst_caps_set_simple(caps, "rate", G_TYPE_INT, rate, NULL);
    if (channels > 0) gst_caps_set_simple(caps, "channels", G_TYPE_INT, channels, NULL);
    gst_buffer_unref(cd);
    if (p->caps) gst_caps_unref(p->caps);
    p->caps = caps;
  }
  p->need_segment = TRUE;
  g_mutex_unlock(&p->mu);
  // Unblocks a push waiting for queue space and clears queued data and EOS.
  gst_element_send_event(p->src, gst_event_new_flush_start());
  gst_element_send_event(p->src, gst_event_new_flush_stop(TRUE));
}

static int flow(GstFlowReturn r) {
  switch (r) {
    case GST_FLOW_OK:
      return SONA_PUSH_OK;
    case GST_FLOW_FLUSHING:
      return SONA_PUSH_FLUSHING;
    case GST_FLOW_EOS:
      return SONA_PUSH_EOS;
    default:
      return SONA_PUSH_ERROR;
  }
}

int sona_gst_push(sona_gst* p, uint64_t gen, uint8_t* data, size_t size, int64_t pts_ns, int64_t dur_ns) {
  GstBuffer* b = gst_buffer_new_wrapped(data, size);
  GST_BUFFER_PTS(b) = pts_ns;
  GST_BUFFER_DURATION(b) = dur_ns;
  GST_BUFFER_OFFSET(b) = gen;
  GstSample* sample = NULL;
  g_mutex_lock(&p->mu);
  if (gen != p->gen) {
    g_mutex_unlock(&p->mu);
    gst_buffer_unref(b);
    return SONA_PUSH_STALE;
  }
  if (p->need_segment) {
    sample = gst_sample_new(b, p->caps, &p->segment, NULL);
    p->need_segment = FALSE;
  }
  g_mutex_unlock(&p->mu);
  if (sample) {
    GstFlowReturn r = gst_app_src_push_sample(GST_APP_SRC(p->src), sample);
    gst_sample_unref(sample);
    gst_buffer_unref(b);
    return flow(r);
  }
  return flow(gst_app_src_push_buffer(GST_APP_SRC(p->src), b));
}

int sona_gst_end_of_stream(sona_gst* p, uint64_t gen) {
  g_mutex_lock(&p->mu);
  int r = SONA_PUSH_STALE;
  if (gen == p->gen) r = flow(gst_app_src_end_of_stream(GST_APP_SRC(p->src)));
  g_mutex_unlock(&p->mu);
  return r;
}

uint8_t* sona_gst_alloc(size_t size) { return g_malloc(size ? size : 1); }
void sona_gst_free(void* data) { g_free(data); }

void sona_gst_set_volume(sona_gst* p, double v) { g_object_set(p->volume, "volume", v, NULL); }

int sona_gst_position(sona_gst* p, int64_t* pos_ns) {
  gint64 pos = -1;
  if (!gst_element_query_position(p->pipeline, GST_FORMAT_TIME, &pos) || pos < 0) return 0;
  *pos_ns = pos;
  return 1;
}

uint64_t sona_gst_queued_ns(sona_gst* p) { return gst_app_src_get_current_level_time(GST_APP_SRC(p->src)); }

int sona_gst_pop(sona_gst* p, int64_t timeout_ns, sona_msg* out) {
  memset(out, 0, sizeof(*out));
  GstMessage* m = gst_bus_timed_pop_filtered(
      p->bus, (GstClockTime)timeout_ns,
      GST_MESSAGE_EOS | GST_MESSAGE_ERROR | GST_MESSAGE_WARNING | GST_MESSAGE_STATE_CHANGED | GST_MESSAGE_ASYNC_DONE |
          GST_MESSAGE_CLOCK_LOST | GST_MESSAGE_LATENCY | GST_MESSAGE_APPLICATION);
  if (!m) return SONA_MSG_NONE;
  GError* e = NULL;
  gchar* dbg = NULL;
  switch (GST_MESSAGE_TYPE(m)) {
    case GST_MESSAGE_EOS:
      out->type = SONA_MSG_EOS;
      break;
    case GST_MESSAGE_ERROR:
    case GST_MESSAGE_WARNING:
      if (GST_MESSAGE_TYPE(m) == GST_MESSAGE_ERROR) {
        gst_message_parse_error(m, &e, &dbg);
        out->type = SONA_MSG_ERROR;
      } else {
        gst_message_parse_warning(m, &e, &dbg);
        out->type = SONA_MSG_WARNING;
      }
      out->text = g_strdup_printf("%s: %s", GST_OBJECT_NAME(GST_MESSAGE_SRC(m)), e ? e->message : "unknown");
      out->debug = dbg;
      g_clear_error(&e);
      break;
    case GST_MESSAGE_STATE_CHANGED:
      if (GST_MESSAGE_SRC(m) == GST_OBJECT(p->pipeline)) {
        GstState o, n, pend;
        gst_message_parse_state_changed(m, &o, &n, &pend);
        out->type = SONA_MSG_STATE;
        out->a = o;
        out->b = n;
        out->c = pend;
      }
      break;
    case GST_MESSAGE_ASYNC_DONE:
      out->type = SONA_MSG_ASYNC_DONE;
      break;
    case GST_MESSAGE_CLOCK_LOST: {
      // Standard recovery: re-select a clock by going through PAUSED.
      GstState cur, pend;
      gst_element_get_state(p->pipeline, &cur, &pend, 0);
      if (cur == GST_STATE_PLAYING) {
        gst_element_set_state(p->pipeline, GST_STATE_PAUSED);
        gst_element_set_state(p->pipeline, GST_STATE_PLAYING);
      }
      out->type = SONA_MSG_CLOCK_LOST;
      break;
    }
    case GST_MESSAGE_LATENCY:
      gst_bin_recalculate_latency(GST_BIN(p->pipeline));
      break;
    default:
      break;
  }
  gst_message_unref(m);
  return out->type;
}

void sona_gst_wake(sona_gst* p) {
  gst_bus_post(p->bus, gst_message_new_application(NULL, gst_structure_new_empty("sona-wake")));
}
