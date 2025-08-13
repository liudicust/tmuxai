package internal

type McpServer struct {
	Name       string `yaml:"name"`
	URL        string `yaml:"url"`
	APIKey     string `yaml:"api_key"`
	Model      string `yaml:"model"`
	BaseURL    string `yaml:"base_url"`
	Type       string `yaml:"type"` // "stdio", "sse", "streamable-http"
	StreamMode bool   `yaml:"stream_mode"`
	Timeout    int    `yaml:"timeout"`
	RetryCount int    `yaml:"retry_count"`
	// 新增字段
	Args    []string          `yaml:"args"`    // stdio 类型的命令参数
	Env     []string          `yaml:"env"`     // stdio 类型的环境变量
	Headers map[string]string `yaml:"headers"` // HTTP 类型的请求头
	Command string            `yaml:"command"` // stdio 类型的命令
}
