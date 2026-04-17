package ui

// All user-facing strings. No inline string literals are used in other UI files.

// ── Onboarding ───────────────────────────────────────────────────────────────

const (
	OnboardingWelcomeTitle = "Welcome to yt-pm"
	OnboardingWelcomeBody  = `YouTube (TUI) Playlist Manager lets you move, copy, clear,
and export YouTube playlists without the YouTube API — it
operates directly through your authenticated browser session.

Press Enter to continue.`

	OnboardingChooseMethodPrompt = "How would you like to authenticate?"
	OnboardingChoicePaste        = "(1) Paste cookie content  [default]"
	OnboardingChoiceBrowser      = "(2) Open browser, then paste cookie content"
	OnboardingChoiceFile         = "(3) Provide a cookie file path"
	OnboardingChoiceHint         = "Press 1–3 or Enter to choose."
	OnboardingCookiePathPrompt   = "Cookie file path: "
	OnboardingCookiePathHint     = "Enter the full path to your Netscape/curl cookie file, then press Enter."
	OnboardingPasteHint   = "Open YouTube in your browser, export your cookies as a Netscape-format\ncookie file, copy the content, and paste it into the box below."
	OnboardingBrowserHint = "A browser window has opened. Log in to YouTube, export your cookies\nas a Netscape-format cookie file, copy the content, and paste it below."
	OnboardingPastePlaceholder   = "Paste cookie content here…"
	OnboardingConnecting         = "Connecting…"
	OnboardingSuccess            = "Connected! Press Enter to continue."
	OnboardingError              = "Connection failed: "
	OnboardingRetryHint          = "Press Esc to go back or q to quit."
)

// ── Video list ────────────────────────────────────────────────────────────────

const (
	VideoListTitleFmt   = "Videos in: %s  (%d)"
	VideoListEmptyMsg   = "(no videos)"
	VideoListHint       = "↑/↓ navigate  •  space select  •  a all  •  c copy  •  m move  •  x remove  •  o open  •  esc back"
	VideoListSelHint    = "%d selected  •  c copy  •  m move  •  x remove  •  a clear  •  esc back"
	VideoListLineFmt    = "%s  %-40s  %-22s  %s"
	VideoListCheckOn    = "●"
	VideoListCheckOff   = "○"
	VideoListCursorOn   = "▸"
	VideoListCursorOff  = " "
)

// ── Broken videos ─────────────────────────────────────────────────────────────

const (
	BrokenVideosTitle      = "Broken / unlisted videos in: %s"
	BrokenVideosNone       = "No broken or unlisted videos found."
	BrokenVideosCount      = "%d broken/unlisted video(s) found"
	BrokenVideoLineFmt     = "  %s  [%s]  %s"
	BrokenVideosRemoveHint = "↑/↓ navigate  •  o open  •  r remove all  •  tab filter  •  q/esc back"
	BrokenVideosBackHint   = "↑/↓ navigate  •  o open  •  tab filter  •  q/esc back"
	BrokenVideosConfirm    = "Remove all %d broken/unlisted videos? (y/n)"
	BrokenVideosFilterHint = "tab: toggle view  [%s]"
	BrokenVideosFilterAll  = "all"
	BrokenVideosFilterBroken   = "broken"
	BrokenVideosFilterUnlisted = "unlisted"
)

// ── Playlist list ─────────────────────────────────────────────────────────────

const (
	PlListDeleteHint      = "d: delete selected playlist"
	PlListDeleteConfirm   = "Type the playlist name to confirm deletion, then press Enter. Esc to cancel."
	PlListDeleteWarning   = "⚠  This will permanently delete the playlist from your YouTube account."
	PlListDeleteMismatch  = "Name does not match — try again."
	PlListDeletedFmt      = "Playlist %q deleted."
)

// ── Playlists ─────────────────────────────────────────────────────────────────

const (
	PlaylistsPaneSource    = "Source playlist"
	PlaylistsPaneTarget    = "Target playlist"
	PlaylistsSelectSource  = "Select source playlist  (↑↓ navigate, Enter select)"
	PlaylistsSelectTarget  = "Select target playlist  (↑↓ navigate, Enter select, Esc back)"
	PlaylistsSelectSingle  = "Select playlist  (↑↓ navigate, Enter select)"
	PlaylistsQuitHint      = "q quit"
	PlaylistsItemFmt       = "%s  [%d videos]"
	PlaylistsEmptyList     = "(no playlists)"
)

// ── Plan ──────────────────────────────────────────────────────────────────────

const (
	PlanHeaderFmt         = "Operation: %s"
	PlanSubHeaderFmt         = "%d videos will be processed"
	PlanSubHeaderFmtFiltered = "%d of %d videos will be processed (%d already in target, skipped)"
	PlanSkipDuplicatesHint   = "d: skip-duplicates [%s]"
	PlanSkipDuplicatesOn     = "ON"
	PlanSkipDuplicatesOff    = "OFF"
	PlanSourceFmt         = "From: %s"
	PlanTargetFmt         = "To:   %s"
	PlanVideoListHeader   = "Videos"
	PlanVideoLineFmt      = "  %s  —  %s  (%s)"
	PlanConfirmHint          = "Press y to confirm, n or q to cancel."
	PlanClearConfirmHint     = "Type the playlist name below to confirm, then press Enter. Press Esc to cancel."
	PlanClearInputPrompt     = "Playlist name: "
	PlanClearMismatch        = "Name does not match — try again."
	PlanExportFilenameHint   = "Output file (edit if needed, then press Enter to export). Esc to cancel."
	PlanExportFilenamePrompt = "File name: "
	PlanExportFilenameEmpty  = "File name cannot be empty."
	PlanScrollHint        = "↑↓ or j/k to scroll"
	PlanExcludedWarning   = "⚠  %d unavailable video(s) excluded (run 'find broken' to review)"
	PlanOpMove            = "MOVE"
	PlanOpCopy            = "COPY"
	PlanOpClear           = "CLEAR"
	PlanOpRemove          = "REMOVE"
	PlanOpExport          = "EXPORT"
)

// ── Stats ─────────────────────────────────────────────────────────────────────
const (
	StatsTitleFmt  = "Stats: %s"
	StatsLoading   = "Computing stats…"
	StatsVideos    = "Videos:    %d"
	StatsTotalTime = "Total:     %s"
	StatsAvgTime   = "Average:   %s"
	StatsLongest   = "Longest:   %s  (%s)"
	StatsShortest  = "Shortest:  %s  (%s)"
	StatsChannels  = "Channels:  %d unique"
	StatsHint      = "Press any key to go back"
)

// ── Progress ──────────────────────────────────────────────────────────────────

const (
	ProgressTitle      = "Processing…"
	ProgressCountFmt   = "Video %d of %d"
	ProgressCurrentFmt = "Current: %s"
	ProgressFailedFmt  = "Failures so far: %d"
	ProgressDone       = "Done!"
)

// ── Result ────────────────────────────────────────────────────────────────────

const (
	ResultTitle      = "Operation complete"
	ResultSuccessFmt = "Succeeded: %d"
	ResultFailedFmt  = "Failed:    %d"
	ResultLogFmt     = "Log file:  %s"
	ResultQuitHint   = "Press Enter or r to return to playlists  •  q to quit"
)
