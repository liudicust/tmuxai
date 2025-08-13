package system

import (
	"errors"
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

	for {
		// 更新显示项目的选择状态
		for i, item := range items {
			if selectedItems[i] {
				displayItems[i] = "[✓] " + item
			} else {
				displayItems[i] = "[ ] " + item
			}
		}

		// 在顶部添加退出选项，然后是分隔符，再是服务器列表，最后是确认选项
		allOptions := []string{
			"❌ Exit (Press Enter to quit)",
			"---",
		}
		allOptions = append(allOptions, displayItems...)
		allOptions = append(allOptions, "---", "✓ Confirm Selection")

		prompt := promptui.Select{
			Label:     "Select MCP Servers (↑↓: navigate, Space: toggle, Enter: confirm, Ctrl+C: quit)",
			Items:     allOptions,
			Size:      20,
			CursorPos: 0, // 默认选中退出选项
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

		// 切换选择状态（需要调整索引，因为添加了退出选项和分隔符）
		adjustedIdx := idx - 2 // 减去退出选项和第一个分隔符
		if adjustedIdx >= 0 && adjustedIdx < len(items) {
			selectedItems[adjustedIdx] = !selectedItems[adjustedIdx]
		}
	}
}
