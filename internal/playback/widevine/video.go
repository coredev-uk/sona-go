//go:build linux || (darwin && cgo)

package widevine

/*
#include <stdlib.h>
#include "shim.h"
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unsafe"
)

// The CDM's own video decoder. Apple issues a software CDM its music video
// keys at level SW_SECURE_DECODE: the CDM installs them, but its Decrypt
// refuses them (ErrNoKey), so only DecryptAndDecodeFrame, which decrypts
// and decodes inside the CDM and hands back decoded pictures, can use
// them. Sona uses it for video only; audio keeps going through Decrypt.

// VideoCodec is cdm::VideoCodec.
type VideoCodec uint32

// CodecH264 is the only codec Sona feeds the CDM's decoder.
const CodecH264 VideoCodec = 2

// VideoProfile is cdm::VideoCodecProfile.
type VideoProfile uint32

// H.264 profiles (cdm::VideoCodecProfile).
const (
	ProfileUnknown               VideoProfile = 0
	ProfileH264Baseline          VideoProfile = 2
	ProfileH264Main              VideoProfile = 3
	ProfileH264Extended          VideoProfile = 4
	ProfileH264High              VideoProfile = 5
	ProfileH264High10            VideoProfile = 6
	ProfileH264High422           VideoProfile = 7
	ProfileH264High444Predictive VideoProfile = 8
)

// H264Profile maps an H.264 profile_idc (the avcC AVCProfileIndication)
// to the CDM's profile; ProfileUnknown for one it does not name.
func H264Profile(idc uint8) VideoProfile {
	switch idc {
	case 66:
		return ProfileH264Baseline
	case 77:
		return ProfileH264Main
	case 88:
		return ProfileH264Extended
	case 100:
		return ProfileH264High
	case 110:
		return ProfileH264High10
	case 122:
		return ProfileH264High422
	case 244:
		return ProfileH264High444Predictive
	}
	return ProfileUnknown
}

// VideoConfig configures the CDM's video decoder (cdm::VideoDecoderConfig_2;
// the output is always I420).
type VideoConfig struct {
	Codec   VideoCodec
	Profile VideoProfile
	// Width and Height are the coded picture size (the sample entry's).
	Width, Height int
	// ExtraData is the codec's out-of-band configuration; nil for H.264
	// fed as Annex B with in-band parameter sets, as Chromium feeds it.
	ExtraData []byte
	CBCS      bool // 'cbcs' encryption; false is 'cenc'
}

// VideoFrame is one decoded picture.
type VideoFrame struct {
	Width, Height int
	Timestamp     time.Duration // the presentation time its access unit carried
	// Data holds the planes packed as I420: Width×Height luma, then the
	// two chroma planes of ⌈Width/2⌉×⌈Height/2⌉ (U, then V).
	Data []byte
}

// Errors returned by the video decoder.
var (
	// ErrNeedMoreData: the decoder has no picture ready yet (or, when
	// draining, none left).
	ErrNeedMoreData = errors.New("widevine: decoder needs more data")
	// ErrDecode: the CDM failed to decode an access unit.
	ErrDecode = errors.New("widevine: video decode failed")
	// ErrVideoInit: the CDM's decoder refused the configuration.
	ErrVideoInit = errors.New("widevine: the CDM's video decoder refused the configuration")
)

// InitializeVideoDecoder (re)initialises the CDM's video decoder with cfg,
// releasing a decoder initialised before.
func (c *CDM) InitializeVideoDecoder(ctx context.Context, cfg VideoConfig) error {
	var st C.int
	err := c.do(ctx, func() {
		C.sona_cdm_video_deinit(c.ptr)
		var extra *C.uint8_t
		if len(cfg.ExtraData) > 0 {
			extra = bytesPtr(cfg.ExtraData)
		}
		var cbcs C.int
		if cfg.CBCS {
			cbcs = 1
		}
		st = C.sona_cdm_video_init(c.ptr, C.uint32_t(cfg.Codec), C.uint32_t(cfg.Profile), C.int32_t(cfg.Width), C.int32_t(cfg.Height),
			extra, C.uint32_t(len(cfg.ExtraData)), cbcs)
	})
	if err != nil {
		return err
	}
	if st != 0 {
		return fmt.Errorf("%w (status %d)", ErrVideoInit, int(st))
	}
	return nil
}

// ResetVideoDecoder drops every picture the decoder holds (before a seek).
func (c *CDM) ResetVideoDecoder(ctx context.Context) error {
	return c.do(ctx, func() { C.sona_cdm_video_reset(c.ptr) })
}

// DeinitializeVideoDecoder releases the video decoder.
func (c *CDM) DeinitializeVideoDecoder(ctx context.Context) error {
	return c.do(ctx, func() { C.sona_cdm_video_deinit(c.ptr) })
}

// DecodeVideo decrypts and decodes one access unit b (Buffer fields as for
// Decrypt; Out is unused, and a nil KeyID feeds a clear unit). A nil b
// drains the decoder at the end of the stream: call it until
// ErrNeedMoreData. It returns the next picture, ErrNeedMoreData when none
// is ready, ErrNoKey, ErrDecrypt or ErrDecode.
func (c *CDM) DecodeVideo(b *Buffer) (VideoFrame, error) {
	var (
		f  VideoFrame
		st C.int
	)
	err := c.do(context.Background(), func() {
		var (
			out                    C.sona_video_frame
			kid, iv, data          *C.uint8_t
			kidLen, ivLen, dataLen C.uint32_t
			subs                   *C.uint32_t
			nsub                   C.uint32_t
			ts                     C.int64_t
			cbcs                   C.int
			crypt, skip            C.uint32_t
		)
		if b != nil && len(b.Data) > 0 {
			data, dataLen, ts = bytesPtr(b.Data), C.uint32_t(len(b.Data)), C.int64_t(b.Timestamp.Microseconds())
			if len(b.KeyID) > 0 && len(b.IV) > 0 {
				kid, kidLen, iv, ivLen = bytesPtr(b.KeyID), C.uint32_t(len(b.KeyID)), bytesPtr(b.IV), C.uint32_t(len(b.IV))
				if len(b.Subsamples) > 0 {
					subs, nsub = (*C.uint32_t)(unsafe.Pointer(&b.Subsamples[0])), C.uint32_t(len(b.Subsamples))
				}
				if b.CBCS {
					cbcs = 1
				}
				crypt, skip = C.uint32_t(b.CryptBlocks), C.uint32_t(b.SkipBlocks)
			}
		}
		st = C.sona_cdm_video_decode(c.ptr, kid, kidLen, iv, ivLen, data, dataLen, subs, nsub, ts, cbcs, crypt, skip, &out)
		if st == 0 {
			f = VideoFrame{
				Width: int(out.width), Height: int(out.height),
				Timestamp: time.Duration(out.timestamp_us) * time.Microsecond,
				Data:      C.GoBytes(unsafe.Pointer(out.data), C.int(out.size)),
			}
			C.sona_video_frame_free(&out)
		}
	})
	if err != nil {
		return VideoFrame{}, err
	}
	switch st {
	case 0:
		return f, nil
	case 1:
		return VideoFrame{}, ErrNeedMoreData
	case 2:
		return VideoFrame{}, ErrNoKey
	case 4:
		return VideoFrame{}, ErrDecrypt
	case 5:
		return VideoFrame{}, ErrDecode
	default:
		return VideoFrame{}, fmt.Errorf("%w: status %d", ErrDecode, int(st))
	}
}
