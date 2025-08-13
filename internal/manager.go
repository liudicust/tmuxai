package internal

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/logger"
	"github.com/alvinunreal/tmuxai/system"
	"github.com/fatih/color"
)

type AIResponse struct {
	Message                string
	SendKeys               []string
	ExecCommand            []string
	PasteMultilineContent  string
	RequestAccomplished    bool
	ExecPaneSeemsBusy      bool
	WaitingForUserResponse bool
	NoComment              bool
	// 新增MCP工具调用支持
	McpToolCalls []McpToolCall
}

// MCP工具调用结构体
type McpToolCall struct {
	ServerName string                 `json:"server_name"`
	ToolName   string                 `json:"tool_name"`
	Arguments  map[string]interface{} `json:"arguments"`
}

// Parsed only when pane is prepared
type CommandExecHistory struct {
	Command string
	Output  string
	Code    int
}

// Manager represents the TmuxAI manager agent
type Manager struct {
	Config           *config.Config
	AiClient         *AiClient
	Status           string // running, waiting, done
	PaneId           string
	ExecPane         *system.TmuxPaneDetails
	Messages         []ChatMessage
	ExecHistory      []CommandExecHistory
	WatchMode        bool
	OS               string
	SessionOverrides map[string]interface{} // session-only config overrides
	McpServers       []config.McpServer     // currently selected MCP servers for this session
	// 新增MCP客户端
	McpClient *McpClient
}

// NewManager creates a new manager agent
// 在 NewManager 函数中修复 MCP 客户端初始化
func NewManager(cfg *config.Config) (*Manager, error) {
	if cfg.OpenRouter.APIKey == "" {
		fmt.Println("OpenRouter API key is required. Set it in the config file or as an environment variable: TMUXAI_OPENROUTER_API_KEY")
		return nil, fmt.Errorf("OpenRouter API key is required")
	}

	paneId, err := system.TmuxCurrentPaneId()
	if err != nil {
		// If we're not in a tmux session, start a new session and execute the same command
		paneId, err = system.TmuxCreateSession()
		if err != nil {
			return nil, fmt.Errorf("system.TmuxCreateSession failed: %w", err)
		}

		// Create a right pane for chat (current pane becomes exec pane)
		chatPaneId, err := system.TmuxCreateNewPane(paneId)
		if err != nil {
			return nil, fmt.Errorf("failed to create chat pane: %w", err)
		}

		// Switch to the right pane (chat pane) and start tmuxai there
		system.TmuxSelectPane(chatPaneId)
		args := strings.Join(os.Args[1:], " ")
		system.TmuxSendCommandToPane(chatPaneId, "cnp-ai "+args, true)
		system.TmuxSendCommandToPane(chatPaneId, "C-l", false)
		// shell initialization may take some time
		time.Sleep(1 * time.Second)
		system.TmuxSendCommandToPane(paneId, "Enter", false)
		err = system.TmuxAttachSession(paneId)
		if err != nil {
			return nil, fmt.Errorf("system.TmuxAttachSession failed: %w", err)
		}
		os.Exit(0)
	}

	aiClient := NewAiClient(&cfg.OpenRouter)
	os := system.GetOSDetails()

	// 初始化空的 MCP 客户端（不连接任何服务器）
	mcpClient := NewMcpClient([]config.McpServer{})

	manager := &Manager{
		Config:           cfg,
		AiClient:         aiClient,
		PaneId:           paneId,
		Messages:         []ChatMessage{},
		ExecHistory:      []CommandExecHistory{},
		ExecPane:         &system.TmuxPaneDetails{},
		OS:               os,
		SessionOverrides: make(map[string]interface{}),
		McpServers:       []config.McpServer{}, // 改为空数组，用户需要主动选择
		McpClient:        mcpClient,
	}
	manager.InitExecPane()
	return manager, nil
}

// Start starts the manager agent
func (m *Manager) Start(initMessage string) error {
	cliInterface := NewCLIInterface(m)
	if initMessage != "" {
		logger.Info("Initial task provided: %s", initMessage)
	}
	if err := cliInterface.Start(initMessage); err != nil {
		logger.Error("Failed to start CLI interface: %v", err)
		return err
	}
	return nil
}

func (m *Manager) Println(msg string) {
	fmt.Println(m.GetPrompt() + msg)
}

func (m *Manager) GetConfig() *config.Config {
	return m.Config
}

// getPrompt returns the prompt string with color
func (m *Manager) GetPrompt() string {
	tmuxaiColor := color.New(color.FgGreen, color.Bold)
	arrowColor := color.New(color.FgYellow, color.Bold)
	stateColor := color.New(color.FgMagenta, color.Bold)

	var stateSymbol string
	switch m.Status {
	case "running":
		stateSymbol = "▶"
	case "waiting":
		stateSymbol = "?"
	case "done":
		stateSymbol = "✓"
	default:
		stateSymbol = ""
	}
	if m.WatchMode {
		stateSymbol = "∞"
	}

	prompt := tmuxaiColor.Sprint("CNP-AI")
	if stateSymbol != "" {
		prompt += " " + stateColor.Sprint("["+stateSymbol+"]")
	}
	prompt += arrowColor.Sprint(" » ")
	return prompt
}

func (ai *AIResponse) String() string {
	return fmt.Sprintf(`
	Message: %s
	SendKeys: %v
	ExecCommand: %v
	PasteMultilineContent: %s
	RequestAccomplished: %v
	ExecPaneSeemsBusy: %v
	WaitingForUserResponse: %v
	NoComment: %v
`,
		ai.Message,
		ai.SendKeys,
		ai.ExecCommand,
		ai.PasteMultilineContent,
		ai.RequestAccomplished,
		ai.ExecPaneSeemsBusy,
		ai.WaitingForUserResponse,
		ai.NoComment,
	)
}
