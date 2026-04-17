package youtube

// Playlist represents a YouTube playlist owned by the authenticated user.
type Playlist struct {
	ID    string
	Title string
	Count int
}

// Video represents a single video entry within a playlist.
type Video struct {
	ID         string
	SetVideoID string // required for removal — NEVER remove without this
	Title      string
	Channel    string
	Duration   string // human-readable e.g. "12:34"
	PlaylistID string

	// Unavailable is true for private, deleted, or region-blocked videos.
	// These entries cannot be played but may still appear in the playlist.
	Unavailable       bool
	UnavailableReason string
}

// Session holds the authentication state needed for API calls.
type Session struct {
	Cookies     map[string]string // cookie name -> value
	Origin      string            // e.g. "https://www.youtube.com"
	VisitorData string            // X-Goog-Visitor-Id, obtained from page load
}
