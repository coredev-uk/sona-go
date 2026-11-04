//go:build (linux || darwin) && cgo

package media

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0 gstreamer-video-1.0
#include <gst/gst.h>
#include <gst/app/gstappsink.h>
#include <gst/video/video.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
	GstElement *pipe;
	GstElement *sink;
	GstSample *sample; // pulled by motion_next, copied out and released by motion_copy
	gsize stride;
	int w, h;
} sona_motion;

static void motion_init(void) {
	static gsize once = 0;
	if (g_once_init_enter(&once)) { gst_init(NULL, NULL); g_once_init_leave(&once, 1); }
}

// motion_has reports whether the element factory exists in the registry.
static int motion_has(const char *name) {
	motion_init();
	GstElementFactory *f = gst_element_factory_find(name);
	if (f == NULL) return 0;
	gst_object_unref(f);
	return 1;
}

// motion_open builds file ! decodebin ! videoconvert ! RGBA ! appsink. The
// decoder is decodebin's choice by rank, so a hardware decoder (VA-API,
// VideoToolbox) is used where its plugin is there, and the picture is
// converted to system memory for the CPU copy into a MyGo bitmap.
static sona_motion *motion_open(const char *path, char **err) {
	motion_init();
	GError *e = NULL;
	GstElement *p = gst_parse_launch(
		"filesrc name=src ! decodebin ! videoconvert ! video/x-raw,format=RGBA ! "
		"appsink name=sink sync=false max-buffers=2 drop=false", &e);
	if (e != NULL) {
		*err = g_strdup(e->message);
		g_error_free(e);
		if (p != NULL) gst_object_unref(p);
		return NULL;
	}
	// The path is set as a property, never parsed as part of the description.
	GstElement *src = gst_bin_get_by_name(GST_BIN(p), "src");
	g_object_set(src, "location", path, NULL);
	gst_object_unref(src);
	sona_motion *m = g_new0(sona_motion, 1);
	m->pipe = p;
	m->sink = gst_bin_get_by_name(GST_BIN(p), "sink");
	if (gst_element_set_state(p, GST_STATE_PLAYING) == GST_STATE_CHANGE_FAILURE) {
		*err = g_strdup("the pipeline would not start");
		gst_element_set_state(p, GST_STATE_NULL);
		gst_object_unref(m->sink);
		gst_object_unref(p);
		g_free(m);
		return NULL;
	}
	return m;
}

// motion_bus_error returns the first error message on the pipeline's bus as
// a new string, or NULL.
static char *motion_bus_error(sona_motion *m) {
	GstBus *bus = gst_element_get_bus(m->pipe);
	GstMessage *msg = gst_bus_pop_filtered(bus, GST_MESSAGE_ERROR);
	gst_object_unref(bus);
	if (msg == NULL) return NULL;
	GError *e = NULL;
	gst_message_parse_error(msg, &e, NULL);
	char *out = g_strdup(e != NULL ? e->message : "decode error");
	if (e != NULL) g_error_free(e);
	gst_message_unref(msg);
	return out;
}

// motion_next pulls the next frame and keeps it for motion_copy. It returns
// the frame's duration in ns (the default when the buffer has none) and its
// size in *w and *h; 0 at the end, -1 on an error whose words are in *err (a
// new string).
static gint64 motion_next(sona_motion *m, int *w, int *h, char **err) {
	if (m->sample != NULL) { // the last one was never copied
		gst_sample_unref(m->sample);
		m->sample = NULL;
	}
	// Wait in slices, so a pipeline error (a missing or damaged file) shows at
	// once, not after the whole wait.
	GstSample *s = NULL;
	for (int i = 0; i < 50 && s == NULL; i++) {
		s = gst_app_sink_try_pull_sample(GST_APP_SINK(m->sink), 100 * GST_MSECOND);
		if (s != NULL) break;
		*err = motion_bus_error(m);
		if (*err != NULL) return -1;
		if (gst_app_sink_is_eos(GST_APP_SINK(m->sink))) return 0;
	}
	if (s == NULL) {
		*err = g_strdup("no picture within 5 s");
		return -1;
	}
	GstVideoInfo info;
	if (!gst_video_info_from_caps(&info, gst_sample_get_caps(s))) {
		gst_sample_unref(s);
		*err = g_strdup("unreadable picture format");
		return -1;
	}
	m->sample = s;
	m->w = GST_VIDEO_INFO_WIDTH(&info);
	m->h = GST_VIDEO_INFO_HEIGHT(&info);
	m->stride = GST_VIDEO_INFO_PLANE_STRIDE(&info, 0);
	*w = m->w;
	*h = m->h;
	GstBuffer *b = gst_sample_get_buffer(s);
	gint64 d = GST_BUFFER_DURATION_IS_VALID(b) ? (gint64)GST_BUFFER_DURATION(b) : 0;
	return d > 0 ? d : 33333333; // 0 would read as the end
}

// motion_copy writes the pulled frame's rows, tightly packed (4*w bytes
// each), into dst, which holds 4*w*h bytes, and releases the frame.
static int motion_copy(sona_motion *m, void *dst) {
	if (m->sample == NULL) return 0;
	GstBuffer *b = gst_sample_get_buffer(m->sample);
	GstMapInfo mi;
	if (!gst_buffer_map(b, &mi, GST_MAP_READ)) {
		gst_sample_unref(m->sample);
		m->sample = NULL;
		return 0;
	}
	gsize row = (gsize)m->w * 4;
	for (int y = 0; y < m->h; y++) memcpy((guint8 *)dst + y * row, mi.data + y * m->stride, row);
	gst_buffer_unmap(b, &mi);
	gst_sample_unref(m->sample);
	m->sample = NULL;
	return 1;
}

static gboolean motion_rewind(sona_motion *m) {
	return gst_element_seek_simple(m->pipe, GST_FORMAT_TIME, GST_SEEK_FLAG_FLUSH | GST_SEEK_FLAG_KEY_UNIT, 0);
}

static void motion_close(sona_motion *m) {
	gst_element_set_state(m->pipe, GST_STATE_NULL);
	if (m->sample != NULL) gst_sample_unref(m->sample);
	gst_object_unref(m->sink);
	gst_object_unref(m->pipe);
	g_free(m);
}
*/
import "C"

import (
	"errors"
	"image"
	"io"
	"sync"
	"time"
	"unsafe"

	"github.com/egoist/mygo/ui"
)

// Supported is whether this build decodes motion artwork: GStreamer through
// cgo (Linux and macOS). Capabilities().MotionArtwork reports it.
const Supported = true

// decoders are the element factories that decode each codec, by name: the
// software ones (libav, OpenH264) and the hardware ones (VA, VideoToolbox,
// NVIDIA, Direct3D, V4L2, Vulkan, Intel, AMD). decodebin picks among them by
// rank.
var decoders = map[string][]string{
	"avc": {
		"avdec_h264", "openh264dec", "vah264dec", "vah264lpdec", "vtdec", "vtdec_hw", "nvh264dec", "nvh264sldec",
		"d3d11h264dec", "d3d12h264dec", "v4l2slh264dec", "v4l2h264dec", "vulkanh264dec", "qsvh264dec", "msdkh264dec", "amfh264dec",
	},
	"hevc": {
		"avdec_h265", "vah265dec", "vah265lpdec", "vtdec", "vtdec_hw", "nvh265dec", "nvh265sldec",
		"d3d11h265dec", "d3d12h265dec", "v4l2slh265dec", "v4l2h265dec", "vulkanh265dec", "qsvh265dec", "msdkh265dec", "amfh265dec",
	},
}

var (
	codecOnce sync.Once
	codecName string
)

// Codec is the codec to ask the motion cache for: "avc" (H.264, which
// Apple always offers and is cheapest to decode) wherever GStreamer has a
// decoder for it, "hevc" where it has one for HEVC only, "" where it has
// neither or lacks the MP4 demuxer or the parsers (no motion artwork).
func Codec() string {
	codecOnce.Do(func() {
		for _, need := range []string{"qtdemux", "appsink", "decodebin", "videoconvert"} {
			if !has(need) {
				return
			}
		}
		for _, codec := range []string{"avc", "hevc"} {
			parser := map[string]string{"avc": "h264parse", "hevc": "h265parse"}[codec]
			if !has(parser) {
				continue
			}
			for _, name := range decoders[codec] {
				if has(name) {
					codecName = codec
					return
				}
			}
		}
	})
	return codecName
}

func has(factory string) bool {
	name := C.CString(factory)
	defer C.free(unsafe.Pointer(name))
	return C.motion_has(name) != 0
}

// gstDecoder is a ui.VideoDecoder of a motion artwork file, decoded by
// GStreamer in this process.
type gstDecoder struct {
	mu     sync.Mutex // Close against a Next on another goroutine
	m      *C.sona_motion
	img    *image.RGBA
	closed bool
}

// NewMotionDecoder opens the motion artwork file at path (the motion cache's
// plain MP4) for a ui.VideoPlayer. The decoder is chosen by GStreamer's
// ranks, hardware ones included. The player calls Next from one goroutine and
// closes the decoder when it is closed.
func NewMotionDecoder(path string) (ui.VideoDecoder, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var cerr *C.char
	m := C.motion_open(cpath, &cerr)
	if m == nil {
		defer C.g_free(C.gpointer(cerr))
		return nil, errors.New(C.GoString(cerr))
	}
	return &gstDecoder{m: m}, nil
}

func (d *gstDecoder) Next() (ui.VideoFrame, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ui.VideoFrame{}, io.ErrClosedPipe
	}
	var w, h C.int
	var cerr *C.char
	dur := C.motion_next(d.m, &w, &h, &cerr)
	switch {
	case dur == 0:
		return ui.VideoFrame{}, io.EOF
	case dur < 0:
		defer C.g_free(C.gpointer(cerr))
		return ui.VideoFrame{}, errors.New("motion: " + C.GoString(cerr))
	}
	if d.img == nil || d.img.Rect.Dx() != int(w) || d.img.Rect.Dy() != int(h) {
		d.img = image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	}
	// Straight from GStreamer's buffer into the image: one copy.
	if C.motion_copy(d.m, unsafe.Pointer(&d.img.Pix[0])) == 0 {
		return ui.VideoFrame{}, errors.New("motion: unreadable picture")
	}
	return ui.VideoFrame{Image: d.img, Duration: time.Duration(dur)}, nil
}

func (d *gstDecoder) Rewind() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return io.ErrClosedPipe
	}
	if C.motion_rewind(d.m) == 0 {
		return errors.New("motion: rewind failed")
	}
	return nil
}

func (d *gstDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	C.motion_close(d.m)
	return nil
}
