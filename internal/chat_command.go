package internal

import (
	"fmt"
	"os"
	"strings"

	"github.com/alvinunreal/tmuxai/logger"
	"github.com/alvinunreal/tmuxai/system"
)

const helpMessage = `Available commands:
- /info: Display system information
- /clear: Clear the chat history
- /reset: Reset the chat history
- /prepare: Prepare the pane for TmuxAI automation
- /watch <prompt>: Start watch mode
- /squash: Summarize the chat history
- /mcp: Manage MCP servers for the current session
- /exit: Exit the application`

var commands = []string{
	"/help",
	"/clear",
	"/reset",
	"/exit",
	"/info",
	"/watch",
	"/prepare",
	"/config",
	"/squash",
	"/mcp",
}

// checks if the given content is a command
func (m *Manager) IsMessageSubcommand(content string) bool {
	content = strings.TrimSpace(strings.ToLower(content)) // Normalize input

	// Any message starting with / is considered a command
	return strings.HasPrefix(content, "/")
}

// processes a command and returns a response
func (m *Manager) ProcessSubCommand(command string) {
	commandLower := strings.ToLower(strings.TrimSpace(command))
	logger.Info("Processing command: %s", command)

	// Get the first word from the command (e.g., "/watch" from "/watch something")
	parts := strings.Fields(commandLower)
	if len(parts) == 0 {
		m.Println("Empty command")
		return
	}

	commandPrefix := parts[0]

	// Process the command using prefix matching
	switch {
	case prefixMatch(commandPrefix, "/help"):
		m.Println(helpMessage)
		return

	case prefixMatch(commandPrefix, "/info"):
		m.formatInfo()
		return

	case prefixMatch(commandPrefix, "/prepare"):
		m.InitExecPane()
		m.PrepareExecPane()
		m.Messages = []ChatMessage{}
		if m.ExecPane.IsPrepared {
			m.Println("Exec pane prepared successfully")
		}
		fmt.Println(m.ExecPane.String())
		m.parseExecPaneCommandHistory()

		logger.Debug("Parsed exec history:")
		for _, history := range m.ExecHistory {
			logger.Debug(fmt.Sprintf("Command: %s\nOutput: %s\nCode: %d\n", history.Command, history.Output, history.Code))
		}

		return

	case prefixMatch(commandPrefix, "/clear"):
		m.Messages = []ChatMessage{}
		system.TmuxClearPane(m.PaneId)
		return

	case prefixMatch(commandPrefix, "/reset"):
		m.Status = ""
		m.Messages = []ChatMessage{}
		system.TmuxClearPane(m.PaneId)
		system.TmuxClearPane(m.ExecPane.Id)
		return

	case prefixMatch(commandPrefix, "/exit"):
		logger.Info("Exit command received, stopping watch mode (if active) and exiting.")
		os.Exit(0)
		return

	case prefixMatch(commandPrefix, "/squash"):
		m.squashHistory()
		return

	case prefixMatch(commandPrefix, "/watch") || commandPrefix == "/w":
		parts := strings.Fields(command)
		if len(parts) > 1 {
			watchDesc := strings.Join(parts[1:], " ")
			startWatch := `
1. Find out if there is new content in the pane based on chat history.
2. Comment only considering the new content in this pane output.

Watch for: ` + watchDesc
			m.Status = "running"
			m.WatchMode = true
			m.startWatchMode(startWatch)
			return
		}
		m.Println("Usage: /watch <description>")
		return

	case prefixMatch(commandPrefix, "/config"):
		handleConfigCommand(m, parts[1:])
		return

	case prefixMatch(commandPrefix, "/mcp"):
		handleMcpCommand(m, parts[1:])
		return

	default:
		m.Println(fmt.Sprintf("Unknown command: %s. Use '/help' for more info.", commandPrefix))
	}
}

// Helper function to check if a command matches a prefix
func prefixMatch(command, target string) bool {
	return strings.HasPrefix(target, command)
}

// formats system information and tmux details into a readable string
func (m *Manager) formatInfo() {
	formatter := system.NewInfoFormatter()
	const labelWidth = 18 // Width of the label column
	formatLine := func(key string, value any) {
		fmt.Print(formatter.LabelColor.Sprintf("%-*s", labelWidth, key))
		fmt.Print("  ")
		fmt.Println(value)
	}
	// Display general information
	fmt.Println(formatter.FormatSection("\nGeneral"))
	formatLine("Version", Version)
	formatLine("Max Capture Lines", m.Config.MaxCaptureLines)
	formatLine("Wait Interval", m.Config.WaitInterval)

	// Display context information section
	fmt.Println(formatter.FormatSection("\nContext"))
	formatLine("Messages", len(m.Messages))
	var totalTokens int
	for _, msg := range m.Messages {
		totalTokens += system.EstimateTokenCount(msg.Content)
	}

	usagePercent := 0.0
	if m.GetMaxContextSize() > 0 {
		usagePercent = float64(totalTokens) / float64(m.GetMaxContextSize()) * 100
	}
	fmt.Print(formatter.LabelColor.Sprintf("%-*s", labelWidth, "Context Size~"))
	fmt.Print("  ") // Two spaces for separation
	fmt.Printf("%s\n", fmt.Sprintf("%d tokens", totalTokens))
	fmt.Printf("%-*s  %s\n", labelWidth, "", formatter.FormatProgressBar(usagePercent, 10))
	formatLine("Max Size", fmt.Sprintf("%d tokens", m.GetMaxContextSize()))

	// Display tmux panes section
	fmt.Println()
	fmt.Println(formatter.FormatSection("Tmux Window Panes"))

	panes, _ := m.GetTmuxPanes()
	for _, pane := range panes {
		pane.Refresh(m.GetMaxCaptureLines())
		fmt.Println(pane.FormatInfo(formatter))
	}
}

// handleConfigCommand processes /config subcommands
func handleConfigCommand(m *Manager, args []string) {
	if len(args) == 0 {
		m.Println("Usage: /config <get|set> [key] [value]")
		return
	}

	subcommand := args[0]
	switch subcommand {
	case "get":
		if len(args) == 1 {
			// Show all config
			m.Println("Current configuration:")
			m.Println(m.FormatConfig())
		} else if len(args) == 2 {
			// Show specific config key
			key := args[1]
			if !isAllowedConfigKey(key) {
				m.Println(fmt.Sprintf("Config key '%s' is not allowed to be modified. Allowed keys: %s", key, strings.Join(AllowedConfigKeys, ", ")))
				return
			}
			value := getConfigValue(m, key)
			m.Println(fmt.Sprintf("%s: %v", key, value))
		} else {
			m.Println("Usage: /config get [key]")
		}

	case "set":
		if len(args) != 3 {
			m.Println("Usage: /config set <key> <value>")
			return
		}
		key := args[1]
		value := args[2]

		if !isAllowedConfigKey(key) {
			m.Println(fmt.Sprintf("Config key '%s' is not allowed to be modified. Allowed keys: %s", key, strings.Join(AllowedConfigKeys, ", ")))
			return
		}

		if err := setConfigValue(m, key, value); err != nil {
			m.Println(fmt.Sprintf("Error setting config: %v", err))
			return
		}

		m.Println(fmt.Sprintf("Set %s = %s", key, value))

	default:
		m.Println(fmt.Sprintf("Unknown /config subcommand: %s. Use 'get' or 'set'.", subcommand))
	}
}

// isAllowedConfigKey checks if a config key is allowed to be modified
func isAllowedConfigKey(key string) bool {
	for _, allowedKey := range AllowedConfigKeys {
		if allowedKey == key {
			return true
		}
	}
	return false
}

// getConfigValue gets the current value of a config key
func getConfigValue(m *Manager, key string) interface{} {
	// Check session overrides first
	if override, exists := m.SessionOverrides[key]; exists {
		return override
	}

	// Get from config
	switch key {
	case "max_capture_lines":
		return m.Config.MaxCaptureLines
	case "max_context_size":
		return m.Config.MaxContextSize
	case "wait_interval":
		return m.Config.WaitInterval
	case "send_keys_confirm":
		return m.Config.SendKeysConfirm
	case "paste_multiline_confirm":
		return m.Config.PasteMultilineConfirm
	case "exec_confirm":
		return m.Config.ExecConfirm
	case "openrouter.model":
		return m.Config.OpenRouter.Model
	default:
		return nil
	}
}

// setConfigValue sets a config value as a session override
func setConfigValue(m *Manager, key, value string) error {
	if m.SessionOverrides == nil {
		m.SessionOverrides = make(map[string]interface{})
	}

	// Parse value based on the expected type
	switch key {
	case "max_capture_lines", "max_context_size", "wait_interval":
		var intVal int
		if _, err := fmt.Sscanf(value, "%d", &intVal); err != nil {
			return fmt.Errorf("invalid integer value: %s", value)
		}
		m.SessionOverrides[key] = intVal
	case "send_keys_confirm", "paste_multiline_confirm", "exec_confirm":
		var boolVal bool
		if _, err := fmt.Sscanf(value, "%t", &boolVal); err != nil {
			return fmt.Errorf("invalid boolean value: %s (use true or false)", value)
		}
		m.SessionOverrides[key] = boolVal
	case "openrouter.model":
		m.SessionOverrides[key] = value
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}

	return nil
}
