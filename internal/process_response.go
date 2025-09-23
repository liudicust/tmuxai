package internal

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/alvinunreal/tmuxai/logger"
	"github.com/kaptinlin/jsonrepair"
)

func (m *Manager) parseAIResponse(response string) (AIResponse, error) {
	logger.Info("parseAIResponse response: %s", response)
	// Tag mapping: tag name -> field
	type tagInfo struct {
		name     string
		isArray  bool
		isBool   bool
		setField func(*AIResponse, string)
	}
	tags := []tagInfo{
		{"TmuxSendKeys", true, false, func(r *AIResponse, v string) { r.SendKeys = append(r.SendKeys, v) }},
		{"ExecCommand", true, false, func(r *AIResponse, v string) { r.ExecCommand = append(r.ExecCommand, v) }},
		{"PasteMultilineContent", false, false, func(r *AIResponse, v string) { r.PasteMultilineContent = v }},
		{"RequestAccomplished", false, true, func(r *AIResponse, v string) { r.RequestAccomplished = isTrue(v) }},
		{"ExecPaneSeemsBusy", false, true, func(r *AIResponse, v string) { r.ExecPaneSeemsBusy = isTrue(v) }},
		{"WaitingForUserResponse", false, true, func(r *AIResponse, v string) { r.WaitingForUserResponse = isTrue(v) }},
		{"NoComment", false, true, func(r *AIResponse, v string) { r.NoComment = isTrue(v) }},
		// 新增MCP工具调用标签
		{"McpToolCall", true, false, func(r *AIResponse, v string) {
			if toolCall, err := parseMcpToolCall(v); err == nil {
				r.McpToolCalls = append(r.McpToolCalls, toolCall)
			}
		}},
	}

	clean := response
	tagPattern := `(?s)<%s>(.*?)</%s>`
	r := AIResponse{}
	cleanForMsg := clean
	for _, t := range tags {
		reTag := regexp.MustCompile(fmt.Sprintf(tagPattern, t.name, t.name))
		tagMatches := reTag.FindAllStringSubmatch(clean, -1)
		for _, m := range tagMatches {
			// m[0] is the full match, m[1] is the value
			if len(m) < 2 {
				continue // skip invalid match
			}
			val := strings.TrimSpace(m[1])
			// Decode XML entities for non-bool tags
			if !t.isBool {
				val = html.UnescapeString(val)
			}

			if t.isArray {
				t.setField(&r, val)
			} else {
				t.setField(&r, val)
			}
		}
		// For message: remove all tag blocks, including code/backtick wrappers
		// Remove code block: ```xml\n<tag>...</tag>\n```, ```\n<tag>...</tag>\n```
		cleanForMsg = regexp.MustCompile(fmt.Sprintf("(?s)```(?:xml)?\\s*<%s>.*?</%s>\\s*```", t.name, t.name)).ReplaceAllString(cleanForMsg, "")
		// Remove single backtick-wrapped tags: `<Tag>...</Tag>`
		cleanForMsg = regexp.MustCompile(fmt.Sprintf("`<%s>.*?</%s>`", t.name, t.name)).ReplaceAllString(cleanForMsg, "")
		// Remove plain tag: <Tag>...</Tag>
		cleanForMsg = reTag.ReplaceAllString(cleanForMsg, "")
	}

	// Special handling: tags that may appear as <TagName> or ```<TagName>``` (no value)
	// Set bool fields to true if such tag is present, even if no value
	for _, t := range tags {
		if !t.isBool {
			continue
		}
		// Match <TagName> or ```<TagName>```
		pat := fmt.Sprintf("(?s)(<%s>\\s*</%s>|<%s>\\s*|```<%s>```|<%s/>)", t.name, t.name, t.name, t.name, t.name)
		if regexp.MustCompile(pat).MatchString(clean) {
			t.setField(&r, "1")
		}
	}

	// Message: trim, collapse multiple newlines
	msg := strings.TrimSpace(cleanForMsg)
	msg = collapseBlankLines(msg)
	// Remove any leftover tag lines (e.g. <TagName>) that may not have been removed
	for _, t := range tags {
		// Remove lines that are just <TagName> or ```<TagName>```
		reLeftover := regexp.MustCompile(fmt.Sprintf("(?m)^\\s*(<%s>\\s*|```<%s>```)?\\s*$", t.name, t.name))
		msg = reLeftover.ReplaceAllString(msg, "")
	}
	msg = strings.TrimSpace(msg)
	r.Message = msg

	return r, nil
}

// 解析MCP工具调用
func parseMcpToolCall(content string) (McpToolCall, error) {
	logger.Info("parseMcpToolCall content: %s", content)

	var toolCall McpToolCall
	var lastErr error

	// 策略1: 标准JSON解析（原有逻辑）
	repairedJSON, err := jsonrepair.JSONRepair(content)
	if err != nil {
		logger.Error("Failed to repair JSON: %v", err)
		repairedJSON = content
	} else {
		logger.Info("JSON repaired from: %s to: %s", content, repairedJSON)
	}

	err = json.Unmarshal([]byte(repairedJSON), &toolCall)
	if err == nil && toolCall.ServerName != "" && toolCall.ToolName != "" {
		logger.Info("parseMcpToolCall success with standard parsing: %v", toolCall)
		return toolCall, nil
	}
	lastErr = err

	// 策略2: 尝试从代码块中提取JSON
	codeBlockJSON := extractJSONFromCodeBlock(content)
	if codeBlockJSON != "" {
		logger.Info("Trying to parse JSON from code block: %s", codeBlockJSON)
		err = json.Unmarshal([]byte(codeBlockJSON), &toolCall)
		if err == nil && toolCall.ServerName != "" && toolCall.ToolName != "" {
			logger.Info("parseMcpToolCall success with code block extraction: %v", toolCall)
			return toolCall, nil
		}
		lastErr = err
	}

	// 策略3: 正则表达式提取关键字段
	regexToolCall, regexErr := extractWithRegex(content)
	if regexErr == nil && regexToolCall.ServerName != "" && regexToolCall.ToolName != "" {
		logger.Info("parseMcpToolCall success with regex extraction: %v", regexToolCall)
		return regexToolCall, nil
	}
	if regexErr != nil {
		lastErr = regexErr
	}

	// 策略4: 模糊匹配关键词
	fuzzyToolCall, fuzzyErr := extractWithFuzzyMatching(content)
	if fuzzyErr == nil && fuzzyToolCall.ServerName != "" && fuzzyToolCall.ToolName != "" {
		logger.Info("parseMcpToolCall success with fuzzy matching: %v", fuzzyToolCall)
		return fuzzyToolCall, nil
	}
	if fuzzyErr != nil {
		lastErr = fuzzyErr
	}

	// 策略5: 部分解析 - 即使只能提取部分字段也尝试返回
	partialToolCall := extractPartialFields(content)
	if partialToolCall.ServerName != "" || partialToolCall.ToolName != "" {
		logger.Info("parseMcpToolCall partial success: %v", partialToolCall)
		return partialToolCall, fmt.Errorf("partial parsing: missing some fields")
	}

	logger.Error("parseMcpToolCall failed with all strategies, last error: %v", lastErr)
	return toolCall, lastErr
}

// 从代码块中提取JSON
func extractJSONFromCodeBlock(content string) string {
	// 匹配 ```json {...} ``` 或 ``` {...} ```
	patterns := []string{
		"(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```",
		"(?s)```\\s*(\\{.*?\\})\\s*```",
		"(?s)`\\s*(\\{.*?\\})\\s*`", // 单反引号
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(content)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}
	return ""
}

// 使用正则表达式提取关键字段
func extractWithRegex(content string) (McpToolCall, error) {
	var toolCall McpToolCall

	// 提取 server_name
	serverNamePatterns := []string{
		`"server_name"\s*:\s*"([^"]+)"`,
		`'server_name'\s*:\s*'([^']+)'`,
		`server_name:\s*"([^"]+)"`,
		`server_name:\s*'([^']+)'`,
		`serverName\s*:\s*"([^"]+)"`,
	}

	for _, pattern := range serverNamePatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(content)
		if len(matches) > 1 {
			toolCall.ServerName = matches[1]
			break
		}
	}

	// 提取 tool_name
	toolNamePatterns := []string{
		`"tool_name"\s*:\s*"([^"]+)"`,
		`'tool_name'\s*:\s*'([^']+)'`,
		`tool_name:\s*"([^"]+)"`,
		`tool_name:\s*'([^']+)'`,
		`toolName\s*:\s*"([^"]+)"`,
	}

	for _, pattern := range toolNamePatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(content)
		if len(matches) > 1 {
			toolCall.ToolName = matches[1]
			break
		}
	}

	// 提取 arguments (简化处理)
	argumentsPatterns := []string{
		`"arguments"\s*:\s*(\{[^}]*\})`,
		`'arguments'\s*:\s*(\{[^}]*\})`,
		`arguments:\s*(\{[^}]*\})`,
	}

	for _, pattern := range argumentsPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(content)
		if len(matches) > 1 {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(matches[1]), &args); err == nil {
				toolCall.Arguments = args
			}
			break
		}
	}

	if toolCall.ServerName == "" && toolCall.ToolName == "" {
		return toolCall, fmt.Errorf("regex extraction failed: no server_name or tool_name found")
	}

	return toolCall, nil
}

// 模糊匹配关键词
func extractWithFuzzyMatching(content string) (McpToolCall, error) {
	var toolCall McpToolCall

	// 将内容按行分割并查找包含关键词的行
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 查找服务器名称
		if strings.Contains(line, "server") && (strings.Contains(line, ":") || strings.Contains(line, "=")) {
			if value := extractValueFromLine(line); value != "" {
				toolCall.ServerName = value
			}
		}

		// 查找工具名称
		if strings.Contains(line, "tool") && (strings.Contains(line, ":") || strings.Contains(line, "=")) {
			if value := extractValueFromLine(line); value != "" {
				toolCall.ToolName = value
			}
		}
	}

	if toolCall.ServerName == "" && toolCall.ToolName == "" {
		return toolCall, fmt.Errorf("fuzzy matching failed: no recognizable fields found")
	}

	return toolCall, nil
}

// 从行中提取值
func extractValueFromLine(line string) string {
	// 尝试多种分隔符和引号组合
	patterns := []string{
		`:\s*"([^"]+)"`,
		`:\s*'([^']+)'`,
		`:\s*([^,}\s]+)`,
		`=\s*"([^"]+)"`,
		`=\s*'([^']+)'`,
		`=\s*([^,}\s]+)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(line)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}
	return ""
}

// 部分字段提取 - 最后的兜底策略
func extractPartialFields(content string) McpToolCall {
	var toolCall McpToolCall

	// 简单的关键词搜索
	content = strings.ToLower(content)

	// 寻找可能的服务器名称
	serverKeywords := []string{"server", "service", "host"}
	for _, keyword := range serverKeywords {
		if idx := strings.Index(content, keyword); idx != -1 {
			// 尝试提取后面的值
			substr := content[idx:]
			if value := extractNearbyValue(substr); value != "" {
				toolCall.ServerName = value
				break
			}
		}
	}

	// 寻找可能的工具名称
	toolKeywords := []string{"tool", "function", "method", "action"}
	for _, keyword := range toolKeywords {
		if idx := strings.Index(content, keyword); idx != -1 {
			substr := content[idx:]
			if value := extractNearbyValue(substr); value != "" {
				toolCall.ToolName = value
				break
			}
		}
	}

	return toolCall
}

// 提取附近的值
func extractNearbyValue(text string) string {
	// 查找引号内的内容或者冒号后的词
	patterns := []string{
		`"([^"]+)"`,
		`'([^']+)'`,
		`:\s*(\w+)`,
		`=\s*(\w+)`,
		`\s+(\w+)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 && len(matches[1]) > 2 { // 至少3个字符
			return matches[1]
		}
	}
	return ""
}

// Helper: check if string is "1" or "true" (case-insensitive)
func isTrue(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s == "1" || s == "true"
}

// Collapse multiple blank lines to a single newline
func collapseBlankLines(s string) string {
	return mustCompile(`\n{2,}`).ReplaceAllString(s, "\n")
}

// mustCompile is a helper for regexp.MustCompile
func mustCompile(expr string) *regexp.Regexp {
	re, err := regexp.Compile(expr)
	if err != nil {
		panic(err)
	}
	return re
}
