package ui

// Playlist is a stub mirroring the real type from the youtube package.
// Replace with the real import once that package is available.
type Playlist struct {
	ID    string
	Title string
	Count int
}

// Video is a stub mirroring the real type from the youtube package.
type Video struct {
	ID                string
	SetVideoID        string
	Title             string
	Channel           string
	Duration          string
	PlaylistID        string
	Unavailable       bool
	UnavailableReason string
}
