package internal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/logger"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

type McpClient struct {
	clients map[string]*client.Client
	mu      sync.RWMutex
}

func NewMcpClient(servers []config.McpServer) *McpClient {
	mc := &McpClient{
		clients: make(map[string]*client.Client),
	}

	for _, server := range servers {
		var trans transport.Interface
		var err error

		// 创建传输层
		switch server.Type {
		case "stdio":
			// 将 map[string]string 转换为 []string 格式
			var envSlice []string
			for key, value := range server.Env {
				envSlice = append(envSlice, fmt.Sprintf("%s=%s", key, value))
			}
			trans = transport.NewStdio(server.Command, envSlice, server.Args...)
		case "sse":
			trans, err = transport.NewSSE(server.URL)
			if err != nil {
				logger.Error("Failed to create SSE transport for server %s: %v", server.Name, err)
				continue
			}
		case "streamable-http", "streamableHTTP", "http":
			trans, err = transport.NewStreamableHTTP(server.URL)
			if err != nil {
				logger.Error("Failed to create StreamableHTTP transport for server %s: %v", server.Name, err)
				continue
			}
		default:
			logger.Error("Unsupported MCP server type: %s for server %s", server.Type, server.Name)
			continue
		}

		// 创建客户端
		mcpClient := client.NewClient(trans)

		// 启动客户端
		err = mcpClient.Start(context.Background())
		if err != nil {
			logger.Error("Failed to start MCP client for server %s: %v", server.Name, err)
			continue
		}

		// 设置通知处理（可选）
		mcpClient.OnNotification(func(notification mcp.JSONRPCNotification) {
			// 处理通知，目前为空实现
		})

		// 初始化客户端
		_, err = mcpClient.Initialize(context.Background(), mcp.InitializeRequest{})
		if err != nil {
			logger.Error("Failed to initialize MCP client for server %s: %v", server.Name, err)
			continue
		}

		// 存储客户端
		mc.clients[server.Name] = mcpClient
		logger.Info("Successfully connected to MCP server: %s", server.Name)
	}

	return mc
}

func (mc *McpClient) CallTool(serverName, toolName string, arguments map[string]interface{}) (string, error) {
	mc.mu.RLock()
	client, exists := mc.clients[serverName]
	mc.mu.RUnlock()
	logger.Info("CallTool clients: %v", mc.clients)
	if !exists {
		return "", fmt.Errorf("MCP server '%s' not found", serverName)
	}
	toolName = serverName + "-" + toolName

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: arguments,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := client.CallTool(ctx, request)
	if err != nil {
		return "", fmt.Errorf("failed to call tool '%s' on server '%s': %v", toolName, serverName, err)
	}

	if result.IsError {
		if len(result.Content) > 0 {
			if textContent, ok := result.Content[0].(mcp.TextContent); ok {
				return "", fmt.Errorf("tool execution error: %s", textContent.Text)
			}
		}
		return "", fmt.Errorf("tool execution error")
	}

	if len(result.Content) > 0 {
		if textContent, ok := result.Content[0].(mcp.TextContent); ok {
			return textContent.Text, nil
		}
	}

	return "", nil
}

func (mc *McpClient) ListTools(serverName string) ([]string, error) {
	mc.mu.RLock()
	client, exists := mc.clients[serverName]
	mc.mu.RUnlock()

	fmt.Println("MCP clients:", mc.clients)
	if !exists {
		return nil, fmt.Errorf("MCP server '%s' not found", serverName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	response, err := client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list tools for server '%s': %v", serverName, err)
	}

	var toolNames []string
	for _, tool := range response.Tools {
		toolNames = append(toolNames, tool.Name)
	}

	return toolNames, nil
}

func (mc *McpClient) GetToolInfo(serverName, toolName string) (map[string]interface{}, error) {
	mc.mu.RLock()
	client, exists := mc.clients[serverName]
	mc.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("MCP server '%s' not found", serverName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tools, err := client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list tools from server '%s': %v", serverName, err)
	}

	for _, tool := range tools.Tools {
		if tool.Name == toolName {
			toolInfo := map[string]interface{}{
				"name":        tool.Name,
				"description": tool.Description,
				"inputSchema": tool.InputSchema,
			}
			return toolInfo, nil
		}
	}

	return nil, fmt.Errorf("tool '%s' not found on server '%s'", toolName, serverName)
}

func (mc *McpClient) IsConnected(serverName string) bool {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	_, exists := mc.clients[serverName]
	return exists
}

func (mc *McpClient) GetConnectedServers() []string {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	servers := make([]string, 0, len(mc.clients))
	for serverName := range mc.clients {
		servers = append(servers, serverName)
	}
	return servers
}

func (mc *McpClient) Close() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	for serverName, client := range mc.clients {
		if err := client.Close(); err != nil {
			fmt.Printf("Error closing MCP client for server %s: %v\n", serverName, err)
		}
	}
	mc.clients = make(map[string]*client.Client)
	return nil
}
