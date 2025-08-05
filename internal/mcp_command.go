package internal

import (
	"fmt"
	"strings"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/system"
)

func handleMcpCommand(m *Manager, args []string) {
	subcommand := "list"
	if len(args) > 0 {
		subcommand = args[0]
	}

	switch subcommand {
	case "list":
		selectMcpServers(m)
	case "current":
		showCurrentMcpServers(m)
	case "help":
		showMcpHelp(m)
	default:
		m.Println(fmt.Sprintf("Unknown /mcp subcommand: %s. Use '/mcp help' for more info.", subcommand))
	}
}

func showMcpHelp(m *Manager) {
	helpText := `
/mcp: Manage MCP (Multi-Context Prompts) servers for the current session.

Available subcommands:

  /mcp or /mcp list
    Show a list of available MCP servers from your config file and interactively select/deselect servers for the current session.

  /mcp current
    Show the list of MCP servers currently selected for this session.

  /mcp help
    Show this help message.
`
	m.Println(helpText)
}

func selectMcpServers(m *Manager) {
	if len(m.Config.Mcp.Servers) == 0 {
		m.Println("No MCP servers configured. Please add servers to your config file.")
		return
	}

	var serverNames []string
	for _, server := range m.Config.Mcp.Servers {
		serverNames = append(serverNames, server.Name)
	}

	// Create a map of currently selected server names for quick lookup
	selectedNames := make(map[string]struct{})
	for _, server := range m.McpServers {
		selectedNames[server.Name] = struct{}{}
	}

	// Run fzf to let the user select/deselect servers
	newlySelectedNames, err := system.InteractiveSelect(serverNames, selectedNames)
	if err != nil {
		m.Println(fmt.Sprintf("Error running interactive selection: %v", err))
		return
	}

	// Update the session's MCP servers based on the new selection
	var updatedMcpServers []config.McpServer
	for _, name := range newlySelectedNames {
		if server, found := findMcpServer(m.Config, name); found {
			updatedMcpServers = append(updatedMcpServers, server)
		}
	}
	m.McpServers = updatedMcpServers

	m.Println("MCP servers for this session have been updated.")
	showCurrentMcpServers(m)
}

func showCurrentMcpServers(m *Manager) {
	if len(m.McpServers) == 0 {
		m.Println("No MCP servers are currently selected for this session.")
		return
	}

	var serverNames []string
	for _, server := range m.McpServers {
		serverNames = append(serverNames, server.Name)
	}

	message := fmt.Sprintf("Current MCP servers for this session: %s", strings.Join(serverNames, ", "))
	m.Println(message)
}

func findMcpServer(cfg *config.Config, name string) (config.McpServer, bool) {
	for _, server := range cfg.Mcp.Servers {
		if server.Name == name {
			return server, true
		}
	}
	return config.McpServer{}, false
}
