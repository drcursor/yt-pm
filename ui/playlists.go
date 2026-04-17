package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── list.Item adapter ─────────────────────────────────────────────────────────

type playlistItem struct{ p Playlist }

func (pi playlistItem) Title() string       { return pi.p.Title }
func (pi playlistItem) Description() string { return fmt.Sprintf("%d videos", pi.p.Count) }
func (pi playlistItem) FilterValue() string { return pi.p.Title }

// ── custom delegate ───────────────────────────────────────────────────────────

type playlistDelegate struct{}

func (d playlistDelegate) Height() int                             { return 2 }
func (d playlistDelegate) Spacing() int                            { return 0 }
func (d playlistDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d playlistDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	pi, ok := listItem.(playlistItem)
	if !ok {
		return
	}
	isSelected := index == m.Index()

	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)

	cursor := "  "
	if isSelected {
		cursor = cursorStyle.Render("▶ ")
		titleStyle = titleStyle.Bold(true).Foreground(lipgloss.Color("212"))
	}

	fmt.Fprintln(w, cursor+titleStyle.Render(pi.p.Title))
	fmt.Fprint(w, "  "+descStyle.Render(fmt.Sprintf("%d videos", pi.p.Count)))
}

// ── pane state ────────────────────────────────────────────────────────────────

type playlistsPane int

const (
	playlistsPaneSource playlistsPane = iota
	playlistsPaneTarget
)

// ── styles ────────────────────────────────────────────────────────────────────

var (
	playlistsPaneTitleActive = lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color("212")).
					BorderStyle(lipgloss.NormalBorder()).
					BorderBottom(true).
					BorderForeground(lipgloss.Color("212")).
					PaddingBottom(1)

	playlistsPaneTitleInactive = lipgloss.NewStyle().
					Foreground(lipgloss.Color("240")).
					BorderStyle(lipgloss.NormalBorder()).
					BorderBottom(true).
					BorderForeground(lipgloss.Color("237")).
					PaddingBottom(1)

	playlistsHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Italic(true)

	playlistsPaneBox = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("237")).
				Padding(0, 1)

	playlistsPaneBoxActive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("212")).
				Padding(0, 1)
)

// ── PlaylistsModel ────────────────────────────────────────────────────────────

type PlaylistsMode int

const (
	PlaylistsModeDouble PlaylistsMode = iota
	PlaylistsModeSingle
)

type PlaylistsModel struct {
	mode     PlaylistsMode
	action   string
	pane     playlistsPane
	source   list.Model
	target   list.Model
	selected *Playlist
	width    int
	height   int
}

func NewPlaylistsModelDouble(playlists []Playlist, termW, termH int) PlaylistsModel {
	return newPlaylistsModel(PlaylistsModeDouble, "", playlists, termW, termH)
}

func NewPlaylistsModelSingle(action string, playlists []Playlist, termW, termH int) PlaylistsModel {
	return newPlaylistsModel(PlaylistsModeSingle, action, playlists, termW, termH)
}

func newPlaylistsModel(mode PlaylistsMode, action string, playlists []Playlist, termW, termH int) PlaylistsModel {
	if termW == 0 {
		termW = 120
	}
	if termH == 0 {
		termH = 40
	}

	items := playlistsToItems(playlists)
	delegate := playlistDelegate{}

	listH := termH - 8
	if listH < 10 {
		listH = 10
	}

	var sourceW, targetW int
	if mode == PlaylistsModeSingle {
		sourceW = termW - 10
	} else {
		sourceW = (termW - 12) / 2
		targetW = sourceW
	}
	if sourceW < 30 {
		sourceW = 30
	}
	if targetW < 30 {
		targetW = 30
	}

	sourceList := list.New(items, delegate, sourceW, listH)
	sourceList.Title = PlaylistsPaneSource
	sourceList.SetShowHelp(false)
	sourceList.SetShowStatusBar(true)
	sourceList.SetFilteringEnabled(true)

	targetList := list.New(items, delegate, targetW, listH)
	targetList.Title = PlaylistsPaneTarget
	targetList.SetShowHelp(false)
	targetList.SetShowStatusBar(true)
	targetList.SetFilteringEnabled(true)

	return PlaylistsModel{
		mode:   mode,
		action: action,
		source: sourceList,
		target: targetList,
		width:  termW,
		height: termH,
	}
}

func playlistsToItems(ps []Playlist) []list.Item {
	items := make([]list.Item, len(ps))
	for i, p := range ps {
		items[i] = playlistItem{p: p}
	}
	return items
}

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (m PlaylistsModel) Init() tea.Cmd { return nil }

func (m PlaylistsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.relayout()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			// Only quit if not currently filtering.
			if !m.activeList().SettingFilter() {
				return m, tea.Quit
			}
		case "enter":
			if !m.activeList().SettingFilter() {
				return m.handleEnter()
			}
		case "esc":
			if m.activeList().SettingFilter() {
				break // let the list handle it
			}
			if m.mode == PlaylistsModeDouble && m.pane == playlistsPaneTarget {
				m.pane = playlistsPaneSource
				m.selected = nil
				return m, nil
			}
		}

		var cmd tea.Cmd
		if m.pane == playlistsPaneSource || m.mode == PlaylistsModeSingle {
			m.source, cmd = m.source.Update(msg)
		} else {
			m.target, cmd = m.target.Update(msg)
		}
		return m, cmd
	}

	return m, nil
}

func (m *PlaylistsModel) activeList() *list.Model {
	if m.pane == playlistsPaneTarget && m.mode == PlaylistsModeDouble {
		return &m.target
	}
	return &m.source
}

func (m PlaylistsModel) handleEnter() (tea.Model, tea.Cmd) {
	if m.mode == PlaylistsModeSingle {
		item, ok := m.source.SelectedItem().(playlistItem)
		if !ok {
			return m, nil
		}
		return m, func() tea.Msg {
			return PlaylistSelectedMsg{Playlist: item.p, Action: m.action}
		}
	}

	if m.pane == playlistsPaneSource {
		item, ok := m.source.SelectedItem().(playlistItem)
		if !ok {
			return m, nil
		}
		src := item.p
		m.selected = &src
		m.pane = playlistsPaneTarget
		return m, nil
	}

	item, ok := m.target.SelectedItem().(playlistItem)
	if !ok {
		return m, nil
	}
	src := *m.selected
	tgt := item.p
	return m, func() tea.Msg {
		return PlaylistsSelectedMsg{Source: src, Target: tgt}
	}
}

func (m PlaylistsModel) View() string {
	if m.mode == PlaylistsModeSingle {
		return m.viewSingle()
	}
	return m.viewDouble()
}

func (m PlaylistsModel) viewSingle() string {
	hint := playlistsHintStyle.Render(PlaylistsSelectSingle + "  •  " + PlaylistsQuitHint)
	content := playlistsPaneBoxActive.Width(m.width - 8).Render(m.source.View())
	return lipgloss.NewStyle().Margin(1, 2).Render(
		lipgloss.JoinVertical(lipgloss.Left, hint, content),
	)
}

func (m PlaylistsModel) viewDouble() string {
	sourceTitle := playlistsPaneTitleActive.Render(PlaylistsPaneSource)
	targetTitle := playlistsPaneTitleInactive.Render(PlaylistsPaneTarget)
	hint := playlistsHintStyle.Render(PlaylistsSelectSource + "  •  " + PlaylistsQuitHint)

	if m.pane == playlistsPaneTarget {
		sourceTitle = playlistsPaneTitleInactive.Render(PlaylistsPaneSource)
		targetTitle = playlistsPaneTitleActive.Render(PlaylistsPaneTarget)
		hint = playlistsHintStyle.Render(PlaylistsSelectTarget + "  •  " + PlaylistsQuitHint)
	}

	halfWidth := (m.width - 12) / 2
	if halfWidth < 30 {
		halfWidth = 30
	}

	sourceBox := playlistsPaneBox
	targetBox := playlistsPaneBox
	if m.pane == playlistsPaneSource {
		sourceBox = playlistsPaneBoxActive
	} else {
		targetBox = playlistsPaneBoxActive
	}

	leftPane := lipgloss.JoinVertical(lipgloss.Left, sourceTitle,
		sourceBox.Width(halfWidth).Render(m.source.View()))
	rightPane := lipgloss.JoinVertical(lipgloss.Left, targetTitle,
		targetBox.Width(halfWidth).Render(m.target.View()))

	panes := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, "  ", rightPane)

	parts := []string{hint}
	if m.selected != nil {
		badge := lipgloss.NewStyle().Foreground(lipgloss.Color("86")).
			Render(fmt.Sprintf("Source: %s", m.selected.Title))
		parts = append(parts, badge)
	}
	parts = append(parts, panes)

	return lipgloss.NewStyle().Margin(1, 2).Render(strings.Join(parts, "\n"))
}

func (m *PlaylistsModel) relayout() {
	listH := m.height - 8
	if listH < 10 {
		listH = 10
	}

	if m.mode == PlaylistsModeSingle {
		m.source.SetWidth(m.width - 10)
		m.source.SetHeight(listH)
		return
	}

	half := (m.width - 12) / 2
	if half < 30 {
		half = 30
	}
	m.source.SetWidth(half)
	m.source.SetHeight(listH)
	m.target.SetWidth(half)
	m.target.SetHeight(listH)
}
