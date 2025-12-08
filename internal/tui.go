package internal

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type errMsg error

type tuiModel struct {
	textarea    textarea.Model
	err         error
	manager     *Manager
	submitting  bool
	quitting    bool
	initMessage string
	ctx         context.Context
	cancel      context.CancelFunc
}

func initialModel(manager *Manager, initMessage string) tuiModel {
	ti := textarea.New()
	ti.Placeholder = "Type a message..."
	ti.Focus()
	ti.CharLimit = 0 // No limit
	ti.SetHeight(3)
	ti.ShowLineNumbers = false

	if initMessage != "" {
		ti.SetValue(initMessage)
	}

	return tuiModel{
		textarea:    ti,
		err:         nil,
		manager:     manager,
		initMessage: initMessage,
	}
}

func (m tuiModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			if m.textarea.Focused() {
				m.textarea.Blur()
			}
		case tea.KeyCtrlC:
			return m, tea.Quit
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

			val := m.textarea.Value()
			if strings.TrimSpace(val) == "" {
				return m, nil
			}

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
			if !m.textarea.Focused() {
				cmd = m.textarea.Focus()
				cmds = append(cmds, cmd)
			}
		}

	case errMsg:
		m.err = msg
		return m, nil
	}

	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m tuiModel) View() string {
	return fmt.Sprintf(
		"\n%s\n\n%s",
		m.textarea.View(),
		"(Ctrl+C to quit, Enter to send)",
	)
}

// StartTUI starts the Bubble Tea interface
func (c *CLIInterface) StartTUI(initMessage string) error {
	c.printWelcomeMessage()

	// Initial message handling
	if initMessage != "" {
		fmt.Printf("%s%s\n", c.manager.GetPrompt(), initMessage)
		c.processInput(initMessage)
	}

	for {
		// Initialize the model
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
			input := m.textarea.Value()

			// Check for exit/quit
			trimmed := strings.TrimSpace(input)
			if trimmed == "exit" || trimmed == "quit" {
				return nil
			}

			if trimmed != "" {
				// We need to print the prompt and input because the TUI clears it or we want a log
				fmt.Printf("%s%s\n", c.manager.GetPrompt(), input)

				// Process the input
				// This function writes to stdout/stderr, so we must be out of Bubble Tea alt screen (or not using alt screen)
				// By default tea.NewProgram doesn't use alt screen unless configured.
				c.processInput(input)
			}
		} else {
			// User quit without submitting (Ctrl+C)
			return nil
		}
	}
}
