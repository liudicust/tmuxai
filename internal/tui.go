package internal

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/system"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	enableFocusReport  = "\x1b[?1004h"
	disableFocusReport = "\x1b[?1004l"
)

type errMsg error

type checkFocusMsg struct{ active bool }

func isPaneActive(mgr *Manager) bool {
	details, err := system.TmuxPanesDetails(mgr.PaneId)
	if err != nil || len(details) == 0 {
		return false
	}
	return details[0].IsActive == 1
}

func focusTick(mgr *Manager) tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
		return checkFocusMsg{active: isPaneActive(mgr)}
	})
}

type tuiModel struct {
	textInput   textinput.Model
	err         error
	manager     *Manager
	submitting  bool
	quitting    bool
	initMessage string
	ctx         context.Context
	cancel      context.CancelFunc
	history     []string
	histIndex   int
	histPath    string
	width       int
	height      int
}

func initialModel(manager *Manager, initMessage string) tuiModel {
	ti := textinput.New()
	ti.Placeholder = "Type your message or \\command..."
	ti.Prompt = manager.GetPrompt()
	ti.Blur()
	ti.CharLimit = 0
	ti.Width = 0

	if initMessage != "" {
		ti.SetValue(initMessage)
	}

	hp := defaultHistoryPath()
	entries := loadHistory(hp)

	return tuiModel{
		textInput:   ti,
		err:         nil,
		manager:     manager,
		initMessage: initMessage,
		history:     entries,
		histIndex:   -1,
		histPath:    hp,
	}
}

func defaultHistoryPath() string {
	return config.GetConfigFilePath("history.txt")
}

func loadHistory(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return []string{}
	}
	s := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	if len(s) > 0 && s[len(s)-1] == "" {
		s = s[:len(s)-1]
	}
	return s
}

func (m *tuiModel) saveHistory() {
	_ = os.WriteFile(m.histPath, []byte(strings.Join(m.history, "\n")), 0644)
}

func (m *tuiModel) appendHistory(val string) {
	if strings.TrimSpace(val) == "" {
		return
	}
	m.history = append(m.history, val)
	m.histIndex = -1
	m.saveHistory()
}

func (m *tuiModel) showHistoryAt(i int) {
	if i < 0 || i >= len(m.history) {
		return
	}
	m.histIndex = i
	m.textInput.SetValue(m.history[i])
}

func (m *tuiModel) prevHistory() {
	if len(m.history) == 0 {
		return
	}
	if m.histIndex <= 0 {
		m.showHistoryAt(len(m.history) - 1)
		return
	}
	m.showHistoryAt(m.histIndex - 1)
}

func (m *tuiModel) nextHistory() {
	if len(m.history) == 0 {
		return
	}
	if m.histIndex < 0 {
		return
	}
	if m.histIndex >= len(m.history)-1 {
		m.histIndex = -1
		m.textInput.SetValue("")
		return
	}
	m.showHistoryAt(m.histIndex + 1)
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(
		tea.Printf(enableFocusReport),
		focusTick(m.manager),
	)
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case checkFocusMsg:
		if msg.active {
			if !m.textInput.Focused() {
				m.textInput.Focus()
				return m, tea.Batch(textinput.Blink, focusTick(m.manager))
			}
		} else {
			if m.textInput.Focused() {
				m.textInput.Blur()
			}
		}
		return m, focusTick(m.manager)
	case tea.FocusMsg:
		return m, tea.Batch(m.textInput.Focus(), textinput.Blink)
	case tea.BlurMsg:
		m.textInput.Blur()
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		available := m.width - 4
		if available < 1 {
			available = 1
		}
		m.textInput.Width = available - len(m.textInput.Prompt)
		if m.textInput.Width < 1 {
			m.textInput.Width = 1
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			if m.textInput.Focused() {
				m.textInput.Blur()
			}
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyUp:
			m.prevHistory()
			return m, nil
		case tea.KeyDown:
			m.nextHistory()
			return m, nil
		case tea.KeyEnter:
			// Handle Shift+Enter for newline
			// Note: Bubble Tea doesn't distinguish Shift+Enter from Enter by default in some terminals,
			// but textarea usually handles Enter as newline.
			// We want Enter to submit and maybe Alt+Enter or something for newline?
			// Actually, standard behavior for chat apps:
			// Enter -> Submit
			// Shift+Enter / Alt+Enter -> Newline
			// However, textarea component defaults Enter to newline.
			// We need to override this behavior.

			// Let's try to detect if we should submit
			// Since tea.KeyEnter usually adds a newline in textarea, we need to intercept it
			// IF we want "Enter to submit".
			// But wait, the previous implementation used readline which is line-based.
			// Bubble Tea's textarea is multiline.

			// Common pattern:
			// Ctrl+S or specialized key to save/send?
			// Or check for empty line at end?
			// Let's stick to: Enter submits, unless Alt+Enter (if detectable)
			// But textarea.Model handles Enter by adding a newline.

			// Let's assume standard behavior for CLI:
			// If we want to submit, maybe we use a specific key binding?
			// The original implementation was:
			// "Process the input (preserving multiline content)" - implying readline handled it.
			// Actually readline usually returns on Enter.
			// So user expects Enter to submit.

			// So, if Enter is pressed, we submit.
			// If user wants newline, maybe Alt+Enter?
			// But let's look at how we can control this.

			// We'll define: Enter -> Submit
			// Alt+Enter (if possible) -> Newline (handled by textarea if we let it pass?)
			// Or we can just use the value.

			val := m.textInput.Value()
			if strings.TrimSpace(val) == "" {
				return m, nil
			}
			m.appendHistory(val)

			// We submit
			m.submitting = true

			// Quit the bubbletea loop to process the message, then we will restart it or similar?
			// The original loop was: ReadLine -> Process -> Loop
			// Here we are in a persistent UI loop.

			// We need to pause the UI, process the message, and then come back.
			// Or process asynchronously.
			// The original ProcessInput was blocking and printed output to stdout.
			// If we are in Bubble Tea, we own stdout.
			// We need to release stdout for the manager to print things, or capture output.
			// Manager.ProcessUserMessage prints directly to stdout/stderr.

			// So we should Quit, process, and then restart the program?
			// Or we can use tea.Exec to run the external process?
			// Manager.ProcessUserMessage is a function call, not an external process, but it writes to stdout.

			return m, tea.Quit

		default:
			if !m.textInput.Focused() {
				cmd = m.textInput.Focus()
				cmds = append(cmds, cmd)
			}
		}

	case errMsg:
		m.err = msg
		return m, nil
	}

	m.textInput, cmd = m.textInput.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m tuiModel) View() string {
	borderColor := lipgloss.Color("62")
	if !m.textInput.Focused() {
		borderColor = lipgloss.Color("240")
	}
	w := m.width
	if w <= 0 {
		w = 80
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Margin(0).
		Width(w - 2)
	return box.Render(m.textInput.View())
}

// StartTUI starts the Bubble Tea interface
func (c *CLIInterface) StartTUI(initMessage string) error {
	//c.printWelcomeMessage()

	// Initial message handling
	if initMessage != "" {
		fmt.Printf("%s%s\n", c.manager.GetPrompt(), initMessage)
		c.processInput(initMessage)
	}

	for {
		// Initialize the model on the normal screen to preserve existing outputs
		p := tea.NewProgram(initialModel(c.manager, ""))

		// Run the program
		finalModel, err := p.Run()
		if err != nil {
			return err
		}

		m, ok := finalModel.(tuiModel)
		if !ok {
			return fmt.Errorf("could not assert model")
		}

		if m.submitting {
			input := m.textInput.Value()

			// Check for exit/quit
			trimmed := strings.TrimSpace(input)
			if trimmed == "exit" || trimmed == "quit" {
				return nil
			}

			if trimmed != "" {
				lower := strings.ToLower(strings.TrimSpace(input))
				if strings.HasPrefix(lower, "/") {
					parts := strings.Fields(lower)
					if len(parts) > 0 {
						if parts[0] == "/reset" || parts[0] == "/clear" {
							fmt.Print(disableFocusReport)
						}
					}
				}
				//fmt.Printf("%s%s\n", c.manager.GetPrompt(), input)

				c.processInput(input)
			}
		} else {
			fmt.Print(disableFocusReport)
			return nil
		}
	}
}
