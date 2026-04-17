package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── styles ────────────────────────────────────────────────────────────────────

var (
	planOpStyleMove = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("196")). // red
			Background(lipgloss.Color("52")).
			Padding(0, 2)

	planOpStyleClear = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("214")). // orange/yellow
				Background(lipgloss.Color("52")).
				Padding(0, 2)

	planOpStyleCopy = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("82")). // green
			Padding(0, 2)

	planOpStyleExport = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39")). // blue
				Padding(0, 2)

	planSubHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				MarginTop(1)

	planRouteStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	planVideoHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212")).
				MarginTop(1).
				MarginBottom(1)

	planVideoLineStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	planHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true).
			MarginTop(1)

	planErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	planWarningBannerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).
				Bold(true).
				MarginBottom(1)

	planScrollHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Italic(true)
)

// ── PlanModel ─────────────────────────────────────────────────────────────────

// PlanModel shows the full operation plan and waits for user confirmation.
type PlanModel struct {
	operation      string // PlanOpMove / PlanOpCopy / PlanOpClear / PlanOpExport
	source         Playlist
	target         Playlist   // empty for single-playlist operations
	videos         []Video
	destVideoIDs   map[string]bool // non-nil for move/copy; enables skip-duplicates toggle
	skipDuplicates bool
	skippedCount   int // dup count (from initViewport)
	excludedCount  int // unavailable videos excluded before plan was built
	viewport       viewport.Model
	confirmInput   textinput.Model
	inputMode      bool   // true when CLEAR confirmation input is active
	inputErr       string // mismatch error message
	width          int
	height         int
	ready          bool
}

// NewPlanModel constructs a PlanModel. For CLEAR/EXPORT operations leave
// target as a zero-value Playlist and destVideoIDs as nil.
// defaultFilename is only used for PlanOpExport; it pre-fills the filename input.
// skippedCount is the number of unavailable videos excluded from the source list.
// termW/termH are the current terminal size.
func NewPlanModel(operation string, source, target Playlist, videos []Video, destVideoIDs map[string]bool, defaultFilename string, skippedCount int, termW, termH int) PlanModel {
	operation = strings.ToUpper(operation)
	if termW == 0 {
		termW = 120
	}
	if termH == 0 {
		termH = 40
	}

	ti := textinput.New()
	ti.CharLimit = 256
	ti.Width = 60

	isClear := operation == PlanOpClear
	isExport := operation == PlanOpExport
	if isClear {
		ti.Placeholder = PlanClearInputPrompt
		ti.Focus()
	} else if isExport {
		ti.Placeholder = PlanExportFilenamePrompt
		ti.SetValue(defaultFilename)
		ti.Focus()
	}
	// PlanOpRemove uses y/n like MOVE/COPY (not type-name like CLEAR).

	m := PlanModel{
		operation:     operation,
		source:        source,
		target:        target,
		videos:        videos,
		destVideoIDs:  destVideoIDs,
		excludedCount: skippedCount,
		confirmInput:  ti,
		inputMode:     isClear || isExport,
		width:         termW,
		height:        termH,
	}
	m.initViewport()
	return m
}

func (m PlanModel) Init() tea.Cmd {
	if m.inputMode {
		return textinput.Blink
	}
	return nil
}

func (m PlanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.initViewport()
		return m, nil

	case tea.KeyMsg:
		if m.inputMode {
			return m.updateClearInput(msg)
		}
		return m.updateConfirm(msg)
	}

	if m.ready {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

// updateClearInput handles key events when the CLEAR or EXPORT input is shown.
func (m PlanModel) updateClearInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		return m, func() tea.Msg { return PlanCancelledMsg{} }
	case tea.KeyEnter:
		if m.operation == PlanOpExport {
			filename := strings.TrimSpace(m.confirmInput.Value())
			if filename == "" {
				m.inputErr = PlanExportFilenameEmpty
				return m, nil
			}
			return m, func() tea.Msg { return PlanConfirmedMsg{Filename: filename} }
		}
		// CLEAR: require exact playlist name match.
		if strings.TrimSpace(m.confirmInput.Value()) == m.source.Title {
			return m, func() tea.Msg { return PlanConfirmedMsg{} }
		}
		m.inputErr = PlanClearMismatch
		m.confirmInput.Reset()
		return m, nil
	}
	var cmd tea.Cmd
	m.confirmInput, cmd = m.confirmInput.Update(msg)
	return m, cmd
}

// updateConfirm handles key events for non-CLEAR operations.
func (m PlanModel) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		return m, func() tea.Msg { return PlanConfirmedMsg{SkipDuplicates: m.skipDuplicates} }
	case "n", "N", "q", "Q":
		return m, func() tea.Msg { return PlanCancelledMsg{} }
	case "ctrl+c":
		return m, tea.Quit
	case "d", "D":
		if len(m.destVideoIDs) > 0 {
			m.skipDuplicates = !m.skipDuplicates
			m.initViewport()
			return m, nil
		}
	// Scroll
	case "up", "k":
		if m.ready {
			m.viewport.LineUp(1)
		}
	case "down", "j":
		if m.ready {
			m.viewport.LineDown(1)
		}
	case "pgup":
		if m.ready {
			m.viewport.HalfViewUp()
		}
	case "pgdown":
		if m.ready {
			m.viewport.HalfViewDown()
		}
	}
	return m, nil
}

func (m PlanModel) View() string {
	if !m.ready {
		return lipgloss.NewStyle().Margin(2, 4).Render("Loading…")
	}

	var b strings.Builder

	// Operation badge.
	b.WriteString(m.operationBadge())
	b.WriteString("\n")

	// Sub-header.
	if m.skipDuplicates && m.skippedCount > 0 {
		active := len(m.videos) - m.skippedCount
		b.WriteString(planSubHeaderStyle.Render(
			fmt.Sprintf(PlanSubHeaderFmtFiltered, active, len(m.videos), m.skippedCount),
		))
	} else {
		b.WriteString(planSubHeaderStyle.Render(
			fmt.Sprintf(PlanSubHeaderFmt, len(m.videos)),
		))
	}
	b.WriteString("\n")

	// Source / target route.
	b.WriteString(planRouteStyle.Render(fmt.Sprintf(PlanSourceFmt, m.source.Title)))
	if m.target.Title != "" {
		b.WriteString("\n")
		b.WriteString(planRouteStyle.Render(fmt.Sprintf(PlanTargetFmt, m.target.Title)))
	}
	b.WriteString("\n")

	// Scrollable video list.
	b.WriteString(planVideoHeaderStyle.Render(PlanVideoListHeader))
	b.WriteString("\n")
	b.WriteString(m.viewport.View())
	b.WriteString("\n")
	b.WriteString(planScrollHintStyle.Render(PlanScrollHint))
	b.WriteString("\n")
	if m.excludedCount > 0 {
		b.WriteString(planWarningBannerStyle.Render(fmt.Sprintf(PlanExcludedWarning, m.excludedCount)))
		b.WriteString("\n")
	}

	// Confirmation area.
	if m.inputMode {
		if m.operation == PlanOpClear {
			b.WriteString(planWarningBannerStyle.Render("⚠  This will permanently clear all videos from the playlist."))
			b.WriteString("\n")
			b.WriteString(planHintStyle.Render(PlanClearConfirmHint))
		} else if m.operation == PlanOpExport {
			b.WriteString(planHintStyle.Render(PlanExportFilenameHint))
		}
		b.WriteString("\n")
		b.WriteString(m.confirmInput.View())
		if m.inputErr != "" {
			b.WriteString("\n")
			b.WriteString(planErrorStyle.Render(m.inputErr))
		}
	} else {
		if len(m.destVideoIDs) > 0 {
			skipState := PlanSkipDuplicatesOff
			if m.skipDuplicates {
				skipState = PlanSkipDuplicatesOn
			}
			b.WriteString(planHintStyle.Render(fmt.Sprintf(PlanSkipDuplicatesHint, skipState)))
			b.WriteString("\n")
		}
		b.WriteString(planHintStyle.Render(PlanConfirmHint))
	}

	return lipgloss.NewStyle().Margin(1, 2).Render(b.String())
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (m PlanModel) operationBadge() string {
	label := fmt.Sprintf(PlanHeaderFmt, m.operation)
	switch m.operation {
	case PlanOpMove:
		return planOpStyleMove.Render(label)
	case PlanOpClear:
		return planOpStyleClear.Render(label)
	case PlanOpRemove:
		return planOpStyleClear.Render(label)
	case PlanOpCopy:
		return planOpStyleCopy.Render(label)
	default:
		return planOpStyleExport.Render(label)
	}
}

// initViewport builds the viewport content and sets its dimensions.
func (m *PlanModel) initViewport() {
	// Reserve lines: operation badge(1) + sub-header(2) + route(2) +
	// video header(2) + scroll hint(1) + confirm area(3) + margins(2) = ~13
	const reservedLines = 14
	vpHeight := m.height - reservedLines
	if vpHeight < 3 {
		vpHeight = 3
	}
	vpWidth := m.width - 6
	if vpWidth < 20 {
		vpWidth = 20
	}

	if !m.ready {
		m.viewport = viewport.New(vpWidth, vpHeight)
	} else {
		m.viewport.Width = vpWidth
		m.viewport.Height = vpHeight
	}

	// Build video list content.
	var sb strings.Builder
	skipped := 0
	dupStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Strikethrough(true)
	for _, v := range m.videos {
		isDup := m.destVideoIDs[v.ID]
		if isDup {
			skipped++
		}
		if m.skipDuplicates && isDup {
			continue
		}
		line := fmt.Sprintf(PlanVideoLineFmt, v.Title, v.Channel, v.Duration)
		if isDup {
			sb.WriteString(dupStyle.Render("  [already in target] "+v.Title))
		} else {
			sb.WriteString(planVideoLineStyle.Render(line))
		}
		sb.WriteString("\n")
	}
	m.skippedCount = skipped
	m.viewport.SetContent(sb.String())
	m.ready = true
}
