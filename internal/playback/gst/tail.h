// The output tail (tail.c): Sona's audio processing between the decoded
// PCM and the sink, rebuilt with every output pipeline:
//
//   audioconvert ! volume name=pregain ! equalizer-10bands name=eq
//     ! audioconvert ! audioresample ! volume name=vol
//
// pregain is the equaliser's preamp (headroom for its boosts), vol the
// user's volume. A sona_tail keeps the settings across rebuilds and
// applies changes to the live bin at once (the next buffer through it).
#ifndef SONA_TAIL_H_
#define SONA_TAIL_H_

#include <gst/gst.h>
#include <stddef.h>

#define SONA_EQ_BANDS 10

typedef struct sona_tail sona_tail;

// sona_tail_create returns a tail at unity volume with a flat equaliser.
sona_tail* sona_tail_create(void);
void sona_tail_destroy(sona_tail* t);

// sona_tail_build makes a new bin with ghost "sink" and "src" pads, set to
// t's settings, and makes it the bin t's setters change (until it is
// finalized). Without equalizer-10bands installed the bin has no
// equaliser. Returns NULL with err set on failure.
GstElement* sona_tail_build(sona_tail* t, char* err, size_t errlen);

void sona_tail_set_volume(sona_tail* t, double linear);
// sona_tail_set_eq sets the preamp (dB) and the band gains (dB).
void sona_tail_set_eq(sona_tail* t, double pregain_db, const double bands_db[SONA_EQ_BANDS]);

// sona_tail_read reads the live bin's settings back from its elements
// (volume and pregain linear, bands in dB). Returns 0 when no bin is live
// or it has no equaliser (bands then untouched).
int sona_tail_read(sona_tail* t, double* volume, double* pregain, double bands_db[SONA_EQ_BANDS]);

#endif  // SONA_TAIL_H_
