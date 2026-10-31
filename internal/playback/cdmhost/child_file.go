//go:build linux || (darwin && cgo)

package cdmhost

import (
	"errors"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/gst"
)

// Library uploads are plain files (MP3, AAC, ALAC) the parent streams to
// the child as they download (LoadFile, FeedFile, EndOfFile). They play
// through their own gst.FilePipeline, which decodes with decodebin and
// shares the output processing (equaliser, volume) with the CDM pipeline;
// one of the two plays at a time. A Load without an init segment while a
// file is current seeks within it.

// output is what the transport controls drive: the CDM pipeline or the
// file pipeline, whichever plays the current track.
type output interface {
	Play() error
	Pause() error
	SetVolume(v float64)
	SetEqualizer(pregain float64, bands [gst.EQBands]float64)
}

// outLocked returns the current output, or nil. Call with mu held.
func (s *server) outLocked() output {
	if s.file != nil {
		return s.file
	}
	if s.pipe != nil {
		return s.pipe
	}
	return nil
}

// fileState is the current upload's progress; guarded by server.mu.
type fileState struct {
	track     uint64        // generation its bytes are fed under (LoadFile's)
	prerolled bool          // the pipeline reached PAUSED: seeks are possible
	seek      time.Duration // seek to make once prerolled; -1 for none
	complete  bool          // every byte was fed
	durSent   bool          // the parent was told the duration
}

// fileLowWater is how many bytes past the demuxer's read position count
// as buffered audio for the underrun detector.
const fileLowWater = 64 << 10

// loadFile makes r.Track a plain file of r.Size bytes played from r.Start.
func (s *server) loadFile(r request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeFileLocked()
	s.next = nextTrack{}
	if s.pipe != nil { // silence the CDM pipeline; the next Load reuses it
		s.pipe.Stop(r.Track)
		_ = s.pipe.Pause()
	}
	f, err := gst.NewFile(gst.Options{Sink: s.sink, Device: s.device}, r.Size)
	if err != nil {
		return err
	}
	f.SetEqualizer(s.eq.Pregain, s.eq.Gains)
	s.file = f
	s.fs = fileState{track: r.Track, seek: -1}
	if r.Start > 0 {
		s.fs.seek = r.Start
	}
	s.track, s.start, s.init, s.gain = r.Track, r.Start, nil, r.Gain
	s.started, s.havePrev, s.ended, s.buffering = false, false, false, false
	s.applyVolumeLocked()
	go s.watchFile(f)
	return f.Pause() // prerolls; watchFile seeks and plays from there
}

// seekFileLocked handles a Load without init segment while a file plays:
// a seek within it as generation r.Track. Call with mu held.
func (s *server) seekFileLocked(r request) error {
	s.track, s.start = r.Track, r.Start
	s.started, s.havePrev, s.ended, s.buffering = false, false, false, false
	if !s.fs.prerolled {
		s.fs.seek = r.Start
		return nil
	}
	if err := s.file.Seek(r.Start); err != nil {
		return err
	}
	if s.playing {
		return s.file.Play()
	}
	return nil
}

func (s *server) feedFile(r request) error {
	s.mu.Lock()
	f, track := s.file, s.fs.track
	s.mu.Unlock()
	if f == nil || r.Track != track {
		return ErrStale
	}
	if err := f.Feed(r.Offset, r.Data); err != nil {
		if errors.Is(err, gst.ErrClosed) {
			return ErrStale
		}
		return err
	}
	return nil
}

func (s *server) endFile(r request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil || r.Track != s.fs.track {
		return ErrStale
	}
	s.fs.complete = true
	s.file.Complete()
	return nil
}

// closeFileLocked drops the file pipeline. Call with mu held.
func (s *server) closeFileLocked() {
	if s.file != nil {
		go s.file.Close()
		s.file = nil
	}
}

// fileQueuedLocked estimates the audio buffered ahead in a file for the
// underrun detector: plenty once it is complete or well ahead of the
// demuxer, none when the demuxer has caught up with the download.
func (s *server) fileQueuedLocked() time.Duration {
	if s.fs.complete {
		return resumeBuffered
	}
	have, _, pos := s.file.Level()
	if have-pos > fileLowWater {
		return resumeBuffered
	}
	return 0
}

// watchFile forwards file pipeline events for f's lifetime.
func (s *server) watchFile(f *gst.FilePipeline) {
	for ev := range f.Events() {
		switch ev.Kind {
		case gst.EventAsyncDone:
			s.mu.Lock()
			if s.file == f && !s.fs.prerolled {
				s.fs.prerolled = true
				if pos := s.fs.seek; pos >= 0 {
					s.fs.seek = -1
					if err := f.Seek(pos); err != nil {
						s.log.Warn("cdmhost: file seek", "err", err)
					}
				}
				if s.playing && !s.buffering {
					_ = f.Play()
				}
			}
			s.mu.Unlock()
		case gst.EventEOS:
			s.mu.Lock()
			// The source never ends before every byte arrived, so an end
			// with bytes still missing is the demuxer's own (a seek past
			// the end): the track has ended either way.
			track, ended := s.track, s.file == f
			pos, _ := f.Position()
			s.mu.Unlock()
			if ended {
				s.emit(Event{Kind: EventEnded, Track: track, Position: pos, At: time.Now()})
			}
		case gst.EventError:
			s.log.Error("cdmhost: file pipeline error", "err", ev.Err)
			s.mu.Lock()
			track, current := s.track, s.file == f
			if current {
				s.closeFileLocked()
			}
			s.mu.Unlock()
			if current {
				s.emit(Event{Kind: EventError, Track: track, Err: ev.Err.Error()})
			}
		case gst.EventWarning:
			s.log.Warn("cdmhost: file pipeline warning", "err", ev.Err)
		}
	}
}
