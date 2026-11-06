// The file pipeline (filehead.c) plays a plain audio file (a library
// upload: MP3, AAC or ALAC in MP4) whose bytes arrive from the parent over
// time:
//
//   source (random-access bytes from an in-memory store) ! decodebin
//     ! <tail: audioconvert, equaliser, volume (tail.h)> ! <sink>
//
// The store holds the whole file in memory as it arrives (never on disk);
// the demuxer pulls any range of it (an MP4's moov may sit at the end), and
// a read past the bytes received so far waits for them.
//
// A live pipeline plays a stream that has no length and cannot seek (a
// broadcaster's MP3 or ADTS AAC): the source hands its bytes on in order,
// as they arrive, and keeps only those not yet read.
#ifndef SONA_FILEHEAD_H_
#define SONA_FILEHEAD_H_

#include <stddef.h>
#include <stdint.h>

#include "tail.h"

typedef struct sona_file sona_file;

// sona_file_new builds the pipeline in NULL state for a file of size bytes,
// or, with live set, for a live stream (size is ignored). props is the
// sink's stream-properties (a GstStructure string) or, for osxaudiosink,
// its unique-id (a Core Audio device UID).
sona_file* sona_file_new(const char* sink, const char* client_name, const char* props, uint64_t size, int live,
                         char* err, size_t errlen);
void sona_file_free(sona_file* f);

sona_tail* sona_file_tail(sona_file* f);
// sona_file_set_state returns a GstStateChangeReturn. The first PLAYING lets
// the decoded audio through to the sink (see filehead.c's gate_probe).
int sona_file_set_state(sona_file* f, int state);
int sona_file_get_state(sona_file* f, int* state, int* pending);

// sona_file_write appends len bytes at off (where the previous write ended).
// Returns 0, or -1 if off is not the end of the data, the write runs past
// the file size, or a live stream would hold more than it may unread.
int sona_file_write(sona_file* f, uint64_t off, const uint8_t* data, size_t len);
// sona_file_complete marks the file complete: a read at its end then ends
// the stream.
void sona_file_complete(sona_file* f);
// sona_file_level reports the bytes received, the file size and the
// demuxer's read position.
void sona_file_level(sona_file* f, uint64_t* have, uint64_t* size, uint64_t* pos);

// sona_file_set_sink_props replaces the sink with one using props (a new
// PipeWire target.object, a new Core Audio device): output moves while
// decoding and the position carry on. Returns 0 on success.
int sona_file_set_sink_props(sona_file* f, const char* props);

// sona_file_seek seeks to pos_ns (flushing, accurate). Returns 0 on success.
int sona_file_seek(sona_file* f, int64_t pos_ns);
int sona_file_position(sona_file* f, int64_t* pos_ns);
int sona_file_duration(sona_file* f, int64_t* dur_ns);

enum {
  SONA_FILE_MSG_NONE = 0,
  SONA_FILE_MSG_EOS,
  SONA_FILE_MSG_ERROR,
  SONA_FILE_MSG_WARNING,
  SONA_FILE_MSG_STATE,  // pipeline state: a = old, b = new
  SONA_FILE_MSG_ASYNC_DONE,
};

typedef struct {
  int type;
  int a, b;
  char* text;  // g_malloc'd; free with g_free (sona_file_free_text)
} sona_file_msg;

// sona_file_pop waits up to timeout_ns for a bus message; latency and
// clock changes are handled internally.
int sona_file_pop(sona_file* f, int64_t timeout_ns, sona_file_msg* out);
void sona_file_wake(sona_file* f);
void sona_file_free_text(char* text);

#endif  // SONA_FILEHEAD_H_
