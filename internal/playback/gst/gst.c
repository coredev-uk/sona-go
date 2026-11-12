#include "gst.h"

#include <gst/app/gstappsink.h>
#include <gst/app/gstappsrc.h>
#include <stdio.h>
#include <string.h>

#include "tail.h"

// Frames per output buffer from the pool (the mixer may push fewer).
#define OUT_POOL_FRAMES 1024
// The output queue: just enough to ride out scheduling hiccups, short so
// equaliser and volume changes are heard at once.
#define OUT_QUEUE_NS (60 * GST_MSECOND)
// Decoded audio a lane holds ahead of the mixer.
#define LANE_SINK_BUFFERS 8

#define PCM_CAPS "audio/x-raw,format=F32LE,layout=interleaved,rate=44100,channels=2,channel-mask=(bitmask)0x3"

struct sona_gst {
  GAsyncQueue* msgs;  // sona_msg*
  sona_tail* tail;
  GstBufferPool* pool;
  GstCaps* pcm_caps;
  char* sink;
  char* client;

  // The output, replaced by rebuilds. out_mu guards these fields; the
  // elements stay valid while referenced.
  GMutex out_mu;
  char* props;
  GstElement* out_pipe;
  GstElement* out_src;
  uint64_t build;
  uint64_t epoch;  // atomic; written under out_mu
  gboolean need_segment;
  int64_t seg_start;  // where the next segment starts
  int64_t next_pts;   // end of the last pushed buffer
};

struct sona_lane {
  sona_gst* owner;
  int index;
  GstElement* pipe;
  GstElement* src;
  GstElement* sink;
  GMutex mu;
  uint64_t gen;      // atomic; also written under mu
  uint64_t out_gen;  // atomic: stamped on decoded buffers
  uint64_t eos_gen;  // atomic: the generation whose EOS reached the sink
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

uint8_t* sona_gst_alloc(size_t size) { return g_malloc(size ? size : 1); }
void sona_gst_free(void* data) { g_free(data); }

static gboolean has_prop(gpointer obj, const char* name) {
  return g_object_class_find_property(G_OBJECT_GET_CLASS(obj), name) != NULL;
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

// Messages ------------------------------------------------------------------

typedef struct {
  sona_gst* p;
  int lane;
  uint64_t build;
} bus_ctx;

static void post(sona_gst* p, sona_msg* m) { g_async_queue_push(p->msgs, m); }

// on_bus turns the messages Sona handles into sona_msgs on the shared
// queue, in the posting thread.
static GstBusSyncReply on_bus(GstBus* bus, GstMessage* m, gpointer user) {
  bus_ctx* c = user;
  sona_msg* out = g_new0(sona_msg, 1);
  out->lane = c->lane;
  out->build = c->build;
  GError* e = NULL;
  gchar* dbg = NULL;
  switch (GST_MESSAGE_TYPE(m)) {
    case GST_MESSAGE_EOS:
      out->type = SONA_MSG_EOS;
      break;
    case GST_MESSAGE_ERROR:
      if (sona_relink_error(m)) {
        out->type = SONA_MSG_RELINK;
        break;
      }
      // fallthrough
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
      if (c->lane < 0 && GST_IS_PIPELINE(GST_MESSAGE_SRC(m))) {
        GstState o, n, pend;
        gst_message_parse_state_changed(m, &o, &n, &pend);
        out->type = SONA_MSG_STATE;
        out->a = o;
        out->b = n;
        out->c = pend;
      }
      break;
    case GST_MESSAGE_ASYNC_DONE:
      if (c->lane < 0) out->type = SONA_MSG_ASYNC_DONE;
      break;
    case GST_MESSAGE_CLOCK_LOST:
      if (c->lane < 0) out->type = SONA_MSG_CLOCK_LOST;
      break;
    case GST_MESSAGE_LATENCY:
      if (c->lane < 0) out->type = SONA_MSG_LATENCY;
      break;
    default:
      break;
  }
  if (out->type == SONA_MSG_NONE)
    g_free(out);
  else
    post(c->p, out);
  return GST_BUS_DROP;
}

static void watch_bus(sona_gst* p, GstElement* pipe, int lane, uint64_t build) {
  bus_ctx* c = g_new0(bus_ctx, 1);
  c->p = p;
  c->lane = lane;
  c->build = build;
  GstBus* bus = gst_element_get_bus(pipe);
  gst_bus_set_sync_handler(bus, on_bus, c, g_free);
  gst_object_unref(bus);
}

int sona_gst_pop(sona_gst* p, int64_t timeout_ns, sona_msg* out) {
  memset(out, 0, sizeof(*out));
  out->lane = -1;  // a timeout is the output's (it runs the stall check), not lane 0's
  sona_msg* m = g_async_queue_timeout_pop(p->msgs, timeout_ns / 1000);
  if (!m) return SONA_MSG_NONE;
  *out = *m;
  g_free(m);
  return out->type;
}

void sona_gst_wake(sona_gst* p) {
  sona_msg* m = g_new0(sona_msg, 1);
  m->type = SONA_MSG_WAKE;
  m->lane = -1;
  post(p, m);
}

// Output --------------------------------------------------------------------

// drop_stale_out discards buffers pushed for an epoch a flush superseded.
static GstPadProbeReturn drop_stale_out(GstPad* pad, GstPadProbeInfo* info, gpointer user) {
  sona_gst* p = user;
  GstBuffer* b = GST_PAD_PROBE_INFO_BUFFER(info);
  if (b && GST_BUFFER_OFFSET(b) != __atomic_load_n(&p->epoch, __ATOMIC_ACQUIRE)) return GST_PAD_PROBE_DROP;
  return GST_PAD_PROBE_OK;
}

// build_output makes appsrc ! tail ! sink in NULL state.
static GstElement* build_output(sona_gst* p, uint64_t build, GstElement** src_out, char* err, size_t errlen) {
  GstElement* src = gst_element_factory_make("appsrc", NULL);
  GstElement* snk = gst_element_factory_make(p->sink, NULL);
  if (!src || !snk) {
    snprintf(err, errlen, "GStreamer element %s is not installed", src ? p->sink : "appsrc");
    if (src) gst_object_unref(src);
    if (snk) gst_object_unref(snk);
    return NULL;
  }
  GstElement* tail = sona_tail_build(p->tail, err, errlen);
  if (!tail) {
    gst_object_unref(src);
    gst_object_unref(snk);
    return NULL;
  }
  g_object_set(src, "format", GST_FORMAT_TIME, "stream-type", GST_APP_STREAM_TYPE_STREAM, "is-live", FALSE, "block",
               TRUE, "max-bytes", (guint64)0, "max-buffers", (guint64)0, "max-time", (guint64)OUT_QUEUE_NS,
               "handle-segment-change", TRUE, "emit-signals", FALSE, "caps", p->pcm_caps, NULL);
  if (has_prop(snk, "sync")) g_object_set(snk, "sync", TRUE, NULL);
  if (p->client && has_prop(snk, "client-name")) g_object_set(snk, "client-name", p->client, NULL);
  if (p->props && *p->props && has_prop(snk, "stream-properties")) {
    GstStructure* s = gst_structure_from_string(p->props, NULL);
    if (s) {
      g_object_set(snk, "stream-properties", s, NULL);
      gst_structure_free(s);
    }
  } else if (p->props && *p->props && has_prop(snk, "unique-id")) {
    g_object_set(snk, "unique-id", p->props, NULL);  // osxaudiosink: the Core Audio device UID
  } else if (p->props && *p->props && has_prop(snk, "device")) {
    g_object_set(snk, "device", p->props, NULL);  // wasapi2sink, wasapisink: the endpoint's device ID
  }
  GstElement* pipe = gst_pipeline_new("sona-output");
  gst_bin_add_many(GST_BIN(pipe), src, tail, snk, NULL);
  if (!gst_element_link_many(src, tail, snk, NULL)) {
    snprintf(err, errlen, "cannot link appsrc ! tail ! %s", p->sink);
    gst_object_unref(pipe);
    return NULL;
  }
  GstPad* pad = gst_element_get_static_pad(src, "src");
  gst_pad_add_probe(pad, GST_PAD_PROBE_TYPE_BUFFER, drop_stale_out, p, NULL);
  gst_object_unref(pad);
  watch_bus(p, pipe, -1, build);
  *src_out = src;
  return pipe;
}

static void teardown(GstElement* pipe) {
  if (!pipe) return;
  gst_element_set_state(pipe, GST_STATE_NULL);
  gst_object_unref(pipe);
}

static gpointer teardown_thread(gpointer pipe) {
  teardown(pipe);
  return NULL;
}

sona_gst* sona_gst_new(const char* sink, const char* client_name, const char* props, char* err, size_t errlen) {
  sona_gst* p = g_new0(sona_gst, 1);
  p->msgs = g_async_queue_new_full(g_free);
  p->tail = sona_tail_create();
  p->sink = g_strdup(sink);
  p->client = g_strdup(client_name);
  p->props = g_strdup(props);
  p->pcm_caps = gst_caps_from_string(PCM_CAPS);
  g_mutex_init(&p->out_mu);
  p->pool = gst_buffer_pool_new();
  GstStructure* cfg = gst_buffer_pool_get_config(p->pool);
  gst_buffer_pool_config_set_params(cfg, p->pcm_caps, OUT_POOL_FRAMES * SONA_CHANNELS * sizeof(float), 4, 0);
  gst_buffer_pool_set_config(p->pool, cfg);
  gst_buffer_pool_set_active(p->pool, TRUE);
  if (!sona_gst_out_rebuild(p, NULL, GST_STATE_READY, err, errlen)) {
    sona_gst_free_pipeline(p);
    return NULL;
  }
  return p;
}

void sona_gst_free_pipeline(sona_gst* p) {
  if (!p) return;
  teardown(p->out_pipe);
  if (p->out_src) gst_object_unref(p->out_src);
  gst_buffer_pool_set_active(p->pool, FALSE);
  gst_object_unref(p->pool);
  gst_caps_unref(p->pcm_caps);
  g_async_queue_unref(p->msgs);
  sona_tail_destroy(p->tail);
  g_mutex_clear(&p->out_mu);
  g_free(p->sink);
  g_free(p->client);
  g_free(p->props);
  g_free(p);
}

struct sona_tail* sona_gst_tail(sona_gst* p) { return p->tail; }

uint64_t sona_gst_out_rebuild(sona_gst* p, const char* props, int state, char* err, size_t errlen) {
  g_mutex_lock(&p->out_mu);
  if (props) {
    g_free(p->props);
    p->props = g_strdup(props);
  }
  uint64_t build = p->build + 1;
  GstElement* src = NULL;
  GstElement* pipe = build_output(p, build, &src, err, errlen);
  if (!pipe) {
    g_mutex_unlock(&p->out_mu);
    return 0;
  }
  GstElement* old = p->out_pipe;
  GstElement* old_src = p->out_src;
  p->out_pipe = pipe;
  p->out_src = gst_object_ref(src);
  p->build = build;
  // Continue the timeline where the old output stopped.
  p->need_segment = TRUE;
  p->seg_start = p->next_pts;
  if (old_src) {
    // Unblocks a push into the old output.
    gst_element_send_event(old_src, gst_event_new_flush_start());
  }
  g_mutex_unlock(&p->out_mu);
  if (old_src) gst_object_unref(old_src);
  GstStateChangeReturn r = gst_element_set_state(pipe, (GstState)state);
  // The old sink may take long to close (its device is gone): playback
  // goes on in the new output meanwhile.
  if (old) g_thread_unref(g_thread_new("sona-output-teardown", teardown_thread, old));
  if (r == GST_STATE_CHANGE_FAILURE) {
    snprintf(err, errlen, "%s: cannot start the output", p->sink);
    return 0;
  }
  return build;
}

static GstElement* out_ref(sona_gst* p) {
  g_mutex_lock(&p->out_mu);
  GstElement* e = p->out_pipe ? gst_object_ref(p->out_pipe) : NULL;
  g_mutex_unlock(&p->out_mu);
  return e;
}

int sona_gst_set_state(sona_gst* p, int state) {
  GstElement* pipe = out_ref(p);
  if (!pipe) return GST_STATE_CHANGE_FAILURE;
  int r = gst_element_set_state(pipe, (GstState)state);
  gst_object_unref(pipe);
  return r;
}

int sona_gst_position(sona_gst* p, int64_t* pos_ns) {
  GstElement* pipe = out_ref(p);
  if (!pipe) return 0;
  gint64 pos = -1;
  gboolean ok = gst_element_query_position(pipe, GST_FORMAT_TIME, &pos);
  gst_object_unref(pipe);
  if (!ok || pos < 0) return 0;
  *pos_ns = pos;
  return 1;
}

uint64_t sona_gst_out_queued_ns(sona_gst* p) {
  g_mutex_lock(&p->out_mu);
  uint64_t q = p->out_src ? gst_app_src_get_current_level_time(GST_APP_SRC(p->out_src)) : 0;
  g_mutex_unlock(&p->out_mu);
  return q;
}

void sona_gst_out_flush(sona_gst* p, uint64_t epoch) {
  g_mutex_lock(&p->out_mu);
  __atomic_store_n(&p->epoch, epoch, __ATOMIC_RELEASE);
  p->need_segment = TRUE;
  p->seg_start = 0;
  p->next_pts = 0;
  GstElement* pipe = p->out_pipe ? gst_object_ref(p->out_pipe) : NULL;
  GstElement* src = p->out_src ? gst_object_ref(p->out_src) : NULL;
  g_mutex_unlock(&p->out_mu);
  if (!pipe) return;
  // The flush makes the sink lose its state: it drops to PAUSED and the
  // pipeline brings it back to PLAYING when it has prerolled again. The
  // pipeline ignores that loss while it is changing state itself, so a
  // flush landing while it sets its elements to PLAYING (just after a
  // preroll) would leave the sink paused under a PLAYING pipeline for good.
  // Holding the pipeline's state lock, as an event sent to the pipeline
  // would, keeps the flush out of its state changes.
  GST_STATE_LOCK(pipe);
  gst_element_send_event(src, gst_event_new_flush_start());
  gst_element_send_event(src, gst_event_new_flush_stop(TRUE));
  GST_STATE_UNLOCK(pipe);
  gst_object_unref(src);
  gst_object_unref(pipe);
}

void sona_gst_out_alloc(sona_gst* p, size_t frames, sona_pcm* out) {
  memset(out, 0, sizeof(*out));
  GstBuffer* b = NULL;
  size_t size = frames * SONA_CHANNELS * sizeof(float);
  if (frames > OUT_POOL_FRAMES || gst_buffer_pool_acquire_buffer(p->pool, &b, NULL) != GST_FLOW_OK)
    b = gst_buffer_new_allocate(NULL, size, NULL);
  gst_buffer_map(b, &out->map, GST_MAP_WRITE);
  out->buf = b;
  out->data = (float*)out->map.data;
  out->frames = frames;
}

int sona_gst_out_push(sona_gst* p, uint64_t epoch, sona_pcm* pcm, size_t frames, int64_t pts_ns) {
  GstBuffer* b = pcm->buf;
  gst_buffer_unmap(b, &pcm->map);
  pcm->buf = NULL;
  gst_buffer_set_size(b, frames * SONA_CHANNELS * sizeof(float));
  int64_t dur = gst_util_uint64_scale_int(frames, GST_SECOND, SONA_RATE);
  GST_BUFFER_PTS(b) = pts_ns;
  GST_BUFFER_DURATION(b) = dur;
  GST_BUFFER_OFFSET(b) = epoch;
  g_mutex_lock(&p->out_mu);
  if (epoch != p->epoch) {
    g_mutex_unlock(&p->out_mu);
    gst_buffer_unref(b);
    return SONA_PUSH_STALE;
  }
  if (!p->out_src) {
    g_mutex_unlock(&p->out_mu);
    gst_buffer_unref(b);
    return SONA_PUSH_NO_OUTPUT;
  }
  GstElement* src = gst_object_ref(p->out_src);
  GstSample* sample = NULL;
  if (p->need_segment) {
    GstSegment seg;
    gst_segment_init(&seg, GST_FORMAT_TIME);
    seg.start = seg.time = seg.position = p->seg_start;
    sample = gst_sample_new(b, p->pcm_caps, &seg, NULL);
    p->need_segment = FALSE;
  }
  p->next_pts = pts_ns + dur;
  g_mutex_unlock(&p->out_mu);
  GstFlowReturn r;
  if (sample) {
    r = gst_app_src_push_sample(GST_APP_SRC(src), sample);
    gst_sample_unref(sample);
    gst_buffer_unref(b);
  } else {
    r = gst_app_src_push_buffer(GST_APP_SRC(src), b);
  }
  gst_object_unref(src);
  return flow(r);
}

int sona_gst_out_eos(sona_gst* p, uint64_t epoch) {
  g_mutex_lock(&p->out_mu);
  int r = SONA_PUSH_STALE;
  if (epoch == p->epoch && p->out_src) r = flow(gst_app_src_end_of_stream(GST_APP_SRC(p->out_src)));
  g_mutex_unlock(&p->out_mu);
  return r;
}

void sona_gst_out_recalc_latency(sona_gst* p) {
  GstElement* pipe = out_ref(p);
  if (!pipe) return;
  gst_bin_recalculate_latency(GST_BIN(pipe));
  gst_object_unref(pipe);
}

void sona_gst_out_clock_lost(sona_gst* p) {
  GstElement* pipe = out_ref(p);
  if (!pipe) return;
  // Standard recovery: re-select a clock by going through PAUSED.
  GstState cur, pend;
  gst_element_get_state(pipe, &cur, &pend, 0);
  if (cur == GST_STATE_PLAYING) {
    gst_element_set_state(pipe, GST_STATE_PAUSED);
    gst_element_set_state(pipe, GST_STATE_PLAYING);
  }
  gst_object_unref(pipe);
}

// Lanes ---------------------------------------------------------------------

// drop_stale discards access units pushed for a generation a flush
// superseded.
static GstPadProbeReturn drop_stale(GstPad* pad, GstPadProbeInfo* info, gpointer user) {
  sona_lane* l = user;
  GstBuffer* b = GST_PAD_PROBE_INFO_BUFFER(info);
  if (b && GST_BUFFER_OFFSET(b) != __atomic_load_n(&l->gen, __ATOMIC_ACQUIRE)) return GST_PAD_PROBE_DROP;
  return GST_PAD_PROBE_OK;
}

// stamp marks decoded buffers (and EOS) with the generation that produced
// them, so the mixer can tell a buffer pulled just before a flush.
static GstPadProbeReturn stamp(GstPad* pad, GstPadProbeInfo* info, gpointer user) {
  sona_lane* l = user;
  uint64_t g = __atomic_load_n(&l->out_gen, __ATOMIC_ACQUIRE);
  if (info->type & GST_PAD_PROBE_TYPE_BUFFER) {
    GstBuffer* b = GST_PAD_PROBE_INFO_BUFFER(info);
    b = gst_buffer_make_writable(b);
    GST_BUFFER_OFFSET(b) = g;
    GST_PAD_PROBE_INFO_DATA(info) = b;
  } else if (GST_EVENT_TYPE(GST_PAD_PROBE_INFO_EVENT(info)) == GST_EVENT_EOS) {
    __atomic_store_n(&l->eos_gen, g, __ATOMIC_RELEASE);
  }
  return GST_PAD_PROBE_OK;
}

sona_lane* sona_lane_new(sona_gst* p, int index, const char* decoder, uint64_t max_queue_ns, char* err,
                         size_t errlen) {
  const char* names[] = {"appsrc", decoder, "audioconvert", "audioresample", "appsink"};
  GstElement* el[5] = {0};
  for (int i = 0; i < 5; i++) {
    el[i] = gst_element_factory_make(names[i], NULL);
    if (!el[i]) {
      snprintf(err, errlen, "GStreamer element %s is not installed", names[i]);
      for (int j = 0; j < i; j++) gst_object_unref(el[j]);
      return NULL;
    }
  }
  g_object_set(el[0], "format", GST_FORMAT_TIME, "stream-type", GST_APP_STREAM_TYPE_STREAM, "is-live", FALSE,
               "block", TRUE, "max-bytes", (guint64)0, "max-buffers", (guint64)0, "max-time", (guint64)max_queue_ns,
               "handle-segment-change", TRUE, "emit-signals", FALSE, NULL);
  g_object_set(el[4], "caps", p->pcm_caps, "sync", FALSE, "async", FALSE, "emit-signals", FALSE, "max-buffers",
               (guint)LANE_SINK_BUFFERS, "drop", FALSE, NULL);
  char name[16];
  snprintf(name, sizeof name, "sona-lane%d", index);
  GstElement* pipe = gst_pipeline_new(name);
  gst_bin_add_many(GST_BIN(pipe), el[0], el[1], el[2], el[3], el[4], NULL);
  if (!gst_element_link_many(el[0], el[1], el[2], el[3], el[4], NULL)) {
    snprintf(err, errlen, "cannot link appsrc ! %s ! audioconvert ! audioresample ! appsink", decoder);
    gst_object_unref(pipe);
    return NULL;
  }
  sona_lane* l = g_new0(sona_lane, 1);
  l->owner = p;
  l->index = index;
  l->pipe = pipe;
  l->src = el[0];
  l->sink = el[4];
  g_mutex_init(&l->mu);
  gst_segment_init(&l->segment, GST_FORMAT_TIME);
  GstPad* pad = gst_element_get_static_pad(l->src, "src");
  gst_pad_add_probe(pad, GST_PAD_PROBE_TYPE_BUFFER, drop_stale, l, NULL);
  gst_object_unref(pad);
  pad = gst_element_get_static_pad(l->sink, "sink");
  gst_pad_add_probe(pad, GST_PAD_PROBE_TYPE_BUFFER | GST_PAD_PROBE_TYPE_EVENT_DOWNSTREAM, stamp, l, NULL);
  gst_object_unref(pad);
  watch_bus(p, pipe, index, 0);
  if (gst_element_set_state(pipe, GST_STATE_PLAYING) == GST_STATE_CHANGE_FAILURE) {
    snprintf(err, errlen, "cannot start decode lane %d", index);
    sona_lane_free(l);
    return NULL;
  }
  return l;
}

void sona_lane_free(sona_lane* l) {
  if (!l) return;
  teardown(l->pipe);
  if (l->caps) gst_caps_unref(l->caps);
  g_mutex_clear(&l->mu);
  g_free(l);
}

void sona_lane_reset(sona_lane* l) {
  gst_element_set_state(l->pipe, GST_STATE_NULL);
  g_mutex_lock(&l->mu);
  l->need_segment = TRUE;
  g_mutex_unlock(&l->mu);
  gst_element_set_state(l->pipe, GST_STATE_PLAYING);
}

void sona_lane_flush(sona_lane* l, uint64_t gen, int64_t start_ns, int64_t stop_ns, const uint8_t* asc,
                     size_t asc_len, int rate, int channels) {
  g_mutex_lock(&l->mu);
  // Unblocks a push waiting for queue space and clears queued data and EOS.
  gst_element_send_event(l->src, gst_event_new_flush_start());
  __atomic_store_n(&l->gen, gen, __ATOMIC_RELEASE);
  __atomic_store_n(&l->out_gen, gen, __ATOMIC_RELEASE);
  gst_segment_init(&l->segment, GST_FORMAT_TIME);
  l->segment.start = start_ns;
  l->segment.time = start_ns;
  l->segment.position = start_ns;
  if (stop_ns >= 0) l->segment.stop = stop_ns;
  if (asc && asc_len) {
    GstBuffer* cd = gst_buffer_new_memdup(asc, asc_len);
    GstCaps* caps = gst_caps_new_simple("audio/mpeg", "mpegversion", G_TYPE_INT, 4, "stream-format", G_TYPE_STRING,
                                        "raw", "framed", G_TYPE_BOOLEAN, TRUE, "codec_data", GST_TYPE_BUFFER, cd, NULL);
    if (rate > 0) gst_caps_set_simple(caps, "rate", G_TYPE_INT, rate, NULL);
    if (channels > 0) gst_caps_set_simple(caps, "channels", G_TYPE_INT, channels, NULL);
    gst_buffer_unref(cd);
    if (l->caps) gst_caps_unref(l->caps);
    l->caps = caps;
  }
  l->need_segment = TRUE;
  gst_element_send_event(l->src, gst_event_new_flush_stop(TRUE));
  g_mutex_unlock(&l->mu);
}

int sona_lane_push(sona_lane* l, uint64_t gen, uint8_t* data, size_t size, int64_t pts_ns, int64_t dur_ns) {
  GstBuffer* b = gst_buffer_new_wrapped(data, size);
  GST_BUFFER_PTS(b) = pts_ns;
  GST_BUFFER_DURATION(b) = dur_ns;
  GST_BUFFER_OFFSET(b) = gen;
  GstSample* sample = NULL;
  g_mutex_lock(&l->mu);
  if (gen != l->gen) {
    g_mutex_unlock(&l->mu);
    gst_buffer_unref(b);
    return SONA_PUSH_STALE;
  }
  if (l->need_segment) {
    sample = gst_sample_new(b, l->caps, &l->segment, NULL);
    l->need_segment = FALSE;
  }
  g_mutex_unlock(&l->mu);
  if (sample) {
    GstFlowReturn r = gst_app_src_push_sample(GST_APP_SRC(l->src), sample);
    gst_sample_unref(sample);
    gst_buffer_unref(b);
    return flow(r);
  }
  return flow(gst_app_src_push_buffer(GST_APP_SRC(l->src), b));
}

int sona_lane_eos(sona_lane* l, uint64_t gen) {
  g_mutex_lock(&l->mu);
  int r = SONA_PUSH_STALE;
  if (gen == l->gen) r = flow(gst_app_src_end_of_stream(GST_APP_SRC(l->src)));
  g_mutex_unlock(&l->mu);
  return r;
}

uint64_t sona_lane_queued_ns(sona_lane* l) { return gst_app_src_get_current_level_time(GST_APP_SRC(l->src)); }

int sona_lane_pull(sona_lane* l, int64_t timeout_ns, sona_pcm* out) {
  memset(out, 0, sizeof(*out));
  GstSample* s = gst_app_sink_try_pull_sample(GST_APP_SINK(l->sink), (GstClockTime)timeout_ns);
  if (!s) {
    if (gst_app_sink_is_eos(GST_APP_SINK(l->sink))) {
      out->gen = __atomic_load_n(&l->eos_gen, __ATOMIC_ACQUIRE);
      return SONA_PULL_EOS;
    }
    return SONA_PULL_NONE;
  }
  GstBuffer* b = gst_buffer_ref(gst_sample_get_buffer(s));
  gst_sample_unref(s);
  out->buf = b;
  out->gen = GST_BUFFER_OFFSET(b);
  if (!gst_buffer_map(b, &out->map, GST_MAP_READ)) {
    gst_buffer_unref(b);
    out->buf = NULL;
    return SONA_PULL_NONE;
  }
  out->data = (float*)out->map.data;
  out->frames = out->map.size / (SONA_CHANNELS * sizeof(float));
  return SONA_PULL_OK;
}

void sona_pcm_release(sona_pcm* pcm) {
  if (!pcm->buf) return;
  gst_buffer_unmap(pcm->buf, &pcm->map);
  gst_buffer_unref(pcm->buf);
  pcm->buf = NULL;
}
