#include "tail.h"

#include <math.h>
#include <stdio.h>

struct sona_tail {
  GMutex mu;
  double volume;
  double pregain_db;
  double bands_db[SONA_EQ_BANDS];
  GWeakRef bin;  // the live bin, if any
};

sona_tail* sona_tail_create(void) {
  sona_tail* t = g_new0(sona_tail, 1);
  g_mutex_init(&t->mu);
  g_weak_ref_init(&t->bin, NULL);
  t->volume = 1;
  return t;
}

void sona_tail_destroy(sona_tail* t) {
  if (!t) return;
  g_weak_ref_clear(&t->bin);
  g_mutex_clear(&t->mu);
  g_free(t);
}

static GstElement* child(GstElement* bin, const char* name) {
  return gst_bin_get_by_name(GST_BIN(bin), name);  // ref'd or NULL
}

// apply_locked sets bin's elements to t's settings; with mu held.
static void apply_locked(sona_tail* t, GstElement* bin) {
  GstElement* e;
  if ((e = child(bin, "vol"))) {
    g_object_set(e, "volume", t->volume, NULL);
    gst_object_unref(e);
  }
  if ((e = child(bin, "pregain"))) {
    g_object_set(e, "volume", pow(10, t->pregain_db / 20), NULL);
    gst_object_unref(e);
  }
  if ((e = child(bin, "eq"))) {
    char prop[8];
    for (int i = 0; i < SONA_EQ_BANDS; i++) {
      snprintf(prop, sizeof prop, "band%d", i);
      g_object_set(e, prop, t->bands_db[i], NULL);
    }
    gst_object_unref(e);
  }
}

GstElement* sona_tail_build(sona_tail* t, char* err, size_t errlen) {
  GstElement* bin = gst_bin_new("tail");
  const char* factories[] = {"audioconvert", "volume", "equalizer-10bands", "audioconvert", "audioresample", "volume"};
  const char* names[] = {"conv-in", "pregain", "eq", "conv-out", "resample", "vol"};
  GstElement* prev = NULL;
  GstElement* first = NULL;
  for (int i = 0; i < 6; i++) {
    GstElement* el = gst_element_factory_make(factories[i], names[i]);
    if (!el) {
      if (i == 2) continue;  // no equaliser: play without one
      snprintf(err, errlen, "GStreamer element %s is not installed", factories[i]);
      gst_object_unref(bin);
      return NULL;
    }
    gst_bin_add(GST_BIN(bin), el);
    if (prev && !gst_element_link(prev, el)) {
      snprintf(err, errlen, "cannot link %s ! %s", GST_ELEMENT_NAME(prev), names[i]);
      gst_object_unref(bin);
      return NULL;
    }
    if (!first) first = el;
    prev = el;
  }
  GstPad* sink = gst_element_get_static_pad(first, "sink");
  GstPad* src = gst_element_get_static_pad(prev, "src");
  gst_element_add_pad(bin, gst_ghost_pad_new("sink", sink));
  gst_element_add_pad(bin, gst_ghost_pad_new("src", src));
  gst_object_unref(sink);
  gst_object_unref(src);
  g_mutex_lock(&t->mu);
  apply_locked(t, bin);
  g_weak_ref_set(&t->bin, bin);
  g_mutex_unlock(&t->mu);
  return bin;
}

// update applies t's settings to the live bin; with mu held.
static void update_locked(sona_tail* t) {
  GstElement* bin = g_weak_ref_get(&t->bin);
  if (!bin) return;
  apply_locked(t, bin);
  gst_object_unref(bin);
}

void sona_tail_set_volume(sona_tail* t, double linear) {
  g_mutex_lock(&t->mu);
  t->volume = linear;
  update_locked(t);
  g_mutex_unlock(&t->mu);
}

void sona_tail_set_eq(sona_tail* t, double pregain_db, const double bands_db[SONA_EQ_BANDS]) {
  g_mutex_lock(&t->mu);
  t->pregain_db = pregain_db;
  for (int i = 0; i < SONA_EQ_BANDS; i++) t->bands_db[i] = bands_db[i];
  update_locked(t);
  g_mutex_unlock(&t->mu);
}

int sona_tail_read(sona_tail* t, double* volume, double* pregain, double bands_db[SONA_EQ_BANDS]) {
  GstElement* bin = g_weak_ref_get(&t->bin);
  if (!bin) return 0;
  int ok = 0;
  GstElement* e;
  if ((e = child(bin, "vol"))) {
    g_object_get(e, "volume", volume, NULL);
    gst_object_unref(e);
  }
  if ((e = child(bin, "pregain"))) {
    g_object_get(e, "volume", pregain, NULL);
    gst_object_unref(e);
  }
  if ((e = child(bin, "eq"))) {
    char prop[8];
    for (int i = 0; i < SONA_EQ_BANDS; i++) {
      snprintf(prop, sizeof prop, "band%d", i);
      g_object_get(e, prop, &bands_db[i], NULL);
    }
    gst_object_unref(e);
    ok = 1;
  }
  gst_object_unref(bin);
  return ok;
}
