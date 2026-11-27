package services

// EventNotificationsChanged carries Notifications whenever one is added,
// read, dismissed or cleared, and after sign-out drops the account's.
const EventNotificationsChanged = "notifications:changed"

// OpenSettings is the OpenTarget.Kind of a Settings section (ID names the
// section, as in /settings?section=addons).
const OpenSettings = "settings"

// Notification kinds. The UI words each one; Go keeps only the names.
const (
	// NoticeReleaseOut: an album the person asked to be reminded of is out.
	// Subject is the album, Artist its artist, Target the album.
	NoticeReleaseOut = "release-out"
	// NoticeNewReleases: the artist tracker found new releases. Count is how
	// many; when it is 1, Subject is the release, Artist its artist, Variant
	// its kind (ReleaseAlbum, ReleaseEP, ReleaseSingle) and Target the album,
	// otherwise Target is the page of the tracker's releases (OpenNewReleases).
	NoticeNewReleases = "new-releases"
	// NoticeAddonOff: an add-on stopped on its own too many times in a row
	// and was switched off. Subject is its name, Target Settings → Add-ons.
	NoticeAddonOff = "addon-off"
)

// Notification is one entry of the notification centre.
type Notification struct {
	ID        int64      `json:"id"`
	Kind      string     `json:"kind"`
	Subject   string     `json:"subject"`
	Artist    string     `json:"artist"`
	Variant   string     `json:"variant"`
	Count     int        `json:"count"`
	Art       Art        `json:"art"`
	Target    OpenTarget `json:"target"`
	CreatedAt int64      `json:"createdAt"` // Unix ms
	Read      bool       `json:"read"`
}

// Notifications is the centre's list, newest first, with how many are
// unread.
type Notifications struct {
	Items  []Notification `json:"items"`
	Unread int            `json:"unread"`
}
