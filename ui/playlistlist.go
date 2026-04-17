package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PlaylistListDoneMsg is emitted when the user quits the list view.
type PlaylistListDoneMsg struct{}

var (
	plListTitleStyle    = lipgloss.NewStyle().Bold(true).MarginBottom(1)
	plListRowStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	plListSelectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	plListSourceStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Strikethrough(true)
	plListCountStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	plListHintStyle     = lipgloss.NewStyle().Faint(true)
	plListWarningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	plListErrorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	plListSuccessStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
	plListModeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
)

type plListMode int

const (
	plListModeBrowse       plListMode = iota
	plListModeSelectTarget            // picking a target for move/copy
	plListModeConfirmDelete
)

// PlaylistListModel is the main screen: a scrollable, actionable list of
// the user's playlists. It replaces the top-level action menu.
type PlaylistListModel struct {
	playlists []Playlist
	cursor    int
	vp        viewport.Model
	width     int
	height    int
	mode      plListMode

	// target-selection state (move / copy)
	pendingAction string
	source        Playlist

	// delete confirmation state
	confirmInput textinput.Model
	confirmErr   string

	statusMsg string
}

func NewPlaylistListModel(playlists []Playlist, termW, termH int) PlaylistListModel {
	if termW == 0 {
		termW = 120
	}
	if termH == 0 {
		termH = 40
	}

	ti := textinput.New()
	ti.Placeholder = "Playlist name"
	ti.CharLimit = 256
	ti.Width = 50

	m := PlaylistListModel{
		playlists:    playlists,
		confirmInput: ti,
		width:        termW,
		height:       termH,
	}
	m.vp = viewport.New(termW-8, m.vpHeight())
	m.vp.SetContent(m.renderContent())
	return m
}

func (m PlaylistListModel) vpHeight() int {
	h := m.height - 9
	if h < 5 {
		h = 5
	}
	return h
}

func (m PlaylistListModel) Init() tea.Cmd { return nil }

// StatusMsg sets a one-line status message shown below the list.
func (m *PlaylistListModel) StatusMsg(s string) { m.statusMsg = s }

func (m PlaylistListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.vp.Width = m.width - 8
		m.vp.Height = m.vpHeight()
		m.vp.SetContent(m.renderContent())
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case plListModeBrowse:
			return m.updateBrowse(msg)
		case plListModeSelectTarget:
			return m.updateTargetSelect(msg)
		case plListModeConfirmDelete:
			return m.updateConfirmDelete(msg)
		}
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m PlaylistListModel) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.statusMsg = ""
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.scrollToCursor()
			m.vp.SetContent(m.renderContent())
		}
	case "down", "j":
		if m.cursor < len(m.playlists)-1 {
			m.cursor++
			m.scrollToCursor()
			m.vp.SetContent(m.renderContent())
		}
	case "enter":
		if len(m.playlists) > 0 {
			p := m.playlists[m.cursor]
			if p.Count == 0 {
				return m, nil
			}
			return m, func() tea.Msg { return PlaylistOpenMsg{Playlist: p} }
		}
	case "m":
		if len(m.playlists) > 0 {
			m.pendingAction = "move"
			m.source = m.playlists[m.cursor]
			m.mode = plListModeSelectTarget
			m.vp.SetContent(m.renderContent())
		}
	case "c":
		if len(m.playlists) > 0 {
			m.pendingAction = "copy"
			m.source = m.playlists[m.cursor]
			m.mode = plListModeSelectTarget
			m.vp.SetContent(m.renderContent())
		}
	case "e":
		if len(m.playlists) > 0 {
			p := m.playlists[m.cursor]
			return m, func() tea.Msg {
				return PlaylistSelectedMsg{Action: "export", Playlist: p}
			}
		}
	case "x":
		if len(m.playlists) > 0 {
			p := m.playlists[m.cursor]
			return m, func() tea.Msg {
				return PlaylistSelectedMsg{Action: "clear", Playlist: p}
			}
		}
	case "f":
		if len(m.playlists) > 0 {
			p := m.playlists[m.cursor]
			return m, func() tea.Msg { return PlaylistScanMsg{Playlist: p} }
		}
	case "s":
		if len(m.playlists) > 0 {
			p := m.playlists[m.cursor]
			return m, func() tea.Msg { return PlaylistStatsMsg{Playlist: p} }
		}
	case "d":
		if len(m.playlists) > 0 {
			m.mode = plListModeConfirmDelete
			m.confirmErr = ""
			m.confirmInput.Reset()
			m.confirmInput.Focus()
			m.vp.SetContent(m.renderContent())
			return m, textinput.Blink
		}
	}
	return m, nil
}

func (m PlaylistListModel) updateTargetSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = plListModeBrowse
		m.vp.SetContent(m.renderContent())
		return m, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.scrollToCursor()
			m.vp.SetContent(m.renderContent())
		}
	case "down", "j":
		if m.cursor < len(m.playlists)-1 {
			m.cursor++
			m.scrollToCursor()
			m.vp.SetContent(m.renderContent())
		}
	case "enter":
		target := m.playlists[m.cursor]
		action := m.pendingAction
		source := m.source
		m.mode = plListModeBrowse
		return m, func() tea.Msg {
			return PlaylistsSelectedMsg{Action: action, Source: source, Target: target}
		}
	}
	return m, nil
}

func (m PlaylistListModel) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.mode = plListModeBrowse
		m.confirmErr = ""
		m.confirmInput.Reset()
		m.vp.SetContent(m.renderContent())
		return m, nil
	case tea.KeyEnter:
		typed := strings.TrimSpace(m.confirmInput.Value())
		target := m.playlists[m.cursor]
		if typed != target.Title {
			m.confirmErr = PlListDeleteMismatch
			m.confirmInput.Reset()
			return m, nil
		}
		m.mode = plListModeBrowse
		m.confirmInput.Reset()
		return m, func() tea.Msg { return PlaylistDeleteRequestMsg{Playlist: target} }
	}
	var cmd tea.Cmd
	m.confirmInput, cmd = m.confirmInput.Update(msg)
	return m, cmd
}

// EnterTargetSelect switches the model to target-selection mode externally
// (used when copy/move is triggered from the video list screen).
func (m PlaylistListModel) EnterTargetSelect(action string, source Playlist) PlaylistListModel {
	m.pendingAction = action
	m.source = source
	m.mode = plListModeSelectTarget
	m.vp.SetContent(m.renderContent())
	return m
}

// RemovePlaylist removes the playlist with the given ID and adjusts the cursor.
func (m PlaylistListModel) RemovePlaylist(id string) PlaylistListModel {
	filtered := make([]Playlist, 0, len(m.playlists)-1)
	for _, p := range m.playlists {
		if p.ID != id {
			filtered = append(filtered, p)
		}
	}
	m.playlists = filtered
	if m.cursor >= len(m.playlists) && m.cursor > 0 {
		m.cursor = len(m.playlists) - 1
	}
	m.vp.SetContent(m.renderContent())
	return m
}

func (m PlaylistListModel) View() string {
	var title string
	switch m.mode {
	case plListModeSelectTarget:
		action := strings.ToUpper(m.pendingAction)
		title = plListTitleStyle.Render(fmt.Sprintf("Select target for %s", action)) + "\n" +
			plListModeStyle.Render("From: "+m.source.Title)
	default:
		title = plListTitleStyle.Render(fmt.Sprintf("Your playlists (%d)", len(m.playlists)))
	}

	var bottom strings.Builder
	switch m.mode {
	case plListModeConfirmDelete:
		bottom.WriteString(plListWarningStyle.Render(PlListDeleteWarning))
		bottom.WriteString("\n")
		bottom.WriteString(plListHintStyle.Render(PlListDeleteConfirm))
		bottom.WriteString("\n")
		bottom.WriteString(m.confirmInput.View())
		if m.confirmErr != "" {
			bottom.WriteString("\n")
			bottom.WriteString(plListErrorStyle.Render(m.confirmErr))
		}
	case plListModeSelectTarget:
		bottom.WriteString(plListHintStyle.Render("↑/↓ navigate  •  Enter select target  •  Esc cancel"))
	default:
		if m.statusMsg != "" {
			bottom.WriteString(plListSuccessStyle.Render(m.statusMsg))
			bottom.WriteString("\n")
		}
		bottom.WriteString(plListHintStyle.Render(
			"↑/↓ navigate  •  enter open  •  m move  •  c copy  •  e export  •  x clear  •  f find broken  •  s stats  •  d delete  •  q quit",
		))
	}

	return lipgloss.NewStyle().Margin(2, 4).Render(
		title + "\n\n" + m.vp.View() + "\n\n" + bottom.String(),
	)
}

func (m PlaylistListModel) renderContent() string {
	if len(m.playlists) == 0 {
		return plListRowStyle.Render("(no playlists found)")
	}

	var sb strings.Builder
	for i, p := range m.playlists {
		isSelected := i == m.cursor
		isSource := m.mode == plListModeSelectTarget && p.ID == m.source.ID

		prefix := "    "
		rowStyle := plListRowStyle
		if isSelected {
			prefix = "▸   "
			rowStyle = plListSelectedStyle
		}
		if isSource {
			rowStyle = plListSourceStyle
		}

		count := fmt.Sprintf("%d videos", p.Count)
		row := fmt.Sprintf("%s%-50s  %s", prefix, truncateStr(p.Title, 50), plListCountStyle.Render(count))
		sb.WriteString(rowStyle.Render(row))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (m *PlaylistListModel) scrollToCursor() {
	vpH := m.vp.Height
	offset := m.vp.YOffset
	if m.cursor < offset {
		m.vp.SetYOffset(m.cursor)
	} else if m.cursor >= offset+vpH {
		m.vp.SetYOffset(m.cursor - vpH + 1)
	}
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
