package system

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/alvinunreal/tmuxai/logger"
	"golang.org/x/term"
)

// TmuxCreateNewPane creates a new vertical split pane (top/bottom) in the specified window and returns its ID
// The new pane is created as the bottom pane, leaving the current pane on top.
func TmuxCreateNewPane(target string) (string, error) {
	tryCreate := func(args ...string) (string, string, error) {
		cmd := exec.Command("tmux", args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return strings.TrimSpace(stdout.String()), stderr.String(), err
	}

	paneId, stderrStr, err := tryCreate("split-window", "-d", "-v", "-p", "30", "-t", target, "-P", "-F", "#{pane_id}")
	if err == nil {
		return paneId, nil
	}

	if strings.Contains(stderrStr, "size missing") {
		logger.Info("tmux split-window percent failed (size missing), retrying without -p")
		paneId2, stderrStr2, err2 := tryCreate("split-window", "-d", "-v", "-t", target, "-P", "-F", "#{pane_id}")
		if err2 == nil {
			return paneId2, nil
		}
		logger.Error("Failed to create tmux pane after retry: %v, stderr: %s", err2, stderrStr2)
		return "", err2
	}

	logger.Error("Failed to create tmux pane: %v, stderr: %s", err, stderrStr)
	return "", err
}

// TmuxPanesDetails gets details for all panes in a target window
func TmuxPanesDetails(target string) ([]TmuxPaneDetails, error) {
	cmd := exec.Command("tmux", "list-panes", "-t", target, "-F", "#{pane_id},#{pane_active},#{pane_pid},#{pane_current_command},#{history_size},#{history_limit}")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		logger.Error("Failed to get tmux pane details for target %s %v, stderr: %s", target, err, stderr.String())
		return nil, err
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, fmt.Errorf("no pane details found for target %s", target)
	}

	lines := strings.Split(output, "\n")
	paneDetails := make([]TmuxPaneDetails, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ",", 6)
		if len(parts) < 5 {
			logger.Error("Invalid pane details format for line: %s", line)
			continue
		}

		id := parts[0]

		// If target starts with '%', it's a pane ID, so only include the matching pane
		if strings.HasPrefix(target, "%") && id != target {
			continue
		}

		active, _ := strconv.Atoi(parts[1])
		pid, _ := strconv.Atoi(parts[2])
		historySize, _ := strconv.Atoi(parts[4])
		historyLimit, _ := strconv.Atoi(parts[5])
		currentCommandArgs := GetProcessArgs(pid)
		isSubShell := IsSubShell(parts[3])

		paneDetail := TmuxPaneDetails{
			Id:                 id,
			IsActive:           active,
			CurrentPid:         pid,
			CurrentCommand:     parts[3],
			CurrentCommandArgs: currentCommandArgs,
			HistorySize:        historySize,
			HistoryLimit:       historyLimit,
			IsSubShell:         isSubShell,
		}

		paneDetails = append(paneDetails, paneDetail)
	}

	return paneDetails, nil
}

// TmuxCapturePane gets the content of a specific pane by ID
func TmuxCapturePane(paneId string, maxLines int) (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-p", "-t", paneId, "-S", fmt.Sprintf("-%d", maxLines))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		logger.Error("Failed to capture pane content from %s: %v, stderr: %s", paneId, err, stderr.String())
		return "", err
	}

	content := strings.TrimSpace(stdout.String())
	return content, nil
}

// Return current tmux window target with session id and window id
func TmuxCurrentWindowTarget() (string, error) {
	paneId, err := TmuxCurrentPaneId()
	if err != nil {
		return "", err
	}

	cmd := exec.Command("tmux", "list-panes", "-t", paneId, "-F", "#{session_id}:#{window_index}")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get window target: %w", err)
	}

	target := strings.TrimSpace(string(output))
	if target == "" {
		return "", fmt.Errorf("empty window target returned")
	}

	if idx := strings.Index(target, "\n"); idx != -1 {
		target = target[:idx]
	}

	return target, nil
}

func TmuxCurrentPaneId() (string, error) {
	tmuxPane := os.Getenv("TMUX_PANE")
	if tmuxPane == "" {
		return "", fmt.Errorf("TMUX_PANE environment variable not set")
	}

	return tmuxPane, nil
}

// CreateTmuxSession creates a new tmux session and returns the new pane id
func TmuxCreateSession() (string, error) {
	width, height := 120, 40
	if term.IsTerminal(int(os.Stdout.Fd())) {
		if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 && h > 0 {
			width, height = w, h
		}
	} else if term.IsTerminal(int(os.Stderr.Fd())) {
		if w, h, err := term.GetSize(int(os.Stderr.Fd())); err == nil && w > 0 && h > 0 {
			width, height = w, h
		}
	}

	cmd := exec.Command(
		"tmux",
		"new-session",
		"-d",
		"-x",
		strconv.Itoa(width),
		"-y",
		strconv.Itoa(height),
		"-P",
		"-F",
		"#S",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		logger.Error("Failed to create tmux session: %v, stderr: %s", err, stderr.String())
		return "", err
	}

	sessionName := strings.TrimSpace(stdout.String())
	if sessionName == "" {
		return "", fmt.Errorf("empty session name returned")
	}

	windowTarget := sessionName + ":0"
	listCmd := exec.Command("tmux", "list-panes", "-t", windowTarget, "-F", "#{pane_id}")
	var listOut, listErr bytes.Buffer
	listCmd.Stdout = &listOut
	listCmd.Stderr = &listErr
	if err := listCmd.Run(); err == nil {
		if paneId := strings.TrimSpace(listOut.String()); paneId != "" {
			if idx := strings.Index(paneId, "\n"); idx != -1 {
				paneId = paneId[:idx]
			}
			return paneId, nil
		}
	}

	fallbackCmd := exec.Command("tmux", "list-panes", "-t", windowTarget, "-F", "#S:#I.#P")
	fallbackCmd.Stdout = &listOut
	fallbackCmd.Stderr = &listErr
	if err := fallbackCmd.Run(); err != nil {
		logger.Error("Failed to resolve tmux pane target: %v, stderr: %s", err, listErr.String())
		return "", err
	}

	paneTarget := strings.TrimSpace(listOut.String())
	if paneTarget == "" {
		return "", fmt.Errorf("empty pane target returned")
	}
	if idx := strings.Index(paneTarget, "\n"); idx != -1 {
		paneTarget = paneTarget[:idx]
	}
	return paneTarget, nil
}

// AttachToTmuxSession attaches to an existing tmux session
func TmuxAttachSession(target string) error {
	sessionTarget := ""
	resolveCmd := exec.Command("tmux", "list-panes", "-t", target, "-F", "#S")
	var out, stderr bytes.Buffer
	resolveCmd.Stdout = &out
	resolveCmd.Stderr = &stderr
	if err := resolveCmd.Run(); err == nil {
		sessionTarget = strings.TrimSpace(out.String())
		if idx := strings.Index(sessionTarget, "\n"); idx != -1 {
			sessionTarget = sessionTarget[:idx]
		}
	}
	if sessionTarget == "" {
		sessionTarget = target
	}

	cmd := exec.Command("tmux", "attach-session", "-t", sessionTarget)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logger.Error("Failed to attach to tmux session: %v", err)
		return err
	}
	return nil
}

func TmuxClearPane(paneId string) error {
	cmd := exec.Command("tmux", "clear-history", "-t", paneId)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		logger.Error("Failed to clear history for pane %s: %v, stderr: %s", paneId, err, stderr.String())
		return err
	}

	if current := os.Getenv("TMUX_PANE"); current != "" && current == paneId {
		fmt.Print("\033[2J\033[1;1H")
		logger.Debug("Successfully cleared current pane %s", paneId)
		return nil
	}

	if err := TmuxSendCommandToPane(paneId, "C-l", false); err != nil {
		return err
	}

	logger.Debug("Successfully cleared pane %s", paneId)
	return nil
}

// TmuxSelectPane selects a specific pane
func TmuxSelectPane(paneId string) error {
	cmd := exec.Command("tmux", "select-pane", "-t", paneId)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		logger.Error("Failed to select tmux pane %s: %v, stderr: %s", paneId, err, stderr.String())
		return err
	}

	logger.Debug("Successfully selected pane %s", paneId)
	return nil
}
