package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 定义开启和关闭焦点上报的转义序列
const (
	enableFocusReport  = "\x1b[?1004h"
	disableFocusReport = "\x1b[?1004l"
)

type textInputModel struct {
	textInput textinput.Model
	err       error
	output    string
	quitting  bool
	width     int
}

func initialTextInputModel(prompt string) textInputModel {
	ti := textinput.New()
	ti.Placeholder = "Type your message or \\command..."
	ti.Focus() // 初始状态为聚焦
	ti.CharLimit = 1000
	ti.Width = 60
	ti.Prompt = prompt

	return textInputModel{
		textInput: ti,
		err:       nil,
		width:     60,
	}
}

func (m textInputModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,               // 启动光标闪烁
		tea.Printf(enableFocusReport), // 关键：告诉终端/Tmux 上报焦点事件
	)
}

func (m textInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {

	// ---------------- 改动开始 ----------------
	// 新版 Bubble Tea 将 Focus 和 Blur 分为了两个独立的消息

	case tea.FocusMsg:
		// 获得焦点：让输入框聚焦（恢复光标闪烁）
		cmd = m.textInput.Focus()
		return m, cmd

	case tea.BlurMsg:
		// 失去焦点：让输入框失焦（停止光标闪烁）
		m.textInput.Blur()
		return m, nil

	// ---------------- 改动结束 ----------------

	case tea.WindowSizeMsg:
		m.width = msg.Width
		availableWidth := m.width - 4
		if availableWidth > 0 {
			m.textInput.Width = availableWidth - len(m.textInput.Prompt)
		}

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			m.output = m.textInput.Value()
			m.quitting = true
			return m, tea.Quit
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			return m, tea.Quit
		}
	}

	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m textInputModel) View() string {
	// 根据是否聚焦稍微改变一下边框颜色
	borderColor := lipgloss.Color("62") // 默认蓝色
	if !m.textInput.Focused() {
		borderColor = lipgloss.Color("240") // 失焦变灰
	}

	focusedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(m.width - 2)

	return focusedStyle.Render(m.textInput.View()) + "\n"
}

func GetInput(prompt string) (string, error) {
	p := tea.NewProgram(initialTextInputModel(prompt))

	m, err := p.Run()
	if err != nil {
		return "", err
	}

	// 退出前关闭焦点上报
	fmt.Print(disableFocusReport)

	if m, ok := m.(textInputModel); ok {
		if m.output == "" && m.quitting {
			return "", nil
		}
		return m.output, nil
	}

	return "", fmt.Errorf("could not assert model")
}

func main() {
	fmt.Println("=== Lipgloss Input Box Demo (Tmux Focus Aware) ===")
	fmt.Println("尝试在 Tmux 中切换 pane，光标应该会自动停止/恢复闪烁。")
	fmt.Println("请输入一些文本（按 Enter 提交，Ctrl+C 退出）：")

	input, err := GetInput("> ")
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	if input != "" {
		fmt.Printf("您输入的内容是: %s\n", input)
	} else {
		fmt.Println("未输入内容或已退出")
	}
}
