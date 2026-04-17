package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/eduardobalsa/yt-pm/ui"
	"github.com/eduardobalsa/yt-pm/youtube"
)

// openBrowser opens url in the default system browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return cmd.Start()
}

// browserOpenCmd returns a tea.Cmd that opens YouTube in the browser.
// It is used as the ConnectFunc for the browser onboarding method.
// When path is non-empty the call is a no-op (file method reuse).
func browserOpenCmd(path string) tea.Cmd {
	if path != "" {
		return nil
	}
	return func() tea.Msg {
		_ = openBrowser("https://www.youtube.com")
		return nil // no message — user fills in the cookie path themselves
	}
}

const sessionFile = "session.json"

// ── App screens ──────────────────────────────────────────────────────────────

type screen int

const (
	screenOnboarding screen = iota
	screenPlaylistList
	screenVideoList
	screenBrokenVideos
	screenLoading // blocking spinner shown while videos are being fetched
	screenPlan
	screenProgress
	screenResult
	screenStats
)

// ── Messages ─────────────────────────────────────────────────────────────────

type playlistsLoadedMsg struct{ playlists []youtube.Playlist }
type playlistsLoadErrMsg struct{ err error }
type videosLoadedMsg struct {
	videos       []youtube.Video
	skippedCount int
}
type sourceVideosLoadedMsg struct {
	videos       []youtube.Video
	skippedCount int
}
type destVideosLoadedMsg struct{ videos []youtube.Video } // dest skipped count not needed
type videosLoadErrMsg struct{ err error }
type playlistDeletedMsg struct{ playlistID string }
type playlistDeleteErrMsg struct{ err error }
type brokenVideosLoadedMsg struct {
	playlist       youtube.Playlist
	videos         []youtube.Video
	truncatedCount int
}
type brokenVideosScanErrMsg struct{ err error }
type videoListLoadedMsg struct {
	playlist youtube.Playlist
	videos   []youtube.Video
}
type exportDoneMsg struct{ path string }
type fatalErrMsg struct{ err error }
type statsVideosLoadedMsg struct {
	playlist ui.Playlist
	videos   []youtube.Video
}

// ── App model ─────────────────────────────────────────────────────────────────

type appModel struct {
	screen    screen
	ytClient  *youtube.Client
	logger    *log.Logger
	logPath   string
	errMsg    string
	termW     int
	termH     int

	// sub-models
	onboarding   ui.OnboardingModel
	playlistList ui.PlaylistListModel
	videoList    ui.VideoListModel
	brokenVideos ui.BrokenVideosModel
	plan         ui.PlanModel
	progress     ui.ProgressModel
	result       ui.ResultModel
	statsModel   ui.StatsModel

	// operation state
	action       string
	source       ui.Playlist
	target       ui.Playlist
	videos       []ui.Video
	allVideos    []youtube.Video // source videos for current operation
	destVideos   []youtube.Video // dest videos for move/copy (used for skip-duplicates)
	skippedCount int             // unavailable videos excluded from the source list
	sourceLoaded bool            // parallel loading tracker
	destLoaded   bool

	// re-auth state (session expired mid-operation or on load)
	reAuthMode   bool
	authRetried  bool        // true after one auto-refresh attempt, prevents loops
	pendingRetry retryTarget // what to dispatch after re-auth completes

	// video-level operation: non-nil when copy/move is initiated from the video list
	selectedVideos []youtube.Video

	// batch operation state (non-nil while a mutating operation is in flight)
	batchState *batchOpState
}

type retryTarget int

const (
	retryNone      retryTarget = iota
	retryPlaylists             // re-run loadPlaylists after re-auth
	retryVideos                // re-run video load(s) after re-auth
)

func newApp(logger *log.Logger, logPath string) appModel {
	m := appModel{
		logger:  logger,
		logPath: logPath,
	}

	needsOnboarding := true
	if _, err := os.Stat(sessionFile); err == nil {
		session, err := youtube.LoadSession(sessionFile)
		if err == nil && len(session.Cookies) > 0 {
			m.ytClient = youtube.NewClient(session, logger)
			m.screen = screenLoading
			needsOnboarding = false
		}
	}
	if needsOnboarding {
		m.screen = screenOnboarding
		m.onboarding = ui.NewOnboardingModel()
		m.onboarding.ConnectFunc = browserOpenCmd
	}
	return m
}

func (m appModel) Init() tea.Cmd {
	switch m.screen {
	case screenOnboarding:
		return m.onboarding.Init()
	case screenLoading:
		return m.loadPlaylists()
	}
	return nil
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.termW = msg.Width
		m.termH = msg.Height

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case fatalErrMsg:
		m.errMsg = msg.err.Error()
		return m, nil

	// ── Onboarding complete ──────────────────────────────────────────────────
	case ui.OnboardingDoneMsg:
		session, err := m.loadSessionFromMsg(msg)
		if err != nil {
			m.errMsg = fmt.Sprintf("failed to load session: %v", err)
			return m, nil
		}
		if len(session.Cookies) == 0 {
			m.errMsg = "no cookies found — make sure you exported a Netscape cookie file from youtube.com"
			return m, nil
		}
		if saveErr := youtube.SaveSession(sessionFile, session); saveErr != nil {
			m.logger.Printf("warning: could not save session: %v", saveErr)
		}
		m.ytClient = youtube.NewClient(session, m.logger)
		m.screen = screenLoading
		return m, m.loadPlaylists()

	// ── Playlists loaded ─────────────────────────────────────────────────────
	case playlistsLoadedMsg:
		ytPlaylists := msg.playlists
		uiPlaylists := make([]ui.Playlist, len(ytPlaylists))
		for i, p := range ytPlaylists {
			uiPlaylists[i] = ui.Playlist{ID: p.ID, Title: p.Title, Count: p.Count}
		}
		m.playlistList = ui.NewPlaylistListModel(uiPlaylists, m.termW, m.termH)
		m.screen = screenPlaylistList
		return m, m.playlistList.Init()

	// ── Playlist list done ────────────────────────────────────────────────────
	case ui.PlaylistListDoneMsg:
		return m, tea.Quit

	// ── Playlist delete ───────────────────────────────────────────────────────
	case ui.PlaylistDeleteRequestMsg:
		p := msg.Playlist
		return m, func() tea.Msg {
			// IRREVERSIBLE: playlist deletion cannot be undone by this app.
			if err := m.ytClient.DeletePlaylist(p.ID); err != nil {
				m.logger.Printf("ERROR DeletePlaylist %s (%s): %v", p.Title, p.ID, err)
				return playlistDeleteErrMsg{err}
			}
			m.logger.Printf("DeletePlaylist succeeded: %s (%s)", p.Title, p.ID)
			return playlistDeletedMsg{playlistID: p.ID}
		}

	case playlistDeletedMsg:
		updated := m.playlistList.RemovePlaylist(msg.playlistID)
		m.playlistList = updated
		return m, nil

	case playlistDeleteErrMsg:
		// Stay on the list screen; surface the error inline via status msg.
		m.playlistList.StatusMsg("delete failed: " + msg.err.Error())
		return m, nil

	// ── Video list ───────────────────────────────────────────────────────────
	case ui.PlaylistOpenMsg:
		p := msg.Playlist
		m.screen = screenLoading
		return m, func() tea.Msg {
			videos, _, err := m.ytClient.ListVideos(p.ID)
			if err != nil {
				return videosLoadErrMsg{err}
			}
			return videoListLoadedMsg{
				playlist: youtube.Playlist{ID: p.ID, Title: p.Title, Count: p.Count},
				videos:   videos,
			}
		}

	case videoListLoadedMsg:
		p := ui.Playlist{ID: msg.playlist.ID, Title: msg.playlist.Title, Count: msg.playlist.Count}
		m.videoList = ui.NewVideoListModel(p, toUIVideos(msg.videos), m.termW, m.termH)
		m.screen = screenVideoList
		return m, m.videoList.Init()

	case ui.VideoListDoneMsg:
		m.screen = screenPlaylistList
		return m, nil

	case ui.VideoOpenMsg:
		videoID := msg.Video.ID
		return m, func() tea.Msg {
			_ = openBrowser("https://www.youtube.com/watch?v=" + videoID)
			return nil
		}

	case ui.VideoListRemoveMsg:
		// Remove selected videos — show plan with y/n confirmation.
		m.source = msg.Source
		m.action = "remove"
		m.allVideos = toYTVideos(msg.Videos)
		m.videos = msg.Videos
		m.plan = ui.NewPlanModel(ui.PlanOpRemove, m.source, ui.Playlist{}, m.videos, nil, "", 0, m.termW, m.termH)
		m.screen = screenPlan
		return m, m.plan.Init()

	case ui.VideoListSelectTargetMsg:
		// User wants to copy/move selected videos; need to pick a target playlist.
		m.selectedVideos = toYTVideos(msg.Videos)
		m.playlistList = m.playlistList.EnterTargetSelect(msg.Action, msg.Source)
		m.screen = screenPlaylistList
		return m, nil

	// ── Broken video scan ─────────────────────────────────────────────────────
	case ui.PlaylistScanMsg:
		p := msg.Playlist
		m.screen = screenLoading
		return m, func() tea.Msg {
			videos, truncatedCount, err := m.ytClient.ScanBrokenVideos(p.ID, p.Count)
			if err != nil {
				return brokenVideosScanErrMsg{err}
			}
			return brokenVideosLoadedMsg{
				playlist:       youtube.Playlist{ID: p.ID, Title: p.Title, Count: p.Count},
				videos:         videos,
				truncatedCount: truncatedCount,
			}
		}

	case brokenVideosLoadedMsg:
		uiVideos := make([]ui.Video, len(msg.videos))
		for i, v := range msg.videos {
			uiVideos[i] = ui.Video{
				ID:                v.ID,
				SetVideoID:        v.SetVideoID,
				Title:             v.Title,
				Channel:           v.Channel,
				Duration:          v.Duration,
				PlaylistID:        v.PlaylistID,
				Unavailable:       v.Unavailable,
				UnavailableReason: v.UnavailableReason,
			}
		}
		p := ui.Playlist{ID: msg.playlist.ID, Title: msg.playlist.Title, Count: msg.playlist.Count}
		m.brokenVideos = ui.NewBrokenVideosModel(p, uiVideos, msg.truncatedCount, m.termW, m.termH)
		m.screen = screenBrokenVideos
		return m, m.brokenVideos.Init()

	case brokenVideosScanErrMsg:
		m.playlistList.StatusMsg("scan failed: " + msg.err.Error())
		m.screen = screenPlaylistList
		return m, nil

	case ui.BrokenVideosDoneMsg:
		m.screen = screenPlaylistList
		return m, nil

	case ui.BrokenVideosRemoveMsg:
		m.source = msg.Playlist
		m.action = "clear"
		m.allVideos = make([]youtube.Video, len(msg.Videos))
		for i, v := range msg.Videos {
			m.allVideos[i] = youtube.Video{
				ID: v.ID, SetVideoID: v.SetVideoID, Title: v.Title,
				Channel: v.Channel, Duration: v.Duration, PlaylistID: v.PlaylistID,
			}
		}
		m.videos = msg.Videos
		m.batchState = newBatchOpState("clear", m.allVideos, m.source, ui.Playlist{}, m.ytClient, m.logger)
		m.progress = ui.NewProgressModel(len(m.allVideos))
		m.screen = screenProgress
		return m, tea.Batch(m.progress.Init(), m.batchState.launchNextBatch())

	// ── Stats ────────────────────────────────────────────────────────────────
	case ui.PlaylistStatsMsg:
		p := msg.Playlist
		m.screen = screenLoading
		return m, func() tea.Msg {
			videos, _, err := m.ytClient.ListVideos(p.ID)
			if err != nil {
				return videosLoadErrMsg{err}
			}
			return statsVideosLoadedMsg{
				playlist: ui.Playlist{ID: p.ID, Title: p.Title, Count: p.Count},
				videos:   videos,
			}
		}

	case statsVideosLoadedMsg:
		uiVideos := toUIVideos(msg.videos)
		m.statsModel = ui.NewStatsModel(msg.playlist, m.termW, m.termH)
		m.screen = screenStats
		pl := msg.playlist
		return m, func() tea.Msg {
			return ui.StatsReadyMsg{Playlist: pl, Videos: uiVideos}
		}

	case ui.StatsDoneMsg:
		m.screen = screenPlaylistList
		return m, nil

	case playlistsLoadErrMsg:
		var authErr *youtube.AuthError
		if errors.As(msg.err, &authErr) {
			if !m.authRetried {
				// First attempt: try refreshing from disk silently.
				m.authRetried = true
				if client, err := m.tryRefreshSession(); err == nil {
					m.ytClient = client
					m.screen = screenLoading
					return m, m.loadPlaylists()
				}
			}
			// Disk refresh failed or already tried — ask the user to re-login.
			m.pendingRetry = retryPlaylists
			return m.startReAuth()
		}
		m.errMsg = fmt.Sprintf("failed to load playlists: %v", msg.err)
		return m, nil

	// ── Playlist(s) selected ─────────────────────────────────────────────────
	case ui.PlaylistsSelectedMsg: // move or copy — load videos
		m.action = msg.Action
		m.source = msg.Source
		m.target = msg.Target
		m.screen = screenLoading
		if len(m.selectedVideos) > 0 {
			// Video-level operation: source videos already known, only load dest.
			m.allVideos = m.selectedVideos
			m.videos = toUIVideos(m.selectedVideos)
			m.selectedVideos = nil
			m.sourceLoaded = true
			m.destLoaded = false
			return m, m.loadDestVideos()
		}
		m.sourceLoaded = false
		m.destLoaded = false
		return m, tea.Batch(m.loadSourceVideos(), m.loadDestVideos())

	case ui.PlaylistSelectedMsg: // export or clear — one playlist
		m.source = msg.Playlist
		m.action = msg.Action
		m.screen = screenLoading
		return m, m.loadVideos(m.source.ID)

	// ── Videos loaded ────────────────────────────────────────────────────────
	case videosLoadedMsg: // single-playlist ops (export / clear)
		m.allVideos = msg.videos
		m.videos = toUIVideos(msg.videos)
		m.skippedCount = msg.skippedCount
		defaultFilename := ""
		if m.action == "export" {
			defaultFilename = exportDefaultFilename(m.source.Title)
		}
		m.plan = ui.NewPlanModel(m.action, m.source, m.target, m.videos, nil, defaultFilename, m.skippedCount, m.termW, m.termH)
		m.screen = screenPlan
		return m, m.plan.Init()

	case sourceVideosLoadedMsg: // move/copy source
		m.allVideos = msg.videos
		m.videos = toUIVideos(msg.videos)
		m.skippedCount = msg.skippedCount
		m.sourceLoaded = true
		if m.destLoaded {
			return m.showMoveCopyPlan()
		}
		return m, nil

	case destVideosLoadedMsg: // move/copy destination
		m.destVideos = msg.videos
		m.destLoaded = true
		if m.sourceLoaded {
			return m.showMoveCopyPlan()
		}
		return m, nil

	case videosLoadErrMsg:
		var authErr *youtube.AuthError
		if errors.As(msg.err, &authErr) {
			if !m.authRetried {
				m.authRetried = true
				if client, err := m.tryRefreshSession(); err == nil {
					m.ytClient = client
					m.screen = screenLoading
					return m, m.retryVideoLoad()
				}
			}
			m.pendingRetry = retryVideos
			return m.startReAuth()
		}
		m.errMsg = fmt.Sprintf("failed to load videos: %v", msg.err)
		return m, nil

	// ── Plan confirmed ───────────────────────────────────────────────────────
	case ui.PlanConfirmedMsg:
		if m.action == "export" {
			filename := msg.Filename
			if filename == "" {
				filename = exportDefaultFilename(m.source.Title)
			}
			videos := m.allVideos
			return m, func() tea.Msg { return m.exportVideos(videos, filename) }
		}
		if msg.SkipDuplicates {
			filtered := make([]youtube.Video, 0, len(m.allVideos))
			destIDs := make(map[string]bool, len(m.destVideos))
			for _, v := range m.destVideos {
				destIDs[v.ID] = true
			}
			for _, v := range m.allVideos {
				if !destIDs[v.ID] {
					filtered = append(filtered, v)
				}
			}
			m.allVideos = filtered
			m.videos = toUIVideos(filtered)
		}
	case ui.PlanCancelledMsg:
		m.screen = screenPlaylistList
		m.authRetried = false
		return m, nil

	case exportDoneMsg:
		m.result = ui.NewResultModel(len(m.videos), 0, m.logPath)
		m.screen = screenResult
		return m, m.result.Init()

	// ── Result screen ────────────────────────────────────────────────────────
	case ui.ReturnToPlaylistsMsg:
		m.screen = screenLoading
		return m, m.loadPlaylists()
	}

	// Delegate to active sub-model.
	return m.delegateUpdate(msg)
}

func (m appModel) delegateUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.screen {
	case screenOnboarding:
		upd, c := m.onboarding.Update(msg)
		m.onboarding = upd.(ui.OnboardingModel)
		cmd = c

	case screenPlaylistList:
		upd, c := m.playlistList.Update(msg)
		m.playlistList = upd.(ui.PlaylistListModel)
		cmd = c

	case screenVideoList:
		upd, c := m.videoList.Update(msg)
		m.videoList = upd.(ui.VideoListModel)
		cmd = c

	case screenBrokenVideos:
		upd, c := m.brokenVideos.Update(msg)
		m.brokenVideos = upd.(ui.BrokenVideosModel)
		cmd = c

case screenPlan:
		upd, c := m.plan.Update(msg)
		m.plan = upd.(ui.PlanModel)
		cmd = c

	case screenProgress:
		upd, c := m.progress.Update(msg)
		m.progress = upd.(ui.ProgressModel)
		cmd = c

	case screenResult:
		upd, c := m.result.Update(msg)
		m.result = upd.(ui.ResultModel)
		cmd = c

	case screenStats:
		upd, c := m.statsModel.Update(msg)
		m.statsModel = upd.(ui.StatsModel)
		cmd = c
	}
	return m, cmd
}

func (m appModel) View() string {
	if m.errMsg != "" {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true).
			Render("Error: "+m.errMsg) +
			"\n\nPress ctrl+c to exit.\n"
	}
	switch m.screen {
	case screenOnboarding:
		v := m.onboarding.View()
		if m.reAuthMode {
			banner := lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).Bold(true).
				Render("Session expired. Provide fresh YouTube cookies to resume.")
			return banner + "\n\n" + v
		}
		return v
	case screenPlaylistList:
		return m.playlistList.View()
	case screenVideoList:
		return m.videoList.View()
	case screenBrokenVideos:
		return m.brokenVideos.View()
	case screenLoading:
		return lipgloss.NewStyle().Margin(2, 4).
			Render("Loading…\n\n" +
				lipgloss.NewStyle().Faint(true).Render("ctrl+c to quit"))
	case screenPlan:
		return m.plan.View()
	case screenProgress:
		return m.progress.View()
	case screenResult:
		return m.result.View()
	case screenStats:
		return m.statsModel.View()
	}
	return ""
}

// ── Commands ──────────────────────────────────────────────────────────────────

func (m appModel) loadPlaylists() tea.Cmd {
	return func() tea.Msg {
		playlists, err := m.ytClient.ListPlaylists()
		if err != nil {
			return playlistsLoadErrMsg{err}
		}
		return playlistsLoadedMsg{playlists}
	}
}

func (m appModel) loadVideos(playlistID string) tea.Cmd {
	return func() tea.Msg {
		videos, skipped, err := m.ytClient.ListVideos(playlistID)
		if err != nil {
			return videosLoadErrMsg{err}
		}
		return videosLoadedMsg{videos: videos, skippedCount: skipped}
	}
}

func (m appModel) loadSourceVideos() tea.Cmd {
	return func() tea.Msg {
		videos, skipped, err := m.ytClient.ListVideos(m.source.ID)
		if err != nil {
			return videosLoadErrMsg{err}
		}
		return sourceVideosLoadedMsg{videos: videos, skippedCount: skipped}
	}
}

func (m appModel) loadDestVideos() tea.Cmd {
	return func() tea.Msg {
		videos, _, err := m.ytClient.ListVideos(m.target.ID)
		if err != nil {
			return videosLoadErrMsg{err}
		}
		return destVideosLoadedMsg{videos: videos}
	}
}

// toUIVideos converts []youtube.Video to []ui.Video.
func toUIVideos(videos []youtube.Video) []ui.Video {
	out := make([]ui.Video, len(videos))
	for i, v := range videos {
		out[i] = ui.Video{
			ID:         v.ID,
			SetVideoID: v.SetVideoID,
			Title:      v.Title,
			Channel:    v.Channel,
			Duration:   v.Duration,
			PlaylistID: v.PlaylistID,
		}
	}
	return out
}

// showMoveCopyPlan builds the plan screen for move/copy once both source and
// dest video lists are loaded.
func (m appModel) showMoveCopyPlan() (tea.Model, tea.Cmd) {
	destIDs := make(map[string]bool, len(m.destVideos))
	for _, v := range m.destVideos {
		destIDs[v.ID] = true
	}
	m.plan = ui.NewPlanModel(m.action, m.source, m.target, m.videos, destIDs, "", m.skippedCount, m.termW, m.termH)
	m.screen = screenPlan
	return m, m.plan.Init()
}

// tryRefreshSession re-reads cookies from disk and returns a new client.
// It tries the paste file first, then the saved session JSON.
func (m appModel) tryRefreshSession() (*youtube.Client, error) {
	if session, err := youtube.LoadSessionFromCookieFile("cookies.txt"); err == nil && len(session.Cookies) > 0 {
		m.logger.Printf("auto-refresh: reloaded session from cookies.txt")
		return youtube.NewClient(session, m.logger), nil
	}
	if session, err := youtube.LoadSession(sessionFile); err == nil && len(session.Cookies) > 0 {
		m.logger.Printf("auto-refresh: reloaded session from session.json")
		return youtube.NewClient(session, m.logger), nil
	}
	return nil, fmt.Errorf("no valid session found on disk")
}

// startReAuth switches to the onboarding screen in re-auth mode, skipping
// directly to the paste step without the welcome or method-choice screens.
func (m appModel) startReAuth() (tea.Model, tea.Cmd) {
	m.reAuthMode = true
	m.onboarding = ui.NewReAuthModel()
	m.onboarding.ConnectFunc = browserOpenCmd
	m.screen = screenOnboarding
	return m, m.onboarding.Init()
}

// retryVideoLoad re-dispatches the appropriate video loading command(s) based
// on the current action.
func (m appModel) retryVideoLoad() tea.Cmd {
	if m.action == "move" || m.action == "copy" {
		m.sourceLoaded = false
		m.destLoaded = false
		return tea.Batch(m.loadSourceVideos(), m.loadDestVideos())
	}
	return m.loadVideos(m.source.ID)
}

// loadSessionFromMsg extracts and persists a session from an OnboardingDoneMsg.
func (m appModel) loadSessionFromMsg(msg ui.OnboardingDoneMsg) (youtube.Session, error) {
	var session youtube.Session
	var err error
	if msg.SessionPath != "" {
		session, err = youtube.LoadSessionFromCookieFile(msg.SessionPath)
		if err != nil {
			session, err = youtube.LoadSession(msg.SessionPath)
		}
	}
	return session, err
}


// ── Batch operations ──────────────────────────────────────────────────────────
//
// All mutating operations (copy, move, clear, remove) use batched API calls:
// multiple videos per request instead of one. YouTube serialises mutations per
// playlist server-side, so concurrent single-video requests are mostly rejected.
// Batching sidesteps this and reduces total request count.
//
// batchSize controls how many videos are bundled per API call. The YouTube
// internal API has no published limit; values up to ~50 are known to work.

const batchSize = 25

// batchResultMsg is returned after each batch API call.
type batchResultMsg struct {
	count     int    // number of videos in this batch
	lastName  string // title of last video in batch (shown in progress bar)
	failed    int    // 0 on success, count on failure (batch is atomic)
	authError bool
}

// batchOpState sequences batched mutations for all mutating operations.
// All fields are touched only from the bubbletea Update goroutine.
type batchOpState struct {
	action    string
	videos    []youtube.Video
	source    ui.Playlist
	target    ui.Playlist
	client    *youtube.Client
	logger    *log.Logger
	nextIdx   int
	processed int
	failed    int
}

func newBatchOpState(action string, videos []youtube.Video, source, target ui.Playlist, client *youtube.Client, logger *log.Logger) *batchOpState {
	return &batchOpState{
		action: action, videos: videos,
		source: source, target: target,
		client: client, logger: logger,
	}
}

// launchNextBatch dispatches the next batchSize videos as a single API call.
// Must only be called when nextIdx < len(videos).
func (s *batchOpState) launchNextBatch() tea.Cmd {
	end := s.nextIdx + batchSize
	if end > len(s.videos) {
		end = len(s.videos)
	}
	batch := s.videos[s.nextIdx:end]
	s.nextIdx = end

	lastName := batch[len(batch)-1].Title
	count := len(batch)
	action := s.action
	source := s.source
	target := s.target
	client := s.client
	logger := s.logger

	return func() tea.Msg {
		switch action {
		case "copy":
			if err := client.AddVideos(target.ID, batch); err != nil {
				var authErr *youtube.AuthError
				if errors.As(err, &authErr) {
					return batchResultMsg{count: count, lastName: lastName, failed: count, authError: true}
				}
				for _, v := range batch {
					logger.Printf("WARN copy failed for video %q (%s): %v", v.Title, v.ID, err)
				}
				return batchResultMsg{count: count, lastName: lastName, failed: count}
			}
			return batchResultMsg{count: count, lastName: lastName}

		case "move":
			if err := client.AddVideos(target.ID, batch); err != nil {
				var authErr *youtube.AuthError
				if errors.As(err, &authErr) {
					return batchResultMsg{count: count, lastName: lastName, failed: count, authError: true}
				}
				for _, v := range batch {
					logger.Printf("WARN move/add failed for video %q (%s): %v — skipping remove", v.Title, v.ID, err)
				}
				return batchResultMsg{count: count, lastName: lastName, failed: count}
			}
			// IRREVERSIBLE: removal from YouTube playlist cannot be undone by this app.
			if err := client.RemoveVideos(source.ID, batch); err != nil {
				var authErr *youtube.AuthError
				if errors.As(err, &authErr) {
					// Add succeeded but remove failed — videos are in both playlists.
					for _, v := range batch {
						logger.Printf("WARN session expired on remove for video %q — VIDEO EXISTS IN BOTH PLAYLISTS, manual cleanup may be required", v.Title)
					}
					return batchResultMsg{count: count, lastName: lastName, failed: count, authError: true}
				}
				// SAFETY WARNING: videos now exist in BOTH playlists — manual cleanup required.
				for _, v := range batch {
					logger.Printf("WARN move/remove failed for video %q (%s): %v — VIDEO EXISTS IN BOTH PLAYLISTS, manual cleanup required", v.Title, v.ID, err)
				}
				return batchResultMsg{count: count, lastName: lastName, failed: count}
			}
			return batchResultMsg{count: count, lastName: lastName}

		default: // "clear", "remove"
			// IRREVERSIBLE: removal from YouTube playlist cannot be undone by this app.
			if err := client.RemoveVideos(source.ID, batch); err != nil {
				var authErr *youtube.AuthError
				if errors.As(err, &authErr) {
					return batchResultMsg{count: count, lastName: lastName, failed: count, authError: true}
				}
				for _, v := range batch {
					logger.Printf("WARN clear failed for video %q (%s): %v", v.Title, v.ID, err)
				}
				return batchResultMsg{count: count, lastName: lastName, failed: count}
			}
			return batchResultMsg{count: count, lastName: lastName}
		}
	}
}


func exportDefaultFilename(playlistTitle string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '_'
	}, playlistTitle)
	if safe == "" {
		safe = "playlist"
	}
	return fmt.Sprintf("%s_%s.json", safe, time.Now().Format("20060102"))
}

func (m appModel) exportVideos(videos []youtube.Video, path string) tea.Msg {
	type exportVideo struct {
		Title    string `json:"title"`
		Channel  string `json:"channel"`
		Duration string `json:"duration"`
		VideoID  string `json:"video_id"`
	}
	out := make([]exportVideo, len(videos))
	for i, v := range videos {
		out[i] = exportVideo{Title: v.Title, Channel: v.Channel, Duration: v.Duration, VideoID: v.ID}
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fatalErrMsg{fmt.Errorf("marshalling export: %w", err)}
	}
	// Write atomically: temp file then rename to prevent partial JSON on crash.
	tmp, err := os.CreateTemp(".", ".yt-export-*.tmp")
	if err != nil {
		return fatalErrMsg{fmt.Errorf("creating temp export file: %w", err)}
	}
	tmpName := tmp.Name()
	writeOK := false
	defer func() {
		if !writeOK {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fatalErrMsg{fmt.Errorf("writing export temp file: %w", err)}
	}
	if err := tmp.Close(); err != nil {
		return fatalErrMsg{fmt.Errorf("closing export temp file: %w", err)}
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fatalErrMsg{fmt.Errorf("renaming export file: %w", err)}
	}
	writeOK = true
	m.logger.Printf("exported %d videos to %s", len(videos), path)
	return exportDoneMsg{path}
}

// ── Override Update to support chained per-video ops ─────────────────────────

func (m appModel) updateWithChain(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ui.OnboardingDoneMsg:
		if m.reAuthMode {
			// Session expired mid-operation: refresh credentials and resume.
			session, err := m.loadSessionFromMsg(msg)
			if err != nil || len(session.Cookies) == 0 {
				m.errMsg = "re-auth failed: could not load cookies — operation aborted"
				m.reAuthMode = false
				return m, nil
			}
			if saveErr := youtube.SaveSession(sessionFile, session); saveErr != nil {
				m.logger.Printf("warning: could not save refreshed session: %v", saveErr)
			}
			newClient := youtube.NewClient(session, m.logger)
			m.ytClient = newClient
			if m.batchState != nil {
				m.batchState.client = newClient
			}
			m.reAuthMode = false
			m.authRetried = false

			switch m.pendingRetry {
			case retryPlaylists:
				m.pendingRetry = retryNone
				m.screen = screenLoading
				return m, m.loadPlaylists()
			case retryVideos:
				m.pendingRetry = retryNone
				m.screen = screenLoading
				return m, m.retryVideoLoad()
			default: // mid-operation resume
				m.screen = screenProgress
				if m.batchState != nil {
					return m, m.batchState.launchNextBatch()
				}
			}
			return m, nil
		}
		return m.Update(msg)

	case ui.PlanConfirmedMsg:
		if m.action == "export" {
			m.screen = screenProgress
			m.progress = ui.NewProgressModel(len(m.videos))
			videos := m.allVideos
			filename := msg.Filename
			if filename == "" {
				filename = exportDefaultFilename(m.source.Title)
			}
			return m, tea.Batch(m.progress.Init(), func() tea.Msg {
				return m.exportVideos(videos, filename)
			})
		}
		// Apply skip-duplicates filter if requested.
		videos := m.allVideos
		if msg.SkipDuplicates && len(m.destVideos) > 0 {
			destIDs := make(map[string]bool, len(m.destVideos))
			for _, v := range m.destVideos {
				destIDs[v.ID] = true
			}
			filtered := make([]youtube.Video, 0, len(videos))
			for _, v := range videos {
				if !destIDs[v.ID] {
					filtered = append(filtered, v)
				}
			}
			m.logger.Printf("skip-duplicates: %d videos filtered out, %d will be processed", len(videos)-len(filtered), len(filtered))
			videos = filtered
		}
		// All mutating operations use the batch path.
		if len(videos) == 0 {
			m.result = ui.NewResultModel(0, 0, m.logPath)
			m.screen = screenResult
			return m, m.result.Init()
		}
		m.batchState = newBatchOpState(m.action, videos, m.source, m.target, m.ytClient, m.logger)
		m.progress = ui.NewProgressModel(len(videos))
		m.screen = screenProgress
		return m, tea.Batch(m.progress.Init(), m.batchState.launchNextBatch())

	case batchResultMsg:
		bs := m.batchState
		if bs == nil {
			return m, nil
		}
		if msg.authError {
			m.logger.Printf("WARN session expired mid-operation — pausing for re-auth")
			bs.nextIdx -= msg.count // step back so this batch is retried after re-auth
			return m.startReAuth()
		}
		bs.processed += msg.count
		bs.failed += msg.failed

		upd, uiCmd := m.progress.Update(ui.ProgressUpdateMsg{
			Current:    bs.processed,
			Total:      len(bs.videos),
			VideoTitle: msg.lastName,
			Failed:     bs.failed,
		})
		m.progress = upd.(ui.ProgressModel)

		if bs.nextIdx < len(bs.videos) {
			return m, tea.Batch(uiCmd, bs.launchNextBatch())
		}

		successes := bs.processed - bs.failed
		if successes < 0 {
			successes = 0
		}
		m.batchState = nil
		m.result = ui.NewResultModel(successes, bs.failed, m.logPath)
		m.screen = screenResult
		return m, tea.Batch(uiCmd, m.result.Init())
	}

	return m.Update(msg)
}

// ── Entry point ───────────────────────────────────────────────────────────────

func main() {
	logPath := fmt.Sprintf("yt_%s.log", time.Now().Format("20060102_150405"))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	logger := log.New(logFile, "", log.LstdFlags)
	logger.Println("yt started")

	model := newApp(logger, logPath)

	// Wrap Update to use the chained approach.
	wrapped := chainedApp{appModel: model}

	p := tea.NewProgram(wrapped, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// toYTVideos converts a slice of ui.Video to youtube.Video.
func toYTVideos(vids []ui.Video) []youtube.Video {
	out := make([]youtube.Video, len(vids))
	for i, v := range vids {
		out[i] = youtube.Video{
			ID:         v.ID,
			SetVideoID: v.SetVideoID,
			Title:      v.Title,
			Channel:    v.Channel,
			Duration:   v.Duration,
			PlaylistID: v.PlaylistID,
		}
	}
	return out
}

// chainedApp wraps appModel and overrides Update to use the chained per-video approach.
type chainedApp struct {
	appModel
}

func (c chainedApp) Init() tea.Cmd { return c.appModel.Init() }
func (c chainedApp) View() string  { return c.appModel.View() }

func (c chainedApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	newModel, cmd := c.appModel.updateWithChain(msg)
	c.appModel = newModel.(appModel)
	return c, cmd
}
