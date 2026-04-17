package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	vlTitleStyle    = lipgloss.NewStyle().Bold(true).MarginBottom(1)
	vlRowStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	vlCursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	vlSelectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	vlBothStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true)
	vlChannelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	vlHintStyle     = lipgloss.NewStyle().Faint(true)
)

// VideoListModel shows a playlist's videos and supports multi-select for
// copy, move, and remove operations.
type VideoListModel struct {
	playlist Playlist
	videos   []Video
	cursor   int
	selected map[int]bool
	vp       viewport.Model
	width    int
	height   int
}

func NewVideoListModel(playlist Playlist, videos []Video, termW, termH int) VideoListModel {
	if termW == 0 {
		termW = 120
	}
	if termH == 0 {
		termH = 40
	}
	m := VideoListModel{
		playlist: playlist,
		videos:   videos,
		selected: make(map[int]bool),
		width:    termW,
		height:   termH,
	}
	m.vp = viewport.New(termW-8, m.vpHeight())
	m.vp.SetContent(m.renderContent())
	return m
}

func (m VideoListModel) vpHeight() int {
	h := m.height - 8
	if h < 4 {
		h = 4
	}
	return h
}

func (m VideoListModel) Init() tea.Cmd { return nil }

func (m VideoListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.vp.Width = m.width - 8
		m.vp.Height = m.vpHeight()
		m.vp.SetContent(m.renderContent())
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m VideoListModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		return m, func() tea.Msg { return VideoListDoneMsg{} }
	case "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.scrollToCursor()
			m.vp.SetContent(m.renderContent())
		}
	case "down", "j":
		if m.cursor < len(m.videos)-1 {
			m.cursor++
			m.scrollToCursor()
			m.vp.SetContent(m.renderContent())
		}

	case " ":
		if len(m.videos) > 0 {
			m.selected[m.cursor] = !m.selected[m.cursor]
			if !m.selected[m.cursor] {
				delete(m.selected, m.cursor)
			}
			m.vp.SetContent(m.renderContent())
		}

	case "a":
		if len(m.selected) == len(m.videos) {
			m.selected = make(map[int]bool)
		} else {
			for i := range m.videos {
				m.selected[i] = true
			}
		}
		m.vp.SetContent(m.renderContent())

	case "x":
		targets := m.targetVideos()
		if len(targets) == 0 {
			return m, nil
		}
		src := m.playlist
		return m, func() tea.Msg {
			return VideoListRemoveMsg{Source: src, Videos: targets}
		}

	case "c", "m":
		targets := m.targetVideos()
		if len(targets) == 0 {
			return m, nil
		}
		action := msg.String()
		if action == "m" {
			action = "move"
		} else {
			action = "copy"
		}
		src := m.playlist
		return m, func() tea.Msg {
			return VideoListSelectTargetMsg{Action: action, Source: src, Videos: targets}
		}

	case "o":
		if len(m.videos) > 0 {
			v := m.videos[m.cursor]
			return m, func() tea.Msg { return VideoOpenMsg{Video: v} }
		}
	}
	return m, nil
}

// targetVideos returns the selected videos, or the cursor video if nothing is selected.
func (m VideoListModel) targetVideos() []Video {
	if len(m.selected) > 0 {
		out := make([]Video, 0, len(m.selected))
		for i, v := range m.videos {
			if m.selected[i] {
				out = append(out, v)
			}
		}
		return out
	}
	if len(m.videos) > 0 {
		return []Video{m.videos[m.cursor]}
	}
	return nil
}

func (m VideoListModel) View() string {
	title := vlTitleStyle.Render(fmt.Sprintf(VideoListTitleFmt, m.playlist.Title, len(m.videos)))

	var content string
	if len(m.videos) == 0 {
		content = vlHintStyle.Render(VideoListEmptyMsg)
	} else {
		content = m.vp.View()
	}

	hint := VideoListHint
	if nsel := len(m.selected); nsel > 0 {
		hint = fmt.Sprintf(VideoListSelHint, nsel)
	}

	return lipgloss.NewStyle().Margin(2, 4).Render(
		title + "\n\n" + content + "\n\n" + vlHintStyle.Render(hint),
	)
}

func (m VideoListModel) renderContent() string {
	if len(m.videos) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, v := range m.videos {
		isCursor := i == m.cursor
		isSel := m.selected[i]

		cursor := VideoListCursorOff
		check := VideoListCheckOff
		rowStyle := vlRowStyle
		chStyle := vlChannelStyle

		if isCursor && isSel {
			cursor = VideoListCursorOn
			check = VideoListCheckOn
			rowStyle = vlBothStyle
			chStyle = vlBothStyle
		} else if isCursor {
			cursor = VideoListCursorOn
			rowStyle = vlCursorStyle
			chStyle = vlCursorStyle
		} else if isSel {
			check = VideoListCheckOn
			rowStyle = vlSelectedStyle
			chStyle = vlSelectedStyle
		}

		title := truncateStr(v.Title, 40)
		ch := truncateStr(v.Channel, 22)
		row := fmt.Sprintf("%s%s  %-40s  %-22s  %s", cursor, check, title, ch, v.Duration)
		sb.WriteString(rowStyle.Render(row))
		sb.WriteString("\n")
		_ = chStyle // silence unused warning; chStyle used in per-field approach if needed
	}
	return sb.String()
}

func (m *VideoListModel) scrollToCursor() {
	vpH := m.vp.Height
	offset := m.vp.YOffset
	if m.cursor < offset {
		m.vp.SetYOffset(m.cursor)
	} else if m.cursor >= offset+vpH {
		m.vp.SetYOffset(m.cursor - vpH + 1)
	}
}
