package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── styles ────────────────────────────────────────────────────────────────────

var (
	progressTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212")).
				MarginBottom(1)

	progressBarBgStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("237"))

	progressBarFgStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("82"))

	progressCountStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	progressCurrentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Italic(true)

	progressFailedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")).
				Bold(true)

	progressDoneStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("82")).
				Bold(true)
)

const progressBarWidth = 40

// ── ProgressModel ─────────────────────────────────────────────────────────────

// ProgressModel shows live progress during an operation and emits
// ProgressDoneMsg once the operation completes.
type ProgressModel struct {
	current    int
	total      int
	videoTitle string
	failed     int
	done       bool
	width      int
}

// NewProgressModel creates a ProgressModel for an operation with a known
// total number of videos.
func NewProgressModel(total int) ProgressModel {
	return ProgressModel{total: total}
}

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (m ProgressModel) Init() tea.Cmd { return nil }

func (m ProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case ProgressUpdateMsg:
		m.current = msg.Current
		m.total = msg.Total
		m.videoTitle = msg.VideoTitle
		m.failed = msg.Failed

		// Detect completion: caller sends Current == Total as the final update.
		if m.total > 0 && m.current >= m.total {
			m.done = true
			successes := m.total - m.failed
			if successes < 0 {
				successes = 0
			}
			return m, func() tea.Msg {
				return ProgressDoneMsg{Successes: successes, Failures: m.failed}
			}
		}
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m ProgressModel) View() string {
	var b strings.Builder

	b.WriteString(progressTitleStyle.Render(ProgressTitle))
	b.WriteString("\n")

	if m.done {
		b.WriteString(progressDoneStyle.Render(ProgressDone))
		return lipgloss.NewStyle().Margin(2, 4).Render(b.String())
	}

	// Progress bar.
	b.WriteString(m.renderBar())
	b.WriteString("\n\n")

	// "Video X of Y"
	b.WriteString(progressCountStyle.Render(
		fmt.Sprintf(ProgressCountFmt, m.current, m.total),
	))
	b.WriteString("\n")

	// Current video title.
	if m.videoTitle != "" {
		b.WriteString(progressCurrentStyle.Render(
			fmt.Sprintf(ProgressCurrentFmt, m.videoTitle),
		))
		b.WriteString("\n")
	}

	// Failure count (only when there are failures).
	if m.failed > 0 {
		b.WriteString(progressFailedStyle.Render(
			fmt.Sprintf(ProgressFailedFmt, m.failed),
		))
		b.WriteString("\n")
	}

	return lipgloss.NewStyle().Margin(2, 4).Render(b.String())
}

// renderBar returns a simple text-based progress bar.
func (m ProgressModel) renderBar() string {
	if m.total == 0 {
		return progressBarBgStyle.Render(strings.Repeat("─", progressBarWidth))
	}

	filled := int(float64(m.current) / float64(m.total) * float64(progressBarWidth))
	if filled > progressBarWidth {
		filled = progressBarWidth
	}
	empty := progressBarWidth - filled

	bar := progressBarFgStyle.Render(strings.Repeat("█", filled)) +
		progressBarBgStyle.Render(strings.Repeat("░", empty))

	pct := int(float64(m.current) / float64(m.total) * 100)
	return fmt.Sprintf("%s %3d%%", bar, pct)
}
