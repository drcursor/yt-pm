package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const cookiePasteFile = "cookies.txt"

// onboardingStep tracks which step of the flow we are on.
type onboardingStep int

const (
	onboardingStepWelcome    onboardingStep = iota // Step 1 — welcome message
	onboardingStepChoose                           // Step 2 — choose method
	onboardingStepInput                            // Step 3 — cookie path or paste area
	onboardingStepConnecting                       // Step 4 — connecting / result
)

// onboardingMethod is the chosen authentication method.
type onboardingMethod int

const (
	onboardingMethodNone    onboardingMethod = iota
	onboardingMethodPaste                    // paste cookie content directly (default)
	onboardingMethodBrowser                  // open browser first, then paste cookie content
	onboardingMethodFile                     // user supplies a cookie file path
)

// connectResultMsg is an internal message carrying the outcome of a connection
// attempt.
type connectResultMsg struct {
	sessionPath string
	err         error
}

// OnboardingModel drives the first-run authentication flow.
type OnboardingModel struct {
	step        onboardingStep
	method      onboardingMethod
	input       textinput.Model // used for file path (method 3)
	paste       textarea.Model  // used for cookie paste (methods 1 & 2)
	sessionPath string
	errMsg      string
	width       int
	reAuth      bool // true when re-authenticating mid-session (skips welcome/choose)

	// ConnectFunc is called when the browser method is chosen (path == "")
	// to open the browser as a side-effect. It must return nil as its Msg.
	ConnectFunc func(path string) tea.Cmd
}

// NewOnboardingModel returns a ready-to-use OnboardingModel for first-run.
func NewOnboardingModel() OnboardingModel {
	return newOnboardingModel(false)
}

// NewReAuthModel returns an OnboardingModel that skips straight to the paste
// screen, for use when a session expires mid-session.
func NewReAuthModel() OnboardingModel {
	m := newOnboardingModel(true)
	m.step = onboardingStepInput
	m.method = onboardingMethodPaste
	m.paste.Focus()
	return m
}

func newOnboardingModel(reAuth bool) OnboardingModel {
	ti := textinput.New()
	ti.Placeholder = OnboardingCookiePathPrompt
	ti.CharLimit = 512
	ti.Width = 60

	ta := textarea.New()
	ta.Placeholder = OnboardingPastePlaceholder
	ta.SetWidth(70)
	ta.SetHeight(12)
	ta.ShowLineNumbers = false

	return OnboardingModel{
		step:   onboardingStepWelcome,
		input:  ti,
		paste:  ta,
		reAuth: reAuth,
	}
}

// ── lipgloss styles ───────────────────────────────────────────────────────────

var (
	onboardingTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212")).
				MarginBottom(1)

	onboardingBodyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				MarginBottom(1)

	onboardingHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Italic(true)

	onboardingChoiceStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("86"))

	onboardingErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")).
				Bold(true)

	onboardingSuccessStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("82")).
				Bold(true)

	onboardingSpinnerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214"))
)

// ── tea.Model interface ───────────────────────────────────────────────────────

func (m OnboardingModel) Init() tea.Cmd {
	if m.reAuth {
		return textarea.Blink
	}
	return nil
}

func (m OnboardingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case connectResultMsg:
		if msg.err != nil {
			m.errMsg = OnboardingError + msg.err.Error()
		} else {
			m.sessionPath = msg.sessionPath
			m.errMsg = ""
		}
		return m, nil

	case tea.KeyMsg:
		switch m.step {

		// ── Step 1: welcome ──────────────────────────────────────────────────
		case onboardingStepWelcome:
			if msg.Type == tea.KeyEnter {
				m.step = onboardingStepChoose
			}
			if msg.String() == "q" || msg.Type == tea.KeyCtrlC {
				return m, tea.Quit
			}

		// ── Step 2: choose method ────────────────────────────────────────────
		case onboardingStepChoose:
			switch msg.String() {
			case "1", "enter":
				m.method = onboardingMethodPaste
				m.paste.Reset()
				m.paste.Focus()
				m.step = onboardingStepInput
				return m, textarea.Blink
			case "2":
				m.method = onboardingMethodBrowser
				m.paste.Reset()
				m.paste.Focus()
				m.step = onboardingStepInput
				var openCmd tea.Cmd
				if m.ConnectFunc != nil {
					openCmd = m.ConnectFunc("")
				}
				return m, tea.Batch(textarea.Blink, openCmd)
			case "3":
				m.method = onboardingMethodFile
				m.input.Placeholder = OnboardingCookiePathPrompt
				m.input.Focus()
				m.step = onboardingStepInput
				return m, textinput.Blink
			case "q":
				return m, tea.Quit
			}

		// ── Step 3: input ────────────────────────────────────────────────────
		case onboardingStepInput:
			if m.method == onboardingMethodFile {
				switch msg.Type {
				case tea.KeyEsc:
					m.step = onboardingStepChoose
					m.input.Reset()
					return m, nil
				case tea.KeyEnter:
					m.step = onboardingStepConnecting
					m.errMsg = ""
					path := strings.TrimSpace(m.input.Value())
					return m, m.doConnect(path)
				case tea.KeyCtrlC:
					return m, tea.Quit
				}
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				return m, cmd
			}

			// Paste / browser method — textarea; Ctrl+D to confirm, Esc to go back.
			switch msg.Type {
			case tea.KeyCtrlD:
				content := strings.TrimSpace(m.paste.Value())
				if content == "" {
					return m, nil // nothing pasted yet
				}
				m.step = onboardingStepConnecting
				m.errMsg = ""
				return m, m.doConnectPaste(content)
			case tea.KeyEsc:
				m.step = onboardingStepChoose
				m.paste.Reset()
				return m, nil
			case tea.KeyCtrlC:
				return m, tea.Quit
			}
			var cmd tea.Cmd
			m.paste, cmd = m.paste.Update(msg)
			return m, cmd

		// ── Step 4: connecting ───────────────────────────────────────────────
		case onboardingStepConnecting:
			switch msg.Type {
			case tea.KeyEnter:
				if m.sessionPath != "" && m.errMsg == "" {
					return m, func() tea.Msg {
						return OnboardingDoneMsg{SessionPath: m.sessionPath}
					}
				}
				if m.errMsg != "" {
					m.step = onboardingStepChoose
					m.errMsg = ""
					m.input.Reset()
					m.paste.Reset()
				}
			case tea.KeyEsc:
				if m.errMsg != "" {
					m.step = onboardingStepChoose
					m.errMsg = ""
					m.input.Reset()
					m.paste.Reset()
				}
			case tea.KeyCtrlC:
				return m, tea.Quit
			case tea.KeyRunes:
				if msg.String() == "q" {
					return m, tea.Quit
				}
			}
		}
	}
	return m, nil
}

func (m OnboardingModel) View() string {
	var b strings.Builder

	switch m.step {
	case onboardingStepWelcome:
		if m.reAuth {
			// Should not normally be reached; reAuth starts at stepInput.
			break
		}
		b.WriteString(onboardingTitleStyle.Render(OnboardingWelcomeTitle))
		b.WriteString("\n\n")
		b.WriteString(onboardingBodyStyle.Render(OnboardingWelcomeBody))

	case onboardingStepChoose:
		b.WriteString(onboardingTitleStyle.Render(OnboardingChooseMethodPrompt))
		b.WriteString("\n\n")
		b.WriteString(onboardingChoiceStyle.Render(OnboardingChoicePaste))
		b.WriteString("\n")
		b.WriteString(onboardingChoiceStyle.Render(OnboardingChoiceBrowser))
		b.WriteString("\n")
		b.WriteString(onboardingChoiceStyle.Render(OnboardingChoiceFile))
		b.WriteString("\n\n")
		b.WriteString(onboardingHintStyle.Render(OnboardingChoiceHint))

	case onboardingStepInput:
		switch m.method {
		case onboardingMethodPaste:
			b.WriteString(onboardingBodyStyle.Render(OnboardingPasteHint))
			b.WriteString("\n\n")
			b.WriteString(m.paste.View())
			b.WriteString("\n\n")
			b.WriteString(onboardingHintStyle.Render("Ctrl+D — confirm  •  Esc — go back"))
		case onboardingMethodBrowser:
			b.WriteString(onboardingBodyStyle.Render(OnboardingBrowserHint))
			b.WriteString("\n\n")
			b.WriteString(m.paste.View())
			b.WriteString("\n\n")
			b.WriteString(onboardingHintStyle.Render("Ctrl+D — confirm  •  Esc — go back"))
		case onboardingMethodFile:
			b.WriteString(onboardingBodyStyle.Render(OnboardingCookiePathHint))
			b.WriteString("\n\n")
			b.WriteString(m.input.View())
			b.WriteString("\n\n")
			b.WriteString(onboardingHintStyle.Render("Esc — go back"))
		}

	case onboardingStepConnecting:
		if m.errMsg != "" {
			b.WriteString(onboardingErrorStyle.Render(m.errMsg))
			b.WriteString("\n\n")
			b.WriteString(onboardingHintStyle.Render(OnboardingRetryHint))
		} else if m.sessionPath != "" {
			b.WriteString(onboardingSuccessStyle.Render(OnboardingSuccess))
		} else {
			b.WriteString(onboardingSpinnerStyle.Render(OnboardingConnecting))
		}
	}

	return lipgloss.NewStyle().Margin(2, 4).Render(b.String())
}

// ── helpers ───────────────────────────────────────────────────────────────────

// doConnect handles the file-path method.
func (m OnboardingModel) doConnect(path string) tea.Cmd {
	return func() tea.Msg {
		return connectResultMsg{sessionPath: path}
	}
}

// doConnectPaste saves the pasted cookie content to cookies.txt atomically and
// returns a connectResultMsg with the saved file path.
func (m OnboardingModel) doConnectPaste(content string) tea.Cmd {
	return func() tea.Msg {
		tmp, err := os.CreateTemp(".", ".yt-cookies-*.tmp")
		if err != nil {
			return connectResultMsg{err: fmt.Errorf("creating temp cookie file: %w", err)}
		}
		tmpName := tmp.Name()
		ok := false
		defer func() {
			if !ok {
				_ = os.Remove(tmpName)
			}
		}()
		if err := tmp.Chmod(0600); err != nil {
			_ = tmp.Close()
			return connectResultMsg{err: fmt.Errorf("setting cookie file permissions: %w", err)}
		}
		if _, err := fmt.Fprint(tmp, content); err != nil {
			_ = tmp.Close()
			return connectResultMsg{err: fmt.Errorf("writing cookie content: %w", err)}
		}
		if err := tmp.Close(); err != nil {
			return connectResultMsg{err: fmt.Errorf("closing temp cookie file: %w", err)}
		}
		if err := os.Rename(tmpName, cookiePasteFile); err != nil {
			return connectResultMsg{err: fmt.Errorf("saving cookie file: %w", err)}
		}
		ok = true
		return connectResultMsg{sessionPath: cookiePasteFile}
	}
}
