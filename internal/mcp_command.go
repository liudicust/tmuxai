package internal

import (
	"errors"
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
		m.Println(fmt.Sprintf("Unknown /mcp subcommand: %s. Use '/mcp help' for more info.", subcommand), StyleError)
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
	m.Println(helpText, StyleInfo)
}

func selectMcpServers(m *Manager) {
	if len(m.Config.Mcp.Servers) == 0 {
		m.Println("No MCP servers configured. Please add servers to your config file.", StyleInfo)
		return
	}

	// 仅构建服务器名称列表（不再预先加载工具列表）
	var serverNames []string
	for _, server := range m.Config.Mcp.Servers {
		serverNames = append(serverNames, server.Name)
	}

	// 使用历史选择作为预选
	preSelectedTools := m.SelectedMcpTools

	// 懒加载函数：当用户选择了某个服务器后，再去拉取该服务器的工具
	loadTools := func(serverName string) ([]system.ToolInfo, error) {
		server, found := findMcpServer(m.Config, serverName)
		if !found {
			return nil, fmt.Errorf("server '%s' not found", serverName)
		}

		// 仅连接该服务器，拉取工具信息
		tempClient := NewMcpClient([]config.McpServer{server})
		defer tempClient.Close()

		toolNames, err := tempClient.ListTools(serverName)
		if err != nil {
			return nil, err
		}

		var tools []system.ToolInfo
		for _, toolName := range toolNames {
			description := "No description"
			if info, err := tempClient.GetToolInfo(serverName, toolName); err == nil {
				if desc, ok := info["description"].(string); ok && desc != "" {
					description = desc
				}
			}
			tools = append(tools, system.ToolInfo{
				Name:        toolName,
				Description: description, // 图形界面只使用描述
			})
		}
		return tools, nil
	}

	// 执行两步选择（懒加载工具）
	selections, err := system.InteractiveSelectServersAndTools(serverNames, preSelectedTools, loadTools)
	if err != nil {
		if errors.Is(err, system.ErrUserCancelledSelection) {
			return
		}
		m.Println(fmt.Sprintf("Error in server and tool selection: %v", err), StyleError)
		return
	}

	if len(selections) == 0 {
		m.Println("No servers selected.")
		// 清空当前选择
		m.McpServers = []config.McpServer{}
		m.SelectedMcpTools = make(map[string][]string)
		m.McpClient.Close()
		m.McpClient = NewMcpClient([]config.McpServer{})
		return
	}

	// 更新选中的服务器和工具
	var selectedServers []config.McpServer
	newSelectedTools := make(map[string][]string)

	for _, selection := range selections {
		if server, found := findMcpServer(m.Config, selection.ServerName); found {
			selectedServers = append(selectedServers, server)
			newSelectedTools[selection.ServerName] = selection.SelectedTools
		}
	}

	// 更新Manager状态
	m.McpServers = selectedServers
	m.SelectedMcpTools = newSelectedTools

	// 重新初始化MCP客户端（仅连接选中的服务器）
	m.McpClient.Close()
	m.McpClient = NewMcpClient(selectedServers)

	showCurrentMcpServers(m)
}

func showCurrentMcpServers(m *Manager) {
	if len(m.McpServers) == 0 {
		m.Println("No MCP servers are currently selected for this session.")
		return
	}

	var serverDetails []string
	for _, server := range m.McpServers {
		selectedTools := m.SelectedMcpTools[server.Name]
		serverDetails = append(serverDetails, fmt.Sprintf("%s (%d tools selected)", server.Name, len(selectedTools)))
	}

	arrowColor := color.New(color.FgYellow, color.Bold)
	serverList := arrowColor.Sprint(strings.Join(serverDetails, ", "))
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
