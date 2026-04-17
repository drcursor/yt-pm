package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type bvFilter int

const (
	bvFilterAll      bvFilter = iota // show all unavailable entries
	bvFilterBroken                   // private / deleted / no metadata
	bvFilterUnlisted                 // no channel, has title (likely unlisted)
)

var (
	bvTitleStyle   = lipgloss.NewStyle().Bold(true).MarginBottom(1)
	bvCountStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	bvNoneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	bvRowStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	bvReasonStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	bvHintStyle    = lipgloss.NewStyle().Faint(true)
	bvWarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	bvFilterStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
)

var bvCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)

// BrokenVideosModel shows unavailable / unlisted videos found in a playlist
// and offers to remove them.
type BrokenVideosModel struct {
	playlist       Playlist
	all            []Video  // full scan result (unavailable entries only)
	truncatedCount int      // videos that could not be scanned due to pagination limits
	filter         bvFilter
	cursor         int
	vp             viewport.Model
	width          int
	height         int
	ready          bool
	confirm        bool // waiting for y/n to remove
}

// NewBrokenVideosModel builds the model. videos should be the list returned
// by ScanBrokenVideos (all Unavailable == true). truncatedCount is the number
// of videos that could not be scanned due to pagination limits.
func NewBrokenVideosModel(playlist Playlist, videos []Video, truncatedCount int, termW, termH int) BrokenVideosModel {
	if termW == 0 {
		termW = 120
	}
	if termH == 0 {
		termH = 40
	}
	m := BrokenVideosModel{
		playlist:       playlist,
		all:            videos,
		truncatedCount: truncatedCount,
		width:          termW,
		height:         termH,
	}
	m.vp = viewport.New(termW-8, m.vpHeight())
	m.vp.SetContent(m.renderContent())
	m.ready = true
	return m
}

func (m BrokenVideosModel) vpHeight() int {
	h := m.height - 10
	if h < 4 {
		h = 4
	}
	return h
}

func (m BrokenVideosModel) Init() tea.Cmd { return nil }

func (m BrokenVideosModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.vp.Width = m.width - 8
		m.vp.Height = m.vpHeight()
		m.vp.SetContent(m.renderContent())
		return m, nil

	case tea.KeyMsg:
		if m.confirm {
			return m.updateConfirm(msg)
		}
		return m.updateBrowse(msg)
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m BrokenVideosModel) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, func() tea.Msg { return BrokenVideosDoneMsg{} }
	case "tab":
		m.filter = (m.filter + 1) % 3
		m.cursor = 0
		m.vp.SetContent(m.renderContent())
	case "r":
		visible := m.visibleVideos()
		if len(visible) > 0 {
			m.confirm = true
		}
	case "up", "k":
		visible := m.visibleVideos()
		if m.cursor > 0 {
			m.cursor--
			m.scrollToCursor(len(visible))
			m.vp.SetContent(m.renderContent())
		}
	case "down", "j":
		visible := m.visibleVideos()
		if m.cursor < len(visible)-1 {
			m.cursor++
			m.scrollToCursor(len(visible))
			m.vp.SetContent(m.renderContent())
		}
	case "o":
		visible := m.visibleVideos()
		if len(visible) > 0 {
			v := visible[m.cursor]
			return m, func() tea.Msg { return VideoOpenMsg{Video: v} }
		}
	}
	return m, nil
}

func (m *BrokenVideosModel) scrollToCursor(total int) {
	// Each entry occupies 2 lines (row + reason).
	linePos := m.cursor * 2
	vpH := m.vp.Height
	offset := m.vp.YOffset
	if linePos < offset {
		m.vp.SetYOffset(linePos)
	} else if linePos >= offset+vpH {
		m.vp.SetYOffset(linePos - vpH + 2)
	}
}

func (m BrokenVideosModel) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		visible := m.visibleVideos()
		playlist := m.playlist
		m.confirm = false
		return m, func() tea.Msg {
			return BrokenVideosRemoveMsg{Playlist: playlist, Videos: visible}
		}
	case "n", "N", "esc", "q":
		m.confirm = false
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m BrokenVideosModel) visibleVideos() []Video {
	var out []Video
	for _, v := range m.all {
		if m.matchesFilter(v) {
			out = append(out, v)
		}
	}
	return out
}

func (m BrokenVideosModel) matchesFilter(v Video) bool {
	switch m.filter {
	case bvFilterBroken:
		return isBroken(v)
	case bvFilterUnlisted:
		return isUnlisted(v)
	default:
		return true
	}
}

// isBroken: private, deleted, or completely missing metadata.
func isBroken(v Video) bool {
	return v.SetVideoID == "" || v.Channel == "" && v.Duration == ""
}

// isUnlisted: has a title but no channel (still exists, just unlisted/no attribution).
func isUnlisted(v Video) bool {
	return !isBroken(v) && v.Channel == ""
}

func (m BrokenVideosModel) View() string {
	title := bvTitleStyle.Render(fmt.Sprintf(BrokenVideosTitle, m.playlist.Title))

	visible := m.visibleVideos()

	filterLabel := filterName(m.filter)
	filterLine := bvFilterStyle.Render(fmt.Sprintf(BrokenVideosFilterHint, filterLabel))

	var body string
	if len(m.all) == 0 {
		body = bvNoneStyle.Render(BrokenVideosNone)
	} else {
		count := bvCountStyle.Render(fmt.Sprintf(BrokenVideosCount, len(visible)))
		body = count + "\n" + filterLine + "\n\n" + m.vp.View()
	}

	var bottom string
	if m.confirm {
		bottom = bvWarningStyle.Render(fmt.Sprintf(BrokenVideosConfirm, len(visible)))
	} else if len(visible) > 0 {
		bottom = bvHintStyle.Render(BrokenVideosRemoveHint)
	} else {
		bottom = bvHintStyle.Render(BrokenVideosBackHint)
	}

	var warning string
	if m.truncatedCount > 0 {
		warning = "\n" + bvWarningStyle.Render(fmt.Sprintf("⚠  %d video(s) could not be scanned (pagination limit — export and re-import to find them)", m.truncatedCount))
	}

	return lipgloss.NewStyle().Margin(2, 4).Render(
		title + "\n\n" + body + warning + "\n\n" + bottom,
	)
}

func (m BrokenVideosModel) renderContent() string {
	visible := m.visibleVideos()
	if len(visible) == 0 {
		return bvHintStyle.Render("(none in this view)")
	}
	var sb strings.Builder
	for i, v := range visible {
		isCursor := i == m.cursor
		title := v.Title
		if title == "" {
			title = "(no title)"
		}
		kind := "broken"
		if isUnlisted(v) {
			kind = "unlisted"
		}
		reason := v.UnavailableReason
		if reason == "" {
			reason = kind
		}
		cursor := "  "
		rowStyle := bvRowStyle
		if isCursor {
			cursor = "▸ "
			rowStyle = bvCursorStyle
		}
		line := fmt.Sprintf("%s%-45s  %-10s  %s",
			cursor,
			truncateStr(title, 45),
			kind,
			v.ID,
		)
		sb.WriteString(rowStyle.Render(line))
		sb.WriteString("\n")
		sb.WriteString(bvReasonStyle.Render("    " + reason))
		sb.WriteString("\n")
	}
	return sb.String()
}

func filterName(f bvFilter) string {
	switch f {
	case bvFilterBroken:
		return BrokenVideosFilterBroken
	case bvFilterUnlisted:
		return BrokenVideosFilterUnlisted
	default:
		return BrokenVideosFilterAll
	}
}
