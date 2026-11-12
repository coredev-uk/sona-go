// C side of Sona's GStreamer audio output (gst.c). Two decode lanes feed a
// mixer (gst.go) that writes one continuous PCM stream to the output:
//
//   lane:   appsrc (AAC access units) ! <aac decoder> ! audioconvert
//             ! audioresample ! appsink (F32LE interleaved stereo, SONA_RATE)
//   output: appsrc (mixed PCM) ! <tail: equaliser, volume (tail.h)> ! <sink>
//
// Lanes never touch the sink, so a track is decoded and trimmed by its own
// decoder while the output keeps playing, and the output can be rebuilt
// (device switch, sink failure) without losing decoded audio.
#ifndef SONA_GST_H_
#define SONA_GST_H_

#include <gst/gst.h>
#include <stddef.h>
#include <stdint.h>

#define SONA_RATE 44100
#define SONA_CHANNELS 2

typedef struct sona_gst sona_gst;
typedef struct sona_lane sona_lane;
struct sona_tail;

enum {
  SONA_MSG_NONE = 0,
  SONA_MSG_EOS,
  SONA_MSG_ERROR,
  SONA_MSG_WARNING,
  SONA_MSG_STATE,  // output pipeline state: a = old, b = new, c = pending
  SONA_MSG_ASYNC_DONE,
  SONA_MSG_CLOCK_LOST,
  SONA_MSG_LATENCY,
  // pipewiresink lost its buffers because PipeWire relinked the stream
  // (default device change, sink node destroyed). Not fatal: the stream
  // gets new buffers once linked again.
  SONA_MSG_RELINK,
  SONA_MSG_WAKE,
};

// sona_msg is a bus message of the output (lane < 0, build = the output
// build that posted it) or of lane `lane`.
typedef struct {
  int type;
  int lane;
  uint64_t build;
  int a, b, c;
  char* text;   // g_malloc'd; free with sona_gst_free
  char* debug;  // g_malloc'd; free with sona_gst_free
} sona_msg;

enum {
  SONA_PUSH_OK = 0,
  SONA_PUSH_STALE = 1,     // generation changed: a flush superseded the buffer
  SONA_PUSH_FLUSHING = 2,  // flushed while blocked
  SONA_PUSH_EOS = 3,
  SONA_PUSH_ERROR = 4,
  SONA_PUSH_NO_OUTPUT = 5,  // the output is being rebuilt
};

enum {
  SONA_PULL_NONE = 0,  // nothing within the timeout (or flushing)
  SONA_PULL_OK = 1,
  SONA_PULL_EOS = 2,
};

// sona_pcm is a mapped buffer of interleaved stereo float frames.
typedef struct {
  GstBuffer* buf;
  GstMapInfo map;
  float* data;
  size_t frames;
  uint64_t gen;  // lane pulls: the generation that produced it
} sona_pcm;

int sona_gst_init(char* err, size_t errlen);
char* sona_gst_version(void);  // free with sona_gst_free
int sona_gst_have(const char* factory);
void sona_gst_free(void* data);
uint8_t* sona_gst_alloc(size_t size);

// sona_relink_error reports whether m is pipewiresink's "all buffers have
// been removed" error, which PipeWire triggers by relinking the stream
// (relink_linux.c; never on macOS, relink_darwin.c).
int sona_relink_error(GstMessage* m);

// sona_gst_new makes the shared state (message queue, tail) and builds the
// output: sink with props as its stream-properties (a GstStructure string,
// if the sink has that property) or else its unique-id (osxaudiosink: a
// Core Audio device UID), brought to READY.
sona_gst* sona_gst_new(const char* sink, const char* client_name, const char* props, char* err, size_t errlen);
void sona_gst_free_pipeline(sona_gst* p);
struct sona_tail* sona_gst_tail(sona_gst* p);

// sona_gst_out_rebuild replaces the output pipeline (props NULL keeps the
// sink's properties) and brings it to state. The next pushed buffer opens a
// segment where the previous output stopped. Returns the new build (> 0),
// or 0 with err set.
uint64_t sona_gst_out_rebuild(sona_gst* p, const char* props, int state, char* err, size_t errlen);
int sona_gst_set_state(sona_gst* p, int state);  // output; returns GstStateChangeReturn
int sona_gst_position(sona_gst* p, int64_t* pos_ns);  // output stream time
uint64_t sona_gst_out_queued_ns(sona_gst* p);
// sona_gst_out_flush drops everything the output holds and makes epoch
// current: pushes of other epochs are dropped, the next push opens a
// segment at 0.
void sona_gst_out_flush(sona_gst* p, uint64_t epoch);
// sona_gst_out_alloc returns a writable mapped buffer of frames.
void sona_gst_out_alloc(sona_gst* p, size_t frames, sona_pcm* out);
// sona_gst_out_push unmaps pcm and pushes its first frames at pts_ns
// (ownership taken), blocking while the output queue is full.
int sona_gst_out_push(sona_gst* p, uint64_t epoch, sona_pcm* pcm, size_t frames, int64_t pts_ns);
int sona_gst_out_eos(sona_gst* p, uint64_t epoch);
void sona_gst_out_recalc_latency(sona_gst* p);
void sona_gst_out_clock_lost(sona_gst* p);

// sona_gst_pop waits up to timeout_ns for a message of any pipeline; on
// timeout it returns SONA_MSG_NONE as an output message (lane -1).
int sona_gst_pop(sona_gst* p, int64_t timeout_ns, sona_msg* out);
void sona_gst_wake(sona_gst* p);

// Lanes. sona_lane_new builds lane `index` with its own pipeline, PLAYING.
sona_lane* sona_lane_new(sona_gst* p, int index, const char* decoder, uint64_t max_queue_ns, char* err,
                         size_t errlen);
void sona_lane_free(sona_lane* l);
// sona_lane_reset restarts a lane after a decoder error.
void sona_lane_reset(sona_lane* l);
// sona_lane_flush drops what the lane holds and makes gen current; the next
// push opens a segment [start_ns, stop_ns) (stop_ns < 0: open) with the
// given AAC caps (asc NULL keeps the previous caps). The decoder clips its
// output to the segment.
void sona_lane_flush(sona_lane* l, uint64_t gen, int64_t start_ns, int64_t stop_ns, const uint8_t* asc,
                     size_t asc_len, int rate, int channels);
// sona_lane_push pushes a g_malloc'd access unit (ownership taken),
// blocking while the lane's queue is full.
int sona_lane_push(sona_lane* l, uint64_t gen, uint8_t* data, size_t size, int64_t pts_ns, int64_t dur_ns);
int sona_lane_eos(sona_lane* l, uint64_t gen);
uint64_t sona_lane_queued_ns(sona_lane* l);
// sona_lane_pull waits up to timeout_ns for decoded audio (mapped
// read-only into out) or the lane's end of stream (out->gen says whose).
int sona_lane_pull(sona_lane* l, int64_t timeout_ns, sona_pcm* out);
void sona_pcm_release(sona_pcm* pcm);

#endif  // SONA_GST_H_
