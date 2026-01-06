package internal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/term"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/logger"
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

var lastFocusActive bool
var lastFocusCheck time.Time
var focusChecking int32
var windowSizeMsgLogCount int32
var cursorMoveLogCount int32
var clearLinesLogCount int32

func tuiDebug(mgr *Manager, format string, v ...interface{}) {
	if mgr == nil || mgr.Config == nil || !mgr.Config.Debug {
		return
	}
	logger.Debug(format, v...)
}

func readActive(mgr *Manager) bool {
	if time.Since(lastFocusCheck) < 300*time.Millisecond || atomic.LoadInt32(&focusChecking) == 1 {
		return lastFocusActive
	}
	if !atomic.CompareAndSwapInt32(&focusChecking, 0, 1) {
		return lastFocusActive
	}
	go func() {
		a := isPaneActive(mgr)
		lastFocusActive = a
		lastFocusCheck = time.Now()
		atomic.StoreInt32(&focusChecking, 0)
	}()
	return lastFocusActive
}

func isPaneActive(mgr *Manager) bool {
	details, err := system.TmuxPanesDetails(mgr.PaneId)
	if err != nil || len(details) == 0 {
		return false
	}
	return details[0].IsActive == 1
}

func focusTick(mgr *Manager) tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return checkFocusMsg{active: readActive(mgr)}
	})
}

type tuiModel struct {
	textInput      textinput.Model
	err            error
	manager        *Manager
	submitting     bool
	quitting       bool
	initMessage    string
	ctx            context.Context
	cancel         context.CancelFunc
	history        []string
	histIndex      int
	histPath       string
	width          int
	height         int
	startedAt      time.Time
	expectedWidth  int
	expectedHeight int
	modelOutput    string
}

func initialModel(manager *Manager, initMessage string, width, height int, startedAt time.Time, expectedWidth, expectedHeight int, modelOutput string) tuiModel {
	ti := textinput.New()
	ti.Placeholder = "Type your message or \\command..."
	ti.Prompt = manager.GetPrompt()
	ti.Blur()
	ti.CharLimit = 0

	if width > 0 {
		available := width - 4
		if available < 1 {
			available = 1
		}
		ti.Width = available - lipgloss.Width(ti.Prompt)
		if ti.Width < 1 {
			ti.Width = 1
		}
	} else {
		ti.Width = 0
	}
	promptW := lipgloss.Width(ti.Prompt)
	tuiDebug(manager, "tui.initialModel width=%d height=%d promptW=%d textInput.Width=%d", width, height, promptW, ti.Width)

	if initMessage != "" {
		ti.SetValue(initMessage)
	}

	hp := defaultHistoryPath()
	entries := loadHistory(hp)

	if expectedWidth <= 0 {
		expectedWidth = width
	}
	if expectedHeight <= 0 {
		expectedHeight = height
	}

	return tuiModel{
		textInput:      ti,
		err:            nil,
		manager:        manager,
		initMessage:    initMessage,
		history:        entries,
		histIndex:      -1,
		histPath:       hp,
		width:          width,
		height:         height,
		startedAt:      startedAt,
		expectedWidth:  expectedWidth,
		expectedHeight: expectedHeight,
		modelOutput:    modelOutput,
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
		func() tea.Msg {
			fmt.Print(enableFocusReport)
			return nil
		},
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
		lastFocusActive = true
		lastFocusCheck = time.Now()
		return m, tea.Batch(m.textInput.Focus(), textinput.Blink)
	case tea.BlurMsg:
		lastFocusActive = false
		lastFocusCheck = time.Now()
		m.textInput.Blur()
		return m, nil
	case tea.WindowSizeMsg:
		age := time.Since(m.startedAt)
		if m.expectedWidth > 0 && age < 2*time.Second && msg.Width > 0 && msg.Width < m.expectedWidth-1 {
			if atomic.AddInt32(&windowSizeMsgLogCount, 1) <= 50 {
				tuiDebug(m.manager, "tui.WindowSizeMsg ignored msg=%dx%d cur=%dx%d expected=%dx%d age=%s", msg.Width, msg.Height, m.width, m.height, m.expectedWidth, m.expectedHeight, age)
			}
			return m, nil
		}

		oldModelW, oldModelH := m.width, m.height

		m.width = msg.Width
		m.height = msg.Height

		available := m.width - 4
		if available < 1 {
			available = 1
		}
		m.textInput.Width = available - lipgloss.Width(m.textInput.Prompt)
		if m.textInput.Width < 1 {
			m.textInput.Width = 1
		}

		if atomic.AddInt32(&windowSizeMsgLogCount, 1) <= 50 {
			view := m.View()
			viewW, viewH := lipgloss.Width(view), lipgloss.Height(view)
			promptW := lipgloss.Width(m.textInput.Prompt)
			tuiDebug(m.manager, "tui.WindowSizeMsg applied msg=%dx%d model %dx%d->%dx%d view=%dx%d promptW=%d available=%d textInput.Width=%d expected=%dx%d age=%s", msg.Width, msg.Height, oldModelW, oldModelH, m.width, m.height, viewW, viewH, promptW, available, m.textInput.Width, m.expectedWidth, m.expectedHeight, age)
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
			// No default refocus logic
		}

	case errMsg:
		m.err = msg
		return m, nil
	}

	m.textInput, cmd = m.textInput.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func inputBoxHeight(m tuiModel) int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	boxW := w - 2
	if boxW < 1 {
		boxW = 1
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Margin(0).
		Width(boxW)
	return lipgloss.Height(box.Render(m.textInput.View()))
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
	h := m.height
	if h <= 0 {
		h = 24
	}
	boxW := w - 2
	if boxW < 1 {
		boxW = 1
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Margin(0).
		Width(boxW)
	boxView := box.Render(m.textInput.View())
	boxH := lipgloss.Height(boxView)
	topH := h - boxH
	if topH < 0 {
		topH = 0
	}
	top := renderModelOutput(m.modelOutput, w, topH)
	if top == "" {
		return lipgloss.Place(w, h, lipgloss.Left, lipgloss.Bottom, boxView)
	}
	return lipgloss.JoinVertical(lipgloss.Left, top, boxView)
}

func renderModelOutput(content string, w, h int) string {
	if h <= 0 {
		return ""
	}
	lineStyle := lipgloss.NewStyle().Width(w).MaxWidth(w)
	lines := []string{}
	trimmed := strings.TrimSpace(content)
	if trimmed != "" {
		lines = strings.Split(trimmed, "\n")
	}
	if len(lines) > h {
		lines = lines[len(lines)-h:]
	}
	out := make([]string, 0, h)
	for _, line := range lines {
		out = append(out, lineStyle.Render(line))
	}
	for len(out) < h {
		out = append(out, lineStyle.Render(""))
	}
	return strings.Join(out, "\n")
}

func buildModelOutput(mgr *Manager) string {
	if mgr == nil {
		return ""
	}

	cmdLines := map[string]struct{}{}
	for _, msg := range mgr.Messages {
		if msg.FromUser {
			continue
		}
		lang, code, ok := extractSingleCodeFence(msg.Content)
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(lang)) {
		case "", "sh", "bash", "zsh":
			for _, line := range strings.Split(strings.TrimSpace(code), "\n") {
				v := strings.TrimSpace(line)
				if v == "" {
					continue
				}
				cmdLines[v] = struct{}{}
			}
		}
	}

	var parts []string
	for _, msg := range mgr.Messages {
		if msg.FromUser {
			continue
		}

		lang, code, ok := extractSingleCodeFence(msg.Content)
		if ok {
			switch strings.ToLower(strings.TrimSpace(lang)) {
			case "", "sh", "bash", "zsh":
				rendered := strings.TrimSpace(formatCommandForTUI(code))
				if rendered != "" {
					if len(parts) > 0 {
						parts = append(parts, "")
					}
					parts = append(parts, rendered)
				}
				continue
			}
		}

		visible := stripModelOutputTags(msg.Content)
		visible = strings.TrimSpace(visible)
		if visible == "" {
			continue
		}

		if len(cmdLines) > 0 {
			visible = removeExactLines(visible, cmdLines)
			visible = strings.TrimSpace(collapseBlankLines(visible))
		}
		if visible == "" {
			continue
		}

		if len(parts) > 0 {
			parts = append(parts, "")
		}
		parts = append(parts, formatAIForTUI(system.Cosmetics(visible)))
	}
	return strings.Join(parts, "\n")
}

func extractSingleCodeFence(s string) (string, string, bool) {
	n := strings.ReplaceAll(s, "\r\n", "\n")
	n = strings.TrimSpace(n)
	re := regexp.MustCompile("(?s)^```([a-zA-Z0-9_-]*)\\s*\\n(.*?)\\n```$")
	m := re.FindStringSubmatch(n)
	if m == nil {
		return "", "", false
	}
	lang := strings.TrimSpace(m[1])
	code := strings.TrimRight(m[2], "\n")
	return lang, code, true
}

func removeExactLines(s string, drop map[string]struct{}) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			if _, ok := drop[trimmed]; ok {
				continue
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func formatCommandForTUI(cmd string) string {
	bullet := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true).
		Render("🚀")
	contentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("212"))

	lines := strings.Split(strings.ReplaceAll(cmd, "\r\n", "\n"), "\n")
	bulletWidth := lipgloss.Width(bullet)
	emptyBullet := strings.Repeat(" ", bulletWidth)

	out := make([]string, 0, len(lines))
	for i, line := range lines {
		prefix := bullet
		if i > 0 {
			prefix = emptyBullet
		}
		out = append(out, prefix+" "+contentStyle.Render(line))
	}
	return strings.Join(out, "\n")
}

func formatAIForTUI(msg string) string {
	bullet := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true).
		Render("👾")
	contentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	lines := strings.Split(strings.ReplaceAll(msg, "\r\n", "\n"), "\n")
	bulletWidth := lipgloss.Width(bullet)
	emptyBullet := strings.Repeat(" ", bulletWidth)

	out := make([]string, 0, len(lines))
	for i, line := range lines {
		prefix := bullet
		if i > 0 {
			prefix = emptyBullet
		}
		out = append(out, prefix+" "+contentStyle.Render(line))
	}
	return strings.Join(out, "\n")
}

func stripModelOutputTags(s string) string {
	tags := []string{
		"TmuxSendKeys",
		"ExecCommand",
		"PasteMultilineContent",
		"RequestAccomplished",
		"ExecPaneSeemsBusy",
		"WaitingForUserResponse",
		"NoComment",
		"McpToolCall",
	}
	out := s
	for _, name := range tags {
		out = regexp.MustCompile(fmt.Sprintf("(?s)```(?:xml)?\\s*<%s>.*?</%s>\\s*```", name, name)).ReplaceAllString(out, "")
		out = regexp.MustCompile(fmt.Sprintf("`<%s>.*?</%s>`", name, name)).ReplaceAllString(out, "")
		out = regexp.MustCompile(fmt.Sprintf("(?s)<%s>.*?</%s>", name, name)).ReplaceAllString(out, "")
		out = regexp.MustCompile(fmt.Sprintf("(?s)(<%s>\\s*</%s>|<%s>\\s*|```<%s>```|<%s/>)", name, name, name, name, name)).ReplaceAllString(out, "")
		out = regexp.MustCompile(fmt.Sprintf("(?m)^\\s*(<%s>\\s*|```<%s>```)?\\s*$", name, name)).ReplaceAllString(out, "")
	}
	out = strings.TrimSpace(out)
	out = collapseBlankLines(out)
	return strings.TrimSpace(out)
}

func positionCursorForInputBox(mgr *Manager, view string, where string) {
	bh := lipgloss.Height(view)
	bw := lipgloss.Width(view)
	if atomic.AddInt32(&cursorMoveLogCount, 1) <= 50 {
		tuiDebug(mgr, "tui.cursor.position where=%s view=%dx%d", where, bw, bh)
	}
	fmt.Print("\r")
	fmt.Print("\x1b[999B")
	if bh > 1 {
		fmt.Printf("\x1b[%dA", bh-1)
	}
}

func clearInputBoxLines(mgr *Manager, lines int, where string) {
	if lines <= 0 {
		return
	}
	if atomic.AddInt32(&clearLinesLogCount, 1) <= 50 {
		tuiDebug(mgr, "tui.cursor.clear where=%s lines=%d", where, lines)
	}
	fmt.Print("\r")
	for i := 0; i < lines; i++ {
		fmt.Print("\x1b[2K")
		if i < lines-1 {
			fmt.Print("\x1b[1A\r")
		}
	}
}

// StartTUI starts the Bubble Tea interface
func (c *CLIInterface) StartTUI(initMessage string) error {
	//c.printWelcomeMessage()

	// Initial message handling
	if initMessage != "" {
		fmt.Printf("%s%s\n", c.manager.GetPrompt(), initMessage)
		c.processInput(initMessage)
	} else {
		// Clear screen to hide the command invocation line
		// Use \033[2J to clear screen and \033[1;1H to force cursor to top-left
		fmt.Print("\033[2J\033[1;1H")
	}

	firstRun := true
	for {
		wOut, hOut, errOut := term.GetSize(int(os.Stdout.Fd()))
		wErr, hErr, errErr := term.GetSize(int(os.Stderr.Fd()))
		wIn, hIn, errIn := term.GetSize(int(os.Stdin.Fd()))

		w, h, err := wOut, hOut, errOut
		if err != nil {
			w, h, err = wErr, hErr, errErr
		}
		if err != nil {
			w, h, err = wIn, hIn, errIn
		}
		if err != nil || w == 0 {
			w = 80
			h = 24
		}
		if firstRun {
			tuiDebug(c.manager, "tui.term.GetSize stdout=%dx%d err=%v stderr=%dx%d err=%v stdin=%dx%d err=%v chosen=%dx%d", wOut, hOut, errOut, wErr, hErr, errErr, wIn, hIn, errIn, w, h)
		}

		target := ""
		if c.manager != nil {
			target = c.manager.PaneId
		}
		paneW, paneH := getTmuxPaneSize(target)
		winW, winH := 0, 0
		if firstRun {
			winW, winH = getTmuxWindowSize(target)
			tuiDebug(c.manager, "tui.firstRun sizes term=%dx%d tmuxPane=%dx%d tmuxWin=%dx%d", w, h, paneW, paneH, winW, winH)

			bestPaneW, bestPaneH := paneW, paneH
			bestWinW, bestWinH := winW, winH
			if bestWinW > 0 && bestWinW <= 100 {
				deadline := time.Now().Add(1500 * time.Millisecond)
				for time.Now().Before(deadline) {
					pw, ph := getTmuxPaneSize(target)
					ww, wh := getTmuxWindowSize(target)
					if pw > bestPaneW {
						bestPaneW, bestPaneH = pw, ph
					}
					if ww > bestWinW {
						bestWinW, bestWinH = ww, wh
					}
					if bestWinW > 100 {
						break
					}
					tuiDebug(c.manager, "tui.firstRun wait tmuxPane=%dx%d tmuxWin=%dx%d", pw, ph, ww, wh)
					time.Sleep(50 * time.Millisecond)
				}
				paneW, paneH = bestPaneW, bestPaneH
				winW, winH = bestWinW, bestWinH
				tuiDebug(c.manager, "tui.firstRun settled tmuxPane=%dx%d tmuxWin=%dx%d", paneW, paneH, winW, winH)
			}
		}
		if paneW > 0 {
			if w != paneW || (paneH > 0 && h != paneH) {
				tuiDebug(c.manager, "tui.size override term=%dx%d -> tmuxPane=%dx%d", w, h, paneW, paneH)
			}
			w = paneW
			if paneH > 0 {
				h = paneH
			}

			if firstRun {
				deadline := time.Now().Add(1500 * time.Millisecond)
				for time.Now().Before(deadline) {
					tw, th, err := term.GetSize(int(os.Stdout.Fd()))
					if err == nil && tw > 0 && absInt(tw-paneW) <= 1 {
						w = tw
						h = th
						tuiDebug(c.manager, "tui.firstRun term synced term=%dx%d tmuxPane=%dx%d", tw, th, paneW, paneH)
						break
					}
					time.Sleep(50 * time.Millisecond)
				}
			}
		}

		expectedW, expectedH := paneW, paneH
		if expectedW <= 0 {
			expectedW = w
		}
		if expectedH <= 0 {
			expectedH = h
		}
		startedAt := time.Now()

		firstRun = false

		modelOutput := buildModelOutput(c.manager)
		m0 := initialModel(c.manager, "", w, h, startedAt, expectedW, expectedH, modelOutput)
		p := tea.NewProgram(m0)

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
			fmt.Print(disableFocusReport)

			boxH := inputBoxHeight(m)
			topRow := m.height - boxH + 1
			row := topRow + 1
			if row < 1 {
				row = 1
			}
			if row > m.height {
				row = m.height
			}
			atomic.StoreInt32(&tuiSpinnerRow, int32(row))
			atomic.StoreInt32(&tuiSpinnerEnabled, 1)

			fmt.Print("\033[2J\033[1;1H")
			input := m.textInput.Value()

			// Check for exit/quit
			trimmed := strings.TrimSpace(input)
			if trimmed == "exit" || trimmed == "quit" {
				atomic.StoreInt32(&tuiSpinnerEnabled, 0)
				c.processInput("/exit")
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
			atomic.StoreInt32(&tuiSpinnerEnabled, 0)
		} else {
			fmt.Print(disableFocusReport)
			return nil
		}
	}
}

func getTmuxPaneSize(target string) (int, int) {
	args := []string{"display-message", "-p"}
	if target != "" {
		args = append(args, "-t", target)
	}
	args = append(args, "#{pane_width} #{pane_height}")
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) != 2 {
		return 0, 0
	}
	w, errW := strconv.Atoi(parts[0])
	h, errH := strconv.Atoi(parts[1])
	if errW != nil || errH != nil {
		return 0, 0
	}
	return w, h
}

func getTmuxWindowSize(target string) (int, int) {
	args := []string{"display-message", "-p"}
	if target != "" {
		args = append(args, "-t", target)
	}
	args = append(args, "#{window_width} #{window_height}")
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) != 2 {
		return 0, 0
	}
	w, errW := strconv.Atoi(parts[0])
	h, errH := strconv.Atoi(parts[1])
	if errW != nil || errH != nil {
		return 0, 0
	}
	return w, h
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
