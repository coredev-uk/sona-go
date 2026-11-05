package kit

// Pending is how many covers are queued or being decoded by the workers
// (asked for, not yet published). Main thread only. Tests wait for it to
// reach zero before they take a picture (uitest.Session.Settle).
func (l *ArtLoader) Pending() int { return len(l.live) }
