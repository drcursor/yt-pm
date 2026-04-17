package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── styles ────────────────────────────────────────────────────────────────────

var (
	statsTitleStyle = lipgloss.NewStyle().Bold(true).MarginBottom(1)
	statsBoxStyle   = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 3).
			BorderForeground(lipgloss.Color("240"))
	statsLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	statsHintStyle  = lipgloss.NewStyle().Faint(true).MarginTop(1)
)

// StatsReadyMsg is sent when videos have been loaded and stats are ready to compute.
type StatsReadyMsg struct {
	Playlist Playlist
	Videos   []Video
}

// StatsModel shows aggregated stats for a playlist.
type StatsModel struct {
	playlist       Playlist
	loading        bool
	totalVideos    int
	totalSeconds   int
	avgSeconds     int
	longestTitle   string
	longestSeconds int
	shortestTitle  string
	shortestSeconds int
	channelCount   int
	width          int
	height         int
}

// NewStatsModel creates a StatsModel in loading state.
func NewStatsModel(playlist Playlist, termW, termH int) StatsModel {
	if termW == 0 {
		termW = 120
	}
	if termH == 0 {
		termH = 40
	}
	return StatsModel{
		playlist: playlist,
		loading:  true,
		width:    termW,
		height:   termH,
	}
}

func (m StatsModel) Init() tea.Cmd { return nil }

func (m StatsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case StatsReadyMsg:
		m = m.computeStats(msg.Videos)
		m.loading = false
		return m, nil

	case tea.KeyMsg:
		return m, func() tea.Msg { return StatsDoneMsg{} }
	}
	return m, nil
}

func (m StatsModel) computeStats(videos []Video) StatsModel {
	if len(videos) == 0 {
		m.totalVideos = 0
		return m
	}

	m.totalVideos = len(videos)
	channels := make(map[string]struct{})

	longestSec := -1
	shortestSec := -1

	for _, v := range videos {
		secs := parseDuration(v.Duration)
		m.totalSeconds += secs

		if v.Channel != "" {
			channels[v.Channel] = struct{}{}
		}

		if longestSec < 0 || secs > longestSec {
			longestSec = secs
			m.longestTitle = v.Title
			m.longestSeconds = secs
		}

		if secs > 0 && (shortestSec < 0 || secs < shortestSec) {
			shortestSec = secs
			m.shortestTitle = v.Title
			m.shortestSeconds = secs
		}
	}

	if m.totalVideos > 0 {
		m.avgSeconds = m.totalSeconds / m.totalVideos
	}
	m.channelCount = len(channels)
	if longestSec < 0 {
		m.longestSeconds = 0
	}
	if shortestSec < 0 {
		m.shortestSeconds = 0
	}
	return m
}

func (m StatsModel) View() string {
	if m.loading {
		return lipgloss.NewStyle().Margin(2, 4).Render(StatsLoading)
	}

	title := statsTitleStyle.Render(fmt.Sprintf(StatsTitleFmt, m.playlist.Title))

	var sb strings.Builder
	sb.WriteString(statsLabelStyle.Render(fmt.Sprintf(StatsVideos, m.totalVideos)))
	sb.WriteString("\n")
	sb.WriteString(statsLabelStyle.Render(fmt.Sprintf(StatsTotalTime, formatDuration(m.totalSeconds))))
	sb.WriteString("\n")
	sb.WriteString(statsLabelStyle.Render(fmt.Sprintf(StatsAvgTime, formatDuration(m.avgSeconds))))
	sb.WriteString("\n")
	if m.longestTitle != "" {
		sb.WriteString(statsLabelStyle.Render(fmt.Sprintf(StatsLongest, truncateStr(m.longestTitle, 40), formatDuration(m.longestSeconds))))
		sb.WriteString("\n")
	}
	if m.shortestTitle != "" {
		sb.WriteString(statsLabelStyle.Render(fmt.Sprintf(StatsShortest, truncateStr(m.shortestTitle, 40), formatDuration(m.shortestSeconds))))
		sb.WriteString("\n")
	}
	sb.WriteString(statsLabelStyle.Render(fmt.Sprintf(StatsChannels, m.channelCount)))

	box := statsBoxStyle.Render(sb.String())
	hint := statsHintStyle.Render(StatsHint)

	return lipgloss.NewStyle().Margin(2, 4).Render(title + "\n" + box + "\n" + hint)
}

// formatDuration formats seconds as "Xm Ys" or "Xh Ym Zs".
func formatDuration(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	if seconds < 3600 {
		m := seconds / 60
		s := seconds % 60
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := seconds / 3600
	rem := seconds % 3600
	m := rem / 60
	s := rem % 60
	return fmt.Sprintf("%dh %dm %ds", h, m, s)
}

// parseDuration parses a YouTube duration string ("SS", "M:SS", "MM:SS",
// "H:MM:SS", "HH:MM:SS") and returns the total number of seconds.
// Returns 0 on any error.
func parseDuration(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	parts := strings.Split(s, ":")
	vals := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return 0
		}
		vals[i] = n
	}
	switch len(vals) {
	case 1:
		return vals[0]
	case 2:
		return vals[0]*60 + vals[1]
	case 3:
		return vals[0]*3600 + vals[1]*60 + vals[2]
	}
	return 0
}
