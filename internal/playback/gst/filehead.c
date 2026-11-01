#include "filehead.h"

#include "gst.h"

#include <gst/base/gstbasesrc.h>
#include <gst/gst.h>
#include <stdio.h>
#include <string.h>

// Bytes per buffer when the source drives the pipeline (push mode); in
// pull mode the demuxer asks for the ranges it needs.
#define FILE_BLOCK (64 * 1024)

// store is the file arriving in order. It is reference-counted: the source
// element and the sona_file hold one reference each, and every buffer
// handed out (which wraps the store's memory without copying) another.
typedef struct {
  GMutex mu;
  GCond cond;  // signalled when bytes arrive, the file completes or reads are cancelled
  uint8_t* data;
  uint64_t alloc;     // bytes allocated for data
  uint64_t size;      // the file's size (shrinks to have if the download ends short)
  uint64_t have;      // bytes received: data[:have]
  uint64_t pos;       // end of the last range read
  gboolean complete;  // every byte was received
  gboolean flushing;  // reads return FLUSHING (the source is unlocked)
} store;

static void store_free(gpointer p) {
  store* s = p;
  g_mutex_clear(&s->mu);
  g_cond_clear(&s->cond);
  g_free(s->data);
}

static void store_unref(gpointer p) { g_atomic_rc_box_release_full(p, store_free); }

// SonaBytesSrc serves the store as a random-access byte source: demuxers
// pull any range (qtdemux reads an MP4's moov wherever it is, tag demuxers
// read the file's end), and a read of bytes not yet received waits for them
// until the source is unlocked (flush, stop).
typedef struct {
  GstBaseSrc parent;
  store* st;
} SonaBytesSrc;

typedef struct {
  GstBaseSrcClass parent;
} SonaBytesSrcClass;

G_DEFINE_TYPE(SonaBytesSrc, sona_bytes_src, GST_TYPE_BASE_SRC)

static GstStaticPadTemplate bytes_src_template =
    GST_STATIC_PAD_TEMPLATE("src", GST_PAD_SRC, GST_PAD_ALWAYS, GST_STATIC_CAPS_ANY);

static store* bytes_store(GstBaseSrc* src) { return ((SonaBytesSrc*)src)->st; }

static gboolean bytes_is_seekable(GstBaseSrc* src) { return TRUE; }

static gboolean bytes_get_size(GstBaseSrc* src, guint64* size) {
  store* s = bytes_store(src);
  g_mutex_lock(&s->mu);
  *size = s->size;
  g_mutex_unlock(&s->mu);
  return TRUE;
}

static gboolean bytes_unlock(GstBaseSrc* src) {
  store* s = bytes_store(src);
  g_mutex_lock(&s->mu);
  s->flushing = TRUE;
  g_cond_broadcast(&s->cond);
  g_mutex_unlock(&s->mu);
  return TRUE;
}

static gboolean bytes_unlock_stop(GstBaseSrc* src) {
  store* s = bytes_store(src);
  g_mutex_lock(&s->mu);
  s->flushing = FALSE;
  g_mutex_unlock(&s->mu);
  return TRUE;
}

// bytes_create returns [off, off+len) once received (less at the end of
// the file), waiting for it.
static GstFlowReturn bytes_create(GstBaseSrc* src, guint64 off, guint len, GstBuffer** out) {
  store* s = bytes_store(src);
  g_mutex_lock(&s->mu);
  guint64 end;
  for (;;) {
    if (s->flushing) {
      g_mutex_unlock(&s->mu);
      return GST_FLOW_FLUSHING;
    }
    if (off >= s->size) {
      g_mutex_unlock(&s->mu);
      return GST_FLOW_EOS;
    }
    end = MIN(off + len, s->size);
    if (s->have >= end) break;
    g_cond_wait(&s->cond, &s->mu);
  }
  GstBuffer* b = gst_buffer_new();
  gst_buffer_append_memory(b, gst_memory_new_wrapped(GST_MEMORY_FLAG_READONLY, s->data, s->alloc, off, end - off,
                                                     g_atomic_rc_box_acquire(s), store_unref));
  GST_BUFFER_OFFSET(b) = off;
  GST_BUFFER_OFFSET_END(b) = end;
  s->pos = end;
  g_mutex_unlock(&s->mu);
  *out = b;
  return GST_FLOW_OK;
}

static void sona_bytes_src_finalize(GObject* obj) {
  SonaBytesSrc* b = (SonaBytesSrc*)obj;
  if (b->st) store_unref(b->st);
  G_OBJECT_CLASS(sona_bytes_src_parent_class)->finalize(obj);
}

static void sona_bytes_src_class_init(SonaBytesSrcClass* klass) {
  G_OBJECT_CLASS(klass)->finalize = sona_bytes_src_finalize;
  GstElementClass* ec = GST_ELEMENT_CLASS(klass);
  gst_element_class_add_static_pad_template(ec, &bytes_src_template);
  gst_element_class_set_static_metadata(ec, "Sona file bytes", "Source", "A file received in memory", "Sona");
  GstBaseSrcClass* bc = GST_BASE_SRC_CLASS(klass);
  bc->is_seekable = bytes_is_seekable;
  bc->get_size = bytes_get_size;
  bc->unlock = bytes_unlock;
  bc->unlock_stop = bytes_unlock_stop;
  bc->create = bytes_create;
}

static void sona_bytes_src_init(SonaBytesSrc* b) {
  gst_base_src_set_format(GST_BASE_SRC(b), GST_FORMAT_BYTES);
  gst_base_src_set_blocksize(GST_BASE_SRC(b), FILE_BLOCK);
}

struct sona_file {
  GstElement* pipeline;
  GstElement* tailbin;
  GMutex mu;
  GstElement* sink;   // guarded by mu (a device move replaces it)
  gulong gate;        // guarded by mu: the probe holding audio back (see gate_probe); 0 once open
  gboolean rendered;  // guarded by mu: the sink has rendered (latched; see sink_has_rendered)
  gboolean eos;       // guarded by mu: the stream ended and was not sought since
  gboolean unlinked;  // guarded by mu: the sink (replaced while not playing) is linked when the gate opens
  char* sink_factory;
  char* client_name;
  GstBus* bus;
  sona_tail* tail;
  store* st;
};

// pad_added links decodebin's audio pad to the tail.
static void pad_added(GstElement* dec, GstPad* pad, gpointer user) {
  GstElement* tail = user;
  GstCaps* caps = gst_pad_get_current_caps(pad);
  if (!caps) caps = gst_pad_query_caps(pad, NULL);
  gboolean audio = caps && gst_caps_get_size(caps) > 0 &&
                   g_str_has_prefix(gst_structure_get_name(gst_caps_get_structure(caps, 0)), "audio/");
  if (caps) gst_caps_unref(caps);
  if (!audio) return;
  GstPad* sink = gst_element_get_static_pad(tail, "sink");
  if (!gst_pad_is_linked(sink) && gst_pad_link(pad, sink) != GST_PAD_LINK_OK) {
    GST_ELEMENT_ERROR(dec, CORE, NEGOTIATION, ("cannot link the decoded audio of the file"), (NULL));
  }
  gst_object_unref(sink);
}

static gboolean has_prop(gpointer obj, const char* name) {
  return g_object_class_find_property(G_OBJECT_GET_CLASS(obj), name) != NULL;
}

// configure_sink sets the sink's clocking, client name and stream
// properties (which carry the PipeWire target.object). A sink behind the
// closed gate does not preroll (async off; see gate_probe).
static void configure_sink(GstElement* snk, const char* client_name, const char* props, gboolean async) {
  if (has_prop(snk, "sync")) g_object_set(snk, "sync", TRUE, NULL);
  if (has_prop(snk, "async")) g_object_set(snk, "async", async, NULL);
  if (client_name && has_prop(snk, "client-name")) g_object_set(snk, "client-name", client_name, NULL);
  if (props && *props && has_prop(snk, "stream-properties")) {
    GstStructure* s = gst_structure_from_string(props, NULL);
    if (s) {
      g_object_set(snk, "stream-properties", s, NULL);
      gst_structure_free(s);
    }
  }
}

// gate_probe holds the decoded audio back from the tail and sink until the
// pipeline goes to PLAYING (sona_file_set_state opens the gate).
// pipewiresink (1.6) must never sit in PAUSED with a connected stream
// before it has rendered: PipeWire's format reaches its event thread, which
// then waits for the sink's buffer pool, started only by the first render,
// and a flush (seek) or stop blocks on that thread for good (as does every
// other pipewiresink in the process, which share it). While the gate is
// closed the sink does not preroll, so the pipeline reaches PAUSED once
// decodebin has exposed its pads: the duration is known and seeks work.
// Once open the sink prerolls again (async), so a flushing seek while
// playing restarts the clock at the new position.
static GstPadProbeReturn gate_probe(GstPad* pad, GstPadProbeInfo* info, gpointer user) { return GST_PAD_PROBE_OK; }

static void close_gate_locked(sona_file* f) {
  if (f->gate) return;
  GstPad* pad = gst_element_get_static_pad(f->tailbin, "sink");
  f->gate = gst_pad_add_probe(pad, GST_PAD_PROBE_TYPE_BLOCK_DOWNSTREAM, gate_probe, NULL, NULL);
  gst_object_unref(pad);
}

static void open_gate(sona_file* f) {
  g_mutex_lock(&f->mu);
  if (f->gate) {
    if (has_prop(f->sink, "async")) g_object_set(f->sink, "async", TRUE, NULL);
    if (f->unlinked && gst_element_link(f->tailbin, f->sink)) f->unlinked = FALSE;
    GstPad* pad = gst_element_get_static_pad(f->tailbin, "sink");
    gst_pad_remove_probe(pad, f->gate);
    gst_object_unref(pad);
    f->gate = 0;
  }
  g_mutex_unlock(&f->mu);
}

static guint64 sink_rendered(GstElement* snk) {
  guint64 n = 0;
  if (!has_prop(snk, "stats")) return 0;
  GstStructure* st = NULL;
  g_object_get(snk, "stats", &st, NULL);
  if (st) {
    gst_structure_get_uint64(st, "rendered", &n);
    gst_structure_free(st);
  }
  return n;
}

// sink_has_rendered reports whether the sink in use has ever rendered a
// buffer (its counter restarts on flushes, so the answer is latched).
static gboolean sink_has_rendered(sona_file* f, GstElement** snk) {
  g_mutex_lock(&f->mu);
  GstElement* s = gst_object_ref(f->sink);
  gboolean r = f->rendered;
  g_mutex_unlock(&f->mu);
  if (!r && sink_rendered(s) > 0) {
    g_mutex_lock(&f->mu);
    if (f->sink == s) f->rendered = r = TRUE;
    g_mutex_unlock(&f->mu);
  }
  if (snk) {
    *snk = s;
  } else {
    gst_object_unref(s);
  }
  return r;
}

// settle_sink waits (up to 2 s) until a sink whose stream is connected
// (it has caps) has rendered once, before the pipeline leaves PLAYING: a
// Pause or Close in that window would leave it connected, unrendered and
// stuck (see gate_probe). Rendering follows within milliseconds in PLAYING.
static void settle_sink(sona_file* f) {
  GstElement* snk = NULL;
  if (sink_has_rendered(f, &snk)) {
    gst_object_unref(snk);
    return;
  }
  GstPad* pad = gst_element_get_static_pad(snk, "sink");
  for (int i = 0; i < 200 && gst_pad_has_current_caps(pad) && !sink_has_rendered(f, NULL); i++) g_usleep(10 * 1000);
  gst_object_unref(pad);
  gst_object_unref(snk);
}

// sample_bytes is the size of one sample of a signed or float raw audio
// format (whose silence is all zero bytes), or 0.
static int sample_bytes(const char* fmt) {
  static const struct {
    const char* prefix;
    int bytes;
  } formats[] = {{"S8", 1}, {"S16", 2}, {"S24_32", 4}, {"S24", 3}, {"S20", 3}, {"S18", 3},
                 {"S32", 4}, {"F32", 4}, {"F64", 8}};
  for (size_t i = 0; fmt && i < G_N_ELEMENTS(formats); i++) {
    if (g_str_has_prefix(fmt, formats[i].prefix)) return formats[i].bytes;
  }
  return 0;
}

// prime_probe watches the tail's output. A stream that ends before the sink
// rendered anything (a seek past the end) would leave the sink connected
// with its format but unrendered, which pipewiresink never recovers from
// (see gate_probe): 10 ms of silence, untimed so it renders at once, goes
// ahead of such an end. It is chained into the sink directly: this pad is
// already marked ended.
static GstPadProbeReturn prime_probe(GstPad* pad, GstPadProbeInfo* info, gpointer user) {
  sona_file* f = user;
  if (GST_EVENT_TYPE(GST_PAD_PROBE_INFO_EVENT(info)) != GST_EVENT_EOS) return GST_PAD_PROBE_OK;
  g_mutex_lock(&f->mu);
  f->eos = TRUE;
  g_mutex_unlock(&f->mu);
  if (sink_has_rendered(f, NULL)) return GST_PAD_PROBE_OK;
  GstPad* peer = gst_pad_get_peer(pad);
  GstCaps* caps = peer ? gst_pad_get_current_caps(peer) : NULL;
  if (caps) {
    GstStructure* s = gst_caps_get_structure(caps, 0);
    int rate = 0, channels = 0;
    int bytes = gst_structure_has_name(s, "audio/x-raw") ? sample_bytes(gst_structure_get_string(s, "format")) : 0;
    if (bytes > 0 && gst_structure_get_int(s, "rate", &rate) && gst_structure_get_int(s, "channels", &channels) &&
        rate > 0 && channels > 0) {
      gsize size = (gsize)(rate / 100) * channels * bytes;
      GstBuffer* b = gst_buffer_new_allocate(NULL, size, NULL);
      gst_buffer_memset(b, 0, 0, size);
      gst_pad_chain(peer, b);
    }
    gst_caps_unref(caps);
  }
  if (peer) gst_object_unref(peer);
  return GST_PAD_PROBE_OK;
}

// is_playing reports whether the pipeline was last asked to play, whether or
// not it got there. Not its current or pending state: right after a pause
// (PLAYING to PAUSED is asynchronous: the sink waits for a buffer to
// preroll), it is still PLAYING, with PAUSED pending, for as long as that
// takes, which under load can be long, while the sink already holds the
// stream as a paused one.
static gboolean is_playing(sona_file* f) {
  GST_OBJECT_LOCK(f->pipeline);
  gboolean r = GST_STATE_TARGET(f->pipeline) == GST_STATE_PLAYING;
  GST_OBJECT_UNLOCK(f->pipeline);
  return r;
}

sona_file* sona_file_new(const char* sink, const char* client_name, const char* props, uint64_t size, char* err,
                         size_t errlen) {
  GstElement* dec = gst_element_factory_make("decodebin", NULL);
  GstElement* snk = gst_element_factory_make(sink, NULL);
  if (!dec || !snk) {
    snprintf(err, errlen, "GStreamer element %s is not installed", !dec ? "decodebin" : sink);
    if (dec) gst_object_unref(dec);
    if (snk) gst_object_unref(snk);
    return NULL;
  }
  sona_tail* tail = sona_tail_create();
  GstElement* tb = sona_tail_build(tail, err, errlen);
  if (!tb) {
    gst_object_unref(dec);
    gst_object_unref(snk);
    sona_tail_destroy(tail);
    return NULL;
  }
  configure_sink(snk, client_name, props, FALSE);
  store* st = g_atomic_rc_box_new0(store);
  g_mutex_init(&st->mu);
  g_cond_init(&st->cond);
  st->size = st->alloc = size;
  st->data = g_malloc(size ? size : 1);
  SonaBytesSrc* src = g_object_new(sona_bytes_src_get_type(), NULL);
  src->st = g_atomic_rc_box_acquire(st);
  g_signal_connect(dec, "pad-added", G_CALLBACK(pad_added), tb);

  GstElement* pipe = gst_pipeline_new("sona-file");
  gst_bin_add_many(GST_BIN(pipe), GST_ELEMENT(src), dec, tb, snk, NULL);
  if (!gst_element_link(GST_ELEMENT(src), dec) || !gst_element_link(tb, snk)) {
    snprintf(err, errlen, "cannot link source ! decodebin ! tail ! %s", sink);
    gst_object_unref(pipe);
    store_unref(st);
    sona_tail_destroy(tail);
    return NULL;
  }
  sona_file* f = g_new0(sona_file, 1);
  f->pipeline = pipe;
  f->tailbin = tb;
  f->sink = snk;
  f->sink_factory = g_strdup(sink);
  f->client_name = g_strdup(client_name);
  f->bus = gst_element_get_bus(pipe);
  f->tail = tail;
  f->st = st;
  g_mutex_init(&f->mu);
  close_gate_locked(f);
  GstPad* out = gst_element_get_static_pad(tb, "src");
  gst_pad_add_probe(out, GST_PAD_PROBE_TYPE_EVENT_DOWNSTREAM, prime_probe, f, NULL);
  gst_object_unref(out);
  // The system clock, with pipewiresink resampling to the device: the
  // clock pipewiresink provides starts ahead by its buffering, which showed
  // as the position leaping forward when playback started.
  GstClock* clock = gst_system_clock_obtain();
  gst_pipeline_use_clock(GST_PIPELINE(pipe), clock);
  gst_object_unref(clock);
  return f;
}

void sona_file_free(sona_file* f) {
  if (!f) return;
  if (is_playing(f)) settle_sink(f);
  gst_element_set_state(f->pipeline, GST_STATE_NULL);
  gst_object_unref(f->bus);
  gst_object_unref(f->pipeline);
  store_unref(f->st);
  sona_tail_destroy(f->tail);
  g_mutex_clear(&f->mu);
  g_free(f->sink_factory);
  g_free(f->client_name);
  g_free(f);
}

sona_tail* sona_file_tail(sona_file* f) { return f->tail; }

int sona_file_set_state(sona_file* f, int state) {
  if (state < GST_STATE_PLAYING && is_playing(f)) settle_sink(f);
  int r = gst_element_set_state(f->pipeline, (GstState)state);
  if (state == GST_STATE_PLAYING && r != GST_STATE_CHANGE_FAILURE) open_gate(f);
  return r;
}

int sona_file_get_state(sona_file* f, int* state, int* pending) {
  GstState s = GST_STATE_VOID_PENDING, pend = GST_STATE_VOID_PENDING;
  GstStateChangeReturn r = gst_element_get_state(f->pipeline, &s, &pend, 0);
  *state = s;
  *pending = pend;
  return r;
}

int sona_file_write(sona_file* f, uint64_t off, const uint8_t* data, size_t len) {
  store* s = f->st;
  g_mutex_lock(&s->mu);
  int r = -1;
  if (!s->complete && off == s->have && len <= s->size - s->have) {
    memcpy(s->data + off, data, len);
    s->have += len;
    g_cond_broadcast(&s->cond);
    r = 0;
  }
  g_mutex_unlock(&s->mu);
  return r;
}

void sona_file_complete(sona_file* f) {
  store* s = f->st;
  g_mutex_lock(&s->mu);
  s->complete = TRUE;
  s->size = s->have;
  g_cond_broadcast(&s->cond);
  g_mutex_unlock(&s->mu);
}

void sona_file_level(sona_file* f, uint64_t* have, uint64_t* size, uint64_t* pos) {
  store* s = f->st;
  g_mutex_lock(&s->mu);
  *have = s->have;
  *size = s->size;
  *pos = s->pos;
  g_mutex_unlock(&s->mu);
}

int sona_file_seek(sona_file* f, int64_t pos_ns) {
  g_mutex_lock(&f->mu);
  f->eos = FALSE;
  gboolean unlinked = f->unlinked;
  g_mutex_unlock(&f->mu);
  GstEvent* seek = gst_event_new_seek(1.0, GST_FORMAT_TIME, GST_SEEK_FLAG_FLUSH | GST_SEEK_FLAG_ACCURATE,
                                      GST_SEEK_TYPE_SET, pos_ns, GST_SEEK_TYPE_NONE, -1);
  gboolean ok;
  if (unlinked) {
    // No sink to route the seek through (see replace_sink): send it
    // upstream from the tail. The sink would have had the pipeline restart
    // its running time (the flush resets it to 0); without one, do so here.
    GstPad* in = gst_element_get_static_pad(f->tailbin, "sink");
    ok = gst_pad_push_event(in, seek);
    gst_object_unref(in);
    if (ok) gst_element_set_start_time(f->pipeline, 0);
  } else {
    ok = gst_element_send_event(f->pipeline, seek);
  }
  return ok ? 0 : -1;
}

int sona_file_position(sona_file* f, int64_t* pos_ns) {
  gint64 pos = -1;
  if (!gst_element_query_position(f->pipeline, GST_FORMAT_TIME, &pos) || pos < 0) return 0;
  *pos_ns = pos;
  return 1;
}

int sona_file_duration(sona_file* f, int64_t* dur_ns) {
  gint64 d = -1;
  if (!gst_element_query_duration(f->pipeline, GST_FORMAT_TIME, &d) || d <= 0) return 0;
  *dur_ns = d;
  return 1;
}

int sona_file_pop(sona_file* f, int64_t timeout_ns, sona_file_msg* out) {
  memset(out, 0, sizeof(*out));
  GstMessage* m = gst_bus_timed_pop_filtered(
      f->bus, (GstClockTime)timeout_ns,
      GST_MESSAGE_EOS | GST_MESSAGE_ERROR | GST_MESSAGE_WARNING | GST_MESSAGE_STATE_CHANGED | GST_MESSAGE_ASYNC_DONE |
          GST_MESSAGE_CLOCK_LOST | GST_MESSAGE_LATENCY | GST_MESSAGE_APPLICATION);
  if (!m) return SONA_FILE_MSG_NONE;
  GError* e = NULL;
  gchar* dbg = NULL;
  switch (GST_MESSAGE_TYPE(m)) {
    case GST_MESSAGE_EOS:
      out->type = SONA_FILE_MSG_EOS;
      break;
    case GST_MESSAGE_ERROR:
    case GST_MESSAGE_WARNING:
      if (sona_relink_error(m)) break;  // PipeWire moved the stream; the sink recovers
      if (GST_MESSAGE_TYPE(m) == GST_MESSAGE_ERROR) {
        gst_message_parse_error(m, &e, &dbg);
        out->type = SONA_FILE_MSG_ERROR;
      } else {
        gst_message_parse_warning(m, &e, &dbg);
        out->type = SONA_FILE_MSG_WARNING;
      }
      out->text = g_strdup_printf("%s: %s (%s)", GST_OBJECT_NAME(GST_MESSAGE_SRC(m)), e ? e->message : "unknown",
                                  dbg ? dbg : "");
      g_clear_error(&e);
      g_free(dbg);
      break;
    case GST_MESSAGE_STATE_CHANGED:
      if (GST_MESSAGE_SRC(m) == GST_OBJECT(f->pipeline)) {
        GstState o, n, pend;
        gst_message_parse_state_changed(m, &o, &n, &pend);
        out->type = SONA_FILE_MSG_STATE;
        out->a = o;
        out->b = n;
      }
      break;
    case GST_MESSAGE_ASYNC_DONE:
      out->type = SONA_FILE_MSG_ASYNC_DONE;
      break;
    case GST_MESSAGE_CLOCK_LOST: {
      GstState cur, pend;
      gst_element_get_state(f->pipeline, &cur, &pend, 0);
      if (cur == GST_STATE_PLAYING) {
        gst_element_set_state(f->pipeline, GST_STATE_PAUSED);
        gst_element_set_state(f->pipeline, GST_STATE_PLAYING);
      }
      break;
    }
    case GST_MESSAGE_LATENCY:
      gst_bin_recalculate_latency(GST_BIN(f->pipeline));
      break;
    default:
      break;
  }
  gst_message_unref(m);
  return out->type;
}

void sona_file_wake(sona_file* f) {
  gst_bus_post(f->bus, gst_message_new_application(NULL, gst_structure_new_empty("sona-wake")));
}

void sona_file_free_text(char* text) { g_free(text); }

// replace_sink puts a fresh sink with stream properties props in place of
// the one in use, so the stream reconnects to the new target. With hold the
// gate closes and the new sink stays unlinked until it opens (PLAYING): not
// even the stream's format may reach it before (see gate_probe), and a
// flush would carry the format along. With restart >= 0 the stream, which
// stopping the old sink released, restarts there.
static int replace_sink(sona_file* f, const char* props, gboolean hold, gint64 restart) {
  GstElement* snk = gst_element_factory_make(f->sink_factory, NULL);
  if (!snk) return -1;
  g_mutex_lock(&f->mu);
  if (hold) close_gate_locked(f);
  configure_sink(snk, f->client_name, props, f->gate == 0);
  GstElement* old = f->sink;
  gboolean linked = !f->unlinked;
  f->sink = snk;
  f->rendered = FALSE;
  f->unlinked = hold;
  g_mutex_unlock(&f->mu);
  if (linked) gst_element_unlink(f->tailbin, old);
  gst_element_set_state(old, GST_STATE_NULL);
  gst_bin_remove(GST_BIN(f->pipeline), old);
  gst_bin_add(GST_BIN(f->pipeline), snk);
  if (!hold && !gst_element_link(f->tailbin, snk)) return -1;
  gst_element_sync_state_with_parent(snk);
  return restart >= 0 ? sona_file_seek(f, restart) : 0;
}

// swap is a sink replacement waiting for the tail's output to go idle; it
// is shared with the probe, which may outlive the wait.
typedef struct {
  sona_file* f;
  char* props;
  GMutex mu;
  GCond cond;
  gboolean started, cancelled, done;
  int result;
} swap;

static GstPadProbeReturn swap_probe(GstPad* pad, GstPadProbeInfo* info, gpointer user) {
  swap* w = user;
  g_mutex_lock(&w->mu);
  w->started = !w->cancelled;
  g_mutex_unlock(&w->mu);
  int r = w->started ? replace_sink(w->f, w->props, FALSE, -1) : -1;
  g_mutex_lock(&w->mu);
  w->result = r;
  w->done = TRUE;
  g_cond_signal(&w->cond);
  g_mutex_unlock(&w->mu);
  return GST_PAD_PROBE_REMOVE;
}

static void swap_free(gpointer p) {
  swap* w = p;
  g_mutex_clear(&w->mu);
  g_cond_clear(&w->cond);
  g_free(w->props);
}

static void swap_unref(gpointer p) { g_atomic_rc_box_release_full(p, swap_free); }

int sona_file_set_sink_props(sona_file* f, const char* props) {
  if (!is_playing(f)) {
    // Paused (also while the pause completes) or not yet played: nothing
    // flows to the sink, or the old sink holds the paused stream, which
    // stopping it releases. Swap at once behind the closed gate, then
    // restart the stream where it was (unless it had ended), held at the
    // gate until PLAYING.
    g_mutex_lock(&f->mu);
    gboolean flowing = f->gate == 0 && !f->eos;
    g_mutex_unlock(&f->mu);
    gint64 pos = -1;
    if (flowing && !gst_element_query_position(f->pipeline, GST_FORMAT_TIME, &pos)) pos = -1;
    return replace_sink(f, props, TRUE, pos);
  }
  // Playing (or ended while playing): swap between two buffers, when the
  // tail's output is idle; at once when nothing flows.
  swap* w = g_atomic_rc_box_new0(swap);
  w->f = f;
  w->props = g_strdup(props);
  w->result = -1;
  g_mutex_init(&w->mu);
  g_cond_init(&w->cond);
  GstPad* pad = gst_element_get_static_pad(f->tailbin, "src");
  gulong id = gst_pad_add_probe(pad, GST_PAD_PROBE_TYPE_IDLE, swap_probe, g_atomic_rc_box_acquire(w), swap_unref);
  g_mutex_lock(&w->mu);
  gint64 until = g_get_monotonic_time() + 2 * G_TIME_SPAN_SECOND;
  while (!w->done && g_cond_wait_until(&w->cond, &w->mu, until)) {
  }
  if (!w->done && !w->started) {
    // Never idle (the sink is stuck rendering): give up, and make sure the
    // probe cannot swap later.
    w->cancelled = TRUE;
    g_mutex_unlock(&w->mu);
    if (id) gst_pad_remove_probe(pad, id);
    g_mutex_lock(&w->mu);
  }
  while (w->started && !w->done) g_cond_wait(&w->cond, &w->mu);
  int r = w->done ? w->result : -1;
  g_mutex_unlock(&w->mu);
  gst_object_unref(pad);
  swap_unref(w);
  return r;
}
