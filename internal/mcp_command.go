package internal

import (
	"fmt"
	"strings"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/system"
	"github.com/fatih/color"
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

	// 关闭旧的 MCP 客户端连接
	m.McpClient.Close()

	// 重新初始化 MCP 客户端，只连接选中的服务器
	m.McpClient = NewMcpClient(updatedMcpServers)

	showCurrentMcpServers(m)
}

func showCurrentMcpServers(m *Manager) {
	if len(m.McpServers) == 0 {
		m.Println("No MCP servers are currently selected for this session.")
		return
	}

	var serverNames []string
	for _, server := range m.McpServers {
		// 尝试获取服务器的工具列表
		tools, err := m.McpClient.ListTools(server.Name)
		if err != nil {
			fmt.Println("Error listing tools:", err.Error())
			serverNames = append(serverNames, fmt.Sprintf("%s (tools: unavailable)", server.Name))
		} else {
			serverNames = append(serverNames, fmt.Sprintf("%s (tools: %d available)", server.Name, len(tools)))
		}
	}

	arrowColor := color.New(color.FgYellow, color.Bold)
	serverList := arrowColor.Sprint(strings.Join(serverNames, ", "))
	message := fmt.Sprintf("🧰 Current MCP servers for this session: %s", serverList)
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
