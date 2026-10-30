// C side of Sona's GStreamer output pipeline (gst.c):
// appsrc ! <aac decoder> ! audioconvert ! audioresample ! volume ! <sink>.
#ifndef SONA_GST_H_
#define SONA_GST_H_

#include <stddef.h>
#include <stdint.h>

typedef struct sona_gst sona_gst;

enum {
  SONA_MSG_NONE = 0,
  SONA_MSG_EOS,
  SONA_MSG_ERROR,
  SONA_MSG_WARNING,
  SONA_MSG_STATE,  // pipeline state: a = old, b = new, c = pending
  SONA_MSG_ASYNC_DONE,
  SONA_MSG_CLOCK_LOST,
};

typedef struct {
  int type;
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
};

int sona_gst_init(char* err, size_t errlen);
char* sona_gst_version(void);  // free with sona_gst_free
int sona_gst_have(const char* factory);

// sona_gst_new builds the pipeline in NULL state. props is a GstStructure
// string applied as the sink's stream-properties (if it has that property).
sona_gst* sona_gst_new(const char* decoder, const char* sink, const char* client_name, const char* props,
                       uint64_t max_queue_ns, char* err, size_t errlen);
void sona_gst_free_pipeline(sona_gst* p);

int sona_gst_set_state(sona_gst* p, int state);  // returns GstStateChangeReturn
int sona_gst_get_state(sona_gst* p, int64_t timeout_ns, int* state, int* pending);

// sona_gst_flush drops everything queued or playing, makes gen the current
// generation and arranges for the next pushed buffer to start a segment at
// start_ns with the given AAC caps. Buffers of older generations are dropped.
void sona_gst_flush(sona_gst* p, uint64_t gen, int64_t start_ns, const uint8_t* asc, size_t asc_len, int rate,
                    int channels);
// sona_gst_push pushes a g_malloc'd buffer (ownership taken), blocking while
// the queue is full.
int sona_gst_push(sona_gst* p, uint64_t gen, uint8_t* data, size_t size, int64_t pts_ns, int64_t dur_ns);
int sona_gst_end_of_stream(sona_gst* p, uint64_t gen);
uint8_t* sona_gst_alloc(size_t size);
void sona_gst_free(void* data);

void sona_gst_set_volume(sona_gst* p, double v);
int sona_gst_position(sona_gst* p, int64_t* pos_ns);
uint64_t sona_gst_queued_ns(sona_gst* p);

// sona_gst_pop waits up to timeout_ns for a bus message. Latency and clock
// changes are handled internally before being reported.
int sona_gst_pop(sona_gst* p, int64_t timeout_ns, sona_msg* out);
// sona_gst_wake makes a blocked sona_gst_pop return SONA_MSG_NONE.
void sona_gst_wake(sona_gst* p);

#endif  // SONA_GST_H_
