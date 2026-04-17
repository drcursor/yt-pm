package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── styles ────────────────────────────────────────────────────────────────────

var (
	resultTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212")).
				MarginBottom(2)

	resultSuccessStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("82")).
				Bold(true)

	resultFailureStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")).
				Bold(true)

	resultLogStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	resultHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true).
			MarginTop(2)

	resultBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("237")).
			Padding(1, 3)
)

// ── ResultModel ───────────────────────────────────────────────────────────────

// ResultModel shows the final operation summary.
type ResultModel struct {
	successes int
	failures  int
	logPath   string
	width     int
}

// NewResultModel constructs a ResultModel from the outcome of an operation.
func NewResultModel(successes, failures int, logPath string) ResultModel {
	return ResultModel{
		successes: successes,
		failures:  failures,
		logPath:   logPath,
	}
}

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (m ResultModel) Init() tea.Cmd { return nil }

func (m ResultModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "Q", "ctrl+c":
			return m, tea.Quit
		case "r", "R", "enter":
			return m, func() tea.Msg { return ReturnToPlaylistsMsg{} }
		}
	}
	return m, nil
}

func (m ResultModel) View() string {
	var inner strings.Builder

	inner.WriteString(resultSuccessStyle.Render(
		fmt.Sprintf(ResultSuccessFmt, m.successes),
	))
	inner.WriteString("\n")
	inner.WriteString(resultFailureStyle.Render(
		fmt.Sprintf(ResultFailedFmt, m.failures),
	))

	if m.logPath != "" {
		inner.WriteString("\n\n")
		inner.WriteString(resultLogStyle.Render(
			fmt.Sprintf(ResultLogFmt, m.logPath),
		))
	}

	box := resultBoxStyle.Render(inner.String())

	var outer strings.Builder
	outer.WriteString(resultTitleStyle.Render(ResultTitle))
	outer.WriteString("\n")
	outer.WriteString(box)
	outer.WriteString("\n")
	outer.WriteString(resultHintStyle.Render(ResultQuitHint))

	return lipgloss.NewStyle().Margin(2, 4).Render(outer.String())
}
