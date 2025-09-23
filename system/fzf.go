package system

import (
	"errors"
	"fmt"
	"strings"

	"github.com/trzsz/promptui"
)

// InteractiveSelect 使用 promptui 实现交互式多选功能
// items: 可选择的项目列表
// preSelected: 预先选中的项目（map[string]struct{}格式）
func InteractiveSelect(items []string, preSelected map[string]struct{}) ([]string, error) {
	if len(items) == 0 {
		return nil, errors.New("no items to select")
	}

	// 初始化选择状态，根据 preSelected 设置已选中的项目
	selectedItems := make(map[int]bool)
	for i, item := range items {
		if _, exists := preSelected[item]; exists {
			selectedItems[i] = true
		}
	}

	displayItems := make([]string, len(items))
	for i, item := range items {
		displayItems[i] = "[ ] " + item
	}

	// 添加变量来跟踪光标位置
	cursorPos := 0

	for {
		// 更新显示项目的选择状态
		for i, item := range items {
			if selectedItems[i] {
				displayItems[i] = "[✓] " + item
			} else {
				displayItems[i] = "[ ] " + item
			}
		}

		// 在顶部添加退出选项、确认选项，然后是分隔符，再是列表
		allOptions := []string{
			"❌ Exit (Press Enter to quit)",
			"✓ Confirm Selection",
			"---",
		}
		allOptions = append(allOptions, displayItems...)

		// 动态计算Size：根据项目数量调整，但保持在合理范围内
		// 最小10，最大30，如果项目很多就用30让用户滚动查看
		dynamicSize := len(allOptions)
		if dynamicSize < 10 {
			dynamicSize = 10
		} else if dynamicSize > 30 {
			dynamicSize = 30
		}

		prompt := promptui.Select{
			Label:     "Select Items (↑↓: navigate, Space: toggle, Enter: confirm, Ctrl+C: quit)",
			Items:     allOptions,
			Size:      dynamicSize, // 使用动态Size
			CursorPos: cursorPos,   // 使用动态光标位置
			Templates: &promptui.SelectTemplates{
				Active:   "▶ {{ . | cyan }}",
				Inactive: "  {{ . }}",
				// 设置为空字符串，避免显示选中的项目
				Selected: "",
			},
			HideSelected: true, // 隐藏选中项的显示
		}

		idx, result, err := prompt.Run()
		if err != nil {
			// promptui 默认支持 Ctrl+C 退出
			if strings.Contains(err.Error(), "interrupt") {
				return nil, errors.New("user cancelled selection")
			}
			return nil, err
		}

		// 处理特殊选项
		if strings.Contains(result, "Exit") {
			return nil, nil
		}

		if result == "✓ Confirm Selection" {
			// 返回选中的项目
			var selected []string
			for i, isSelected := range selectedItems {
				if isSelected {
					selected = append(selected, items[i])
				}
			}
			return selected, nil
		}

		if result == "---" {
			continue // 分隔符，忽略
		}

		// 切换选择状态（需要调整索引，因为顶部有退出、确认和分隔符三个项）
		adjustedIdx := idx - 3
		if adjustedIdx >= 0 && adjustedIdx < len(items) {
			selectedItems[adjustedIdx] = !selectedItems[adjustedIdx]
			// 保持光标在当前选择的选项位置
			cursorPos = idx
		}
	}
}
 
// ServerToolSelection 表示服务器和工具的选择结果
type ServerToolSelection struct {
	ServerName    string
	SelectedTools []string
}

// ServerInfo 表示服务器信息
type ServerInfo struct {
	Name  string
	Tools []ToolInfo
}

// ToolInfo 表示工具信息
type ToolInfo struct {
	Name        string
	Description string
}

// InteractiveSelectServersAndTools 实现两步选择：先选择服务器，然后为每个服务器选择工具（懒加载工具列表）
func InteractiveSelectServersAndTools(
	serverNames []string,
	preSelectedTools map[string][]string,
	loadTools func(serverName string) ([]ToolInfo, error),
) ([]ServerToolSelection, error) {
	// 第一步：选择服务器
	if len(serverNames) == 0 {
		return nil, nil
	}

	// 预选所有有历史记录的服务器
	preSelectedServers := make(map[string]struct{})
	for serverName := range preSelectedTools {
		preSelectedServers[serverName] = struct{}{}
	}

	selectedServerNames, err := InteractiveSelect(serverNames, preSelectedServers)
	if err != nil {
		return nil, err
	}
	if len(selectedServerNames) == 0 {
		return nil, nil
	}

	// 第二步：为每个选中的服务器选择工具（懒加载工具）
	var results []ServerToolSelection

	for _, serverName := range selectedServerNames {
		tools, err := loadTools(serverName)
		if err != nil {
			// 工具加载失败时，返回错误以便上层提示；也可按需改为跳过该服务器
			return nil, fmt.Errorf("error loading tools for server '%s': %v", serverName, err)
		}

		if len(tools) == 0 {
			// 如果服务器没有工具，也要添加到结果中
			results = append(results, ServerToolSelection{
				ServerName:    serverName,
				SelectedTools: []string{},
			})
			continue
		}

		// 构建工具显示项（名称 - 描述），限制描述长度
		var toolDisplayItems []string
		toolMap := make(map[string]string) // display -> toolName
		for _, tool := range tools {
			description := tool.Description
			if len(description) > 80 {
				description = description[:77] + "..."
			}
			display := fmt.Sprintf("%s - %s", tool.Name, description)
			toolDisplayItems = append(toolDisplayItems, display)
			toolMap[display] = tool.Name
		}

		// 预选工具：如果有历史记录则按历史，否则默认全选
		preSelectedToolDisplays := make(map[string]struct{})
		if prevTools, exists := preSelectedTools[serverName]; exists && len(prevTools) > 0 {
			// 按历史记录预选
			for _, toolName := range prevTools {
				for display, name := range toolMap {
					if name == toolName {
						preSelectedToolDisplays[display] = struct{}{}
						break
					}
				}
			}
		} else {
			// 默认全选
			for _, display := range toolDisplayItems {
				preSelectedToolDisplays[display] = struct{}{}
			}
		}

		selectedToolDisplays, err := InteractiveSelect(toolDisplayItems, preSelectedToolDisplays)
		if err != nil {
			return nil, fmt.Errorf("error selecting tools for server '%s': %v", serverName, err)
		}

		// 转换回工具名称
		var selectedTools []string
		for _, display := range selectedToolDisplays {
			if toolName, exists := toolMap[display]; exists {
				selectedTools = append(selectedTools, toolName)
			}
		}

		results = append(results, ServerToolSelection{
			ServerName:    serverName,
			SelectedTools: selectedTools,
		})
	}

	return results, nil
}
