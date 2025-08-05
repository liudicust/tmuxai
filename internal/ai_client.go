package internal

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/logger"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// AiClient represents an AI client using Eino framework
type AiClient struct {
	config    *config.OpenRouterConfig
	chatModel model.ToolCallingChatModel
}

// NewAiClient creates a new AI client using Eino framework
func NewAiClient(cfg *config.OpenRouterConfig) *AiClient {
	return &AiClient{
		config: cfg,
	}
}

// initChatModel initializes the Eino ChatModel with OpenRouter configuration
func (c *AiClient) initChatModel(ctx context.Context) error {
	if c.chatModel != nil {
		return nil
	}

	// Configure OpenAI ChatModel to work with OpenRouter
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  c.config.APIKey,
		BaseURL: c.config.BaseURL, // OpenRouter endpoint
		Model:   c.config.Model,
	})
	if err != nil {
		return fmt.Errorf("failed to create chat model: %w", err)
	}

	c.chatModel = chatModel
	return nil
}

// GetResponseFromChatMessages gets a response from the AI based on chat messages
func (c *AiClient) GetResponseFromChatMessages(ctx context.Context, chatMessages []ChatMessage, modelName string) (string, error) {
	// Initialize chat model if not already done
	if err := c.initChatModel(ctx); err != nil {
		return "", err
	}

	// Convert chat messages to Eino schema format
	einoMessages := make([]*schema.Message, 0, len(chatMessages))

	for i, msg := range chatMessages {
		var role schema.RoleType

		if i == 0 && !msg.FromUser {
			role = schema.System
		} else if msg.FromUser {
			role = schema.User
		} else {
			role = schema.Assistant
		}

		einoMessages = append(einoMessages, &schema.Message{
			Role:    role,
			Content: msg.Content,
		})
	}

	logger.Info("Sending %d messages to AI", len(einoMessages))

	// Generate response using Eino ChatModel
	var response *schema.Message
	var err error

	if modelName != "" && modelName != c.config.Model {
		// Override model if specified
		response, err = c.chatModel.Generate(ctx, einoMessages, model.WithModel(modelName))
	} else {
		response, err = c.chatModel.Generate(ctx, einoMessages)
	}

	if err != nil {
		logger.Error("Failed to generate response: %v", err)
		return "", fmt.Errorf("failed to generate response: %w", err)
	}

	responseContent := response.Content
	logger.Debug("Received AI response (%d characters): %s", len(responseContent), responseContent)

	// Debug chat messages if needed
	debugChatMessages(chatMessages, responseContent)

	return responseContent, nil
}

// ChatCompletion provides backward compatibility with the original interface
// Deprecated: Use GetResponseFromChatMessages instead
func (c *AiClient) ChatCompletion(ctx context.Context, messages []Message, modelName string) (string, error) {
	// Convert old Message format to ChatMessage format
	chatMessages := make([]ChatMessage, 0, len(messages))
	for _, msg := range messages {
		fromUser := msg.Role == "user"
		chatMessages = append(chatMessages, ChatMessage{
			Content:   msg.Content,
			FromUser:  fromUser,
			Timestamp: time.Now(),
		})
	}

	return c.GetResponseFromChatMessages(ctx, chatMessages, modelName)
}

// Message represents a chat message (kept for backward compatibility)
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Legacy types kept for backward compatibility
type ChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type ChatCompletionChoice struct {
	Index   int     `json:"index"`
	Message Message `json:"message"`
}

type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Choices []ChatCompletionChoice `json:"choices"`
}

func debugChatMessages(chatMessages []ChatMessage, response string) {
	timestamp := time.Now().Format("20060102-150405")
	configDir, _ := config.GetConfigDir()

	debugDir := fmt.Sprintf("%s/debug", configDir)
	if _, err := os.Stat(debugDir); os.IsNotExist(err) {
		os.Mkdir(debugDir, 0755)
	}

	debugFileName := fmt.Sprintf("%s/debug-%s.txt", debugDir, timestamp)

	file, err := os.Create(debugFileName)
	if err != nil {
		logger.Error("Failed to create debug file: %v", err)
		return
	}
	defer file.Close()

	file.WriteString("==================    SENT CHAT MESSAGES ==================\n\n")

	for i, msg := range chatMessages {
		role := "assistant"
		if msg.FromUser {
			role = "user"
		}
		if i == 0 && !msg.FromUser {
			role = "system"
		}
		timeStr := msg.Timestamp.Format(time.RFC3339)

		file.WriteString(fmt.Sprintf("Message %d: Role=%s, Time=%s\n", i+1, role, timeStr))
		file.WriteString(fmt.Sprintf("Content:\n%s\n\n", msg.Content))
	}

	file.WriteString("==================    RECEIVED RESPONSE ==================\n\n")
	file.WriteString(response)
	file.WriteString("\n\n==================    END DEBUG ==================\n")
}
