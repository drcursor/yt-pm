package ui

// msgs.go — all tea.Msg types shared across screens.

// OnboardingDoneMsg is emitted when the onboarding flow completes
// successfully. SessionPath holds the path to the session/cookie file that
// was either provided by the user or captured from the browser.
type OnboardingDoneMsg struct {
	SessionPath string
}

// PlaylistsSelectedMsg is emitted when the user has chosen both a source
// and a target playlist (move / copy operations).
type PlaylistsSelectedMsg struct {
	Action string // "move" or "copy"
	Source Playlist
	Target Playlist
}

// PlaylistSelectedMsg is emitted when the user has chosen a single playlist
// (export / clear operations). Action is one of "export" or "clear".
type PlaylistSelectedMsg struct {
	Playlist Playlist
	Action   string
}

// PlanConfirmedMsg is emitted when the user confirms the plan.
type PlanConfirmedMsg struct {
	// SkipDuplicates is true when the user enabled the skip-duplicates toggle.
	// Only meaningful for move/copy operations.
	SkipDuplicates bool
	// Filename is the output file path chosen by the user. Only set for export.
	Filename string
}

// PlanCancelledMsg is emitted when the user cancels the plan.
type PlanCancelledMsg struct{}

// ProgressUpdateMsg is sent by the caller to update live progress.
type ProgressUpdateMsg struct {
	Current    int
	Total      int
	VideoTitle string
	Failed     int
}

// ProgressDoneMsg is emitted by ProgressModel when operation finishes.
type ProgressDoneMsg struct {
	Successes int
	Failures  int
}

// PlaylistDeleteRequestMsg is emitted when the user confirms deletion of a playlist.
type PlaylistDeleteRequestMsg struct {
	Playlist Playlist
}

// PlaylistScanMsg is emitted when the user requests a scan for broken/unlisted videos.
type PlaylistScanMsg struct {
	Playlist Playlist
}

// PlaylistOpenMsg is emitted when the user presses Enter on a playlist to browse its videos.
type PlaylistOpenMsg struct {
	Playlist Playlist
}

// VideoListDoneMsg is emitted when the user exits the video list view.
type VideoListDoneMsg struct{}

// VideoListRemoveMsg is emitted when the user confirms removing selected videos.
type VideoListRemoveMsg struct {
	Source  Playlist
	Videos  []Video
}

// VideoListSelectTargetMsg is emitted when the user wants to copy/move selected
// videos and needs to pick a target playlist.
type VideoListSelectTargetMsg struct {
	Action string // "move" or "copy"
	Source Playlist
	Videos []Video
}

// VideoOpenMsg is emitted when the user wants to open a video in the browser.
type VideoOpenMsg struct {
	Video Video
}

// BrokenVideosDoneMsg is emitted when the user exits the broken-videos screen.
type BrokenVideosDoneMsg struct{}

// BrokenVideosRemoveMsg is emitted when the user confirms removing all broken/unlisted videos.
type BrokenVideosRemoveMsg struct {
	Playlist Playlist
	Videos   []Video
}

// ReturnToPlaylistsMsg is emitted by ResultModel when the user presses 'r'.
type ReturnToPlaylistsMsg struct{}

// PlaylistStatsMsg is emitted when the user presses 's' on a playlist.
type PlaylistStatsMsg struct {
	Playlist Playlist
}

// StatsDoneMsg is emitted when the user exits the stats screen.
type StatsDoneMsg struct{}
