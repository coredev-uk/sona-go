package radio

import (
	"strings"
	"time"

	mygoi18n "github.com/egoist/mygo/i18n"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// This file is v0.2.0's features/radio/schedule.ts: what is on air on a live
// station, how far through its slot it is, what comes next, and the station's
// catalogue line. Times are Unix milliseconds (the DTOs' unit) and `now` is
// the moment the page reasons with. Nothing here touches the UI, so the
// player's views can take it as it is.

// span is a stretch of air time: a slot, or a run of them.
type span struct{ startMs, endMs int64 }

func spanOf(e services.StationEvent) span { return span{e.StartMs, e.EndMs} }

// onAir is the programme on air at now, if the schedule covers it.
func onAir(schedule []services.StationEvent, now int64) (services.StationEvent, bool) {
	for _, e := range schedule {
		if e.StartMs <= now && now < e.EndMs {
			return e, true
		}
	}
	return services.StationEvent{}, false
}

// progressOf is how far through its slot a programme is at now, 0..1 (0 for
// an empty span).
func progressOf(s span, now int64) float64 {
	length := s.endMs - s.startMs
	if length <= 0 {
		return 0
	}
	return min(1, max(0, float64(now-s.startMs)/float64(length)))
}

// dayPrefix is the weekday before a time when it falls on another day than
// now: "Tue ".
func dayPrefix(t, now int64) string {
	day, today := time.UnixMilli(t), time.UnixMilli(now)
	y1, m1, d1 := day.Date()
	y2, m2, d2 := today.Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return ""
	}
	return weekdayShort(day) + " "
}

// weekdayShort is the abbreviated weekday in the language shown ("Tue").
func weekdayShort(t time.Time) string {
	return mygoi18n.FormatDate(formatLocale(), t, mygoi18n.DateOptions{Skeleton: "E"})
}

func clockAt(ms int64) string { return i18n.FormatClock(time.UnixMilli(ms)) }

// slotLabel is "10:00–12:00", with the weekday when the slot starts on
// another day than now.
func slotLabel(s span, now int64) string {
	return dayPrefix(s.startMs, now) + clockAt(s.startMs) + "–" + clockAt(s.endMs)
}

// startLabel is when a programme starts: "12:00", or "Tue 09:00" on another
// day than now.
func startLabel(e services.StationEvent, now int64) string {
	return dayPrefix(e.StartMs, now) + clockAt(e.StartMs)
}

func same(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// programmeName is what a programme is called beside its station's name: its
// title, or for a slot named after the station itself ("Apple Music Hits" on
// Apple Music Hits) its description; "" when that only repeats the station's
// tagline.
func programmeName(name, tagline string, e services.StationEvent) string {
	if !same(e.Title, name) {
		return e.Title
	}
	if e.Description != "" && !same(e.Description, tagline) {
		return e.Description
	}
	return ""
}

// nextProgramme is the next programme with a name of its own: the first slot
// after the one on air that is called neither after the station nor like the
// programme on air, so a run of the station's own hours is skipped.
func nextProgramme(name string, schedule []services.StationEvent, now int64) (services.StationEvent, bool) {
	current, has := onAir(schedule, now)
	from := now
	if has {
		from = current.EndMs
	}
	for _, e := range schedule {
		if e.StartMs >= from && !same(e.Title, name) && (!has || !same(e.Title, current.Title)) {
			return e, true
		}
	}
	return services.StationEvent{}, false
}

// onAirSpan is the stretch the programme on air holds: its slot together with
// the unbroken run of slots with the same title around it, so the station's
// own hours ("Apple Music Hits", 08:00–10:00, 10:00–12:00) count down to the
// next show with a name of its own, not to an hour nobody named. False when
// nothing is on air, or when the station's own hours run to the end of the
// schedule (there is nothing ahead to count down to).
func onAirSpan(name string, schedule []services.StationEvent, now int64) (span, bool) {
	i := -1
	for j, e := range schedule {
		if e.StartMs <= now && now < e.EndMs {
			i = j
			break
		}
	}
	if i < 0 {
		return span{}, false
	}
	title := schedule[i].Title
	first, last := i, i
	for first > 0 && same(schedule[first-1].Title, title) && schedule[first-1].EndMs == schedule[first].StartMs {
		first--
	}
	for last < len(schedule)-1 && same(schedule[last+1].Title, title) && schedule[last+1].StartMs == schedule[last].EndMs {
		last++
	}
	if last == len(schedule)-1 && same(title, name) {
		return span{}, false
	}
	return span{schedule[first].StartMs, schedule[last].EndMs}, true
}

// stationLine is the station's catalogue line: what it is, who runs it, and
// an episode's date and length ("Episode · Zane Lowe · 57 min").
func stationLine(s services.Station) string {
	var parts []string
	switch s.Kind {
	case services.StationLive:
		parts = append(parts, i18n.T("Live radio"))
	case services.StationBroadcast:
		parts = append(parts, i18n.T("Live broadcast"))
	case services.StationEpisode:
		parts = append(parts, i18n.T("Episode"))
	default:
		parts = append(parts, i18n.T("Station"))
	}
	if s.Kind == services.StationEpisode && s.ShowName != "" {
		parts = append(parts, s.ShowName)
	} else if s.Provider != "" {
		parts = append(parts, s.Provider)
	}
	if s.AirTimeMs > 0 {
		parts = append(parts, i18n.FormatDate(time.UnixMilli(s.AirTimeMs), i18n.DateMedium))
	}
	if s.DurationMs > 0 {
		parts = append(parts, minutesText(int(roundDiv(s.DurationMs, 60_000))))
	}
	return strings.Join(parts, " · ")
}

func roundDiv(a, b int64) int64 { return (a + b/2) / b }
