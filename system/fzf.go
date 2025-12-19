package system

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var ErrUserCancelledSelection = errors.New("user cancelled selection")

func truncateToWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return runewidth.Truncate(s, w, "…")
}

type mcpSelectItemKind string

const (
	mcpSelectItemKindNormal  mcpSelectItemKind = "normal"
	mcpSelectItemKindConfirm mcpSelectItemKind = "confirm"
	mcpSelectItemKindExit    mcpSelectItemKind = "exit"
	mcpSelectItemKindSep     mcpSelectItemKind = "sep"
)

type mcpSelectOptions struct {
	Title        string
	Compact      bool
	Details      map[string]string
	PreviewLines int
}

type mcpSelectItem struct {
	label string
	kind  mcpSelectItemKind
}

func (i mcpSelectItem) FilterValue() string {
	return i.label
}

type mcpSelectDelegate struct {
	selected map[string]bool
	active   lipgloss.Style
	normal   lipgloss.Style
}

func (d mcpSelectDelegate) Height() int  { return 1 }
func (d mcpSelectDelegate) Spacing() int { return 0 }
func (d mcpSelectDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d mcpSelectDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	si, ok := item.(mcpSelectItem)
	if !ok {
		return
	}

	maxW := m.Width()
	if maxW < 1 {
		maxW = 1
	}

	var line string
	switch si.kind {
	case mcpSelectItemKindConfirm:
		line = "✓ Confirm Selection"
	case mcpSelectItemKindExit:
		line = "❌ Exit"
	case mcpSelectItemKindSep:
		sepW := maxW
		if sepW < 1 {
			sepW = 1
		}
		line = strings.Repeat("─", sepW)
	default:
		box := "[ ]"
		if d.selected[si.label] {
			box = "[✓]"
		}
		line = fmt.Sprintf("%s %s", box, si.label)
	}

	prefix := "  "
	style := d.normal
	if index == m.Index() {
		prefix = "▶ "
		style = d.active
	}

	out := prefix + line
	out = truncateToWidth(out, maxW)
	_, _ = fmt.Fprint(w, style.Render(out))
}

type mcpSelectModel struct {
	list         list.Model
	items        []string
	selected     map[string]bool
	opts         mcpSelectOptions
	done         bool
	cancelled    bool
	winW         int
	contentH     int
	listW        int
	vp           viewport.Model
	focusPreview bool
	lastSelected string
}

func (m *mcpSelectModel) previewEnabled() bool {
	return m.opts.Details != nil && m.opts.PreviewLines > 0
}

func (m mcpSelectModel) Init() tea.Cmd {
	return nil
}

func (m mcpSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		footerH := 1
		contentH := msg.Height - footerH
		if contentH < 1 {
			contentH = 1
		}

		m.winW = msg.Width
		m.contentH = contentH

		if m.previewEnabled() {
			sepW := 1
			minListW := 24
			minPreviewW := 30

			listW := (msg.Width - sepW) / 2
			if listW < minListW {
				listW = minListW
			}
			if msg.Width < minListW+sepW+minPreviewW {
				listW = msg.Width - sepW - minPreviewW
				if listW < 1 {
					listW = 1
				}
			}

			vpW := msg.Width - sepW - listW
			if vpW < 1 {
				vpW = 1
			}

			m.listW = listW
			m.list.SetSize(listW, contentH)
			m.vp.Width = vpW
			m.vp.Height = contentH
			m.vp.SetContent(m.previewContent())
			return m, nil
		}

		m.listW = msg.Width
		m.list.SetSize(msg.Width, contentH)
		m.vp.Width = 0
		m.vp.Height = 0
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.cancelled = true
			return m, tea.Quit
		case "tab":
			if m.previewEnabled() && m.vp.Height > 0 {
				m.focusPreview = !m.focusPreview
				if m.focusPreview {
					m.vp.SetContent(m.previewContent())
				}
			}
			return m, nil
		case "enter":
			if m.focusPreview {
				return m, nil
			}
			if li := m.list.SelectedItem(); li != nil {
				if it, ok := li.(mcpSelectItem); ok {
					switch it.kind {
					case mcpSelectItemKindConfirm:
						m.done = true
						return m, tea.Quit
					case mcpSelectItemKindExit:
						m.cancelled = true
						return m, tea.Quit
					case mcpSelectItemKindSep:
						return m, nil
					default:
						m.selected[it.label] = !m.selected[it.label]
						return m, nil
					}
				}
			}
			return m, nil
		case " ":
			if m.focusPreview {
				return m, nil
			}
			if li := m.list.SelectedItem(); li != nil {
				if it, ok := li.(mcpSelectItem); ok {
					if it.kind == mcpSelectItemKindNormal {
						m.selected[it.label] = !m.selected[it.label]
					}
				}
			}
			return m, nil
		case "a":
			if m.focusPreview {
				return m, nil
			}
			anyUnselected := false
			for _, it := range m.items {
				if !m.selected[it] {
					anyUnselected = true
					break
				}
			}
			for _, it := range m.items {
				m.selected[it] = anyUnselected
			}
			return m, nil
		case "c":
			m.done = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	if m.focusPreview {
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}

	m.list, cmd = m.list.Update(msg)
	if m.previewEnabled() && m.vp.Height > 0 {
		m.updatePreviewOnSelectionChange()
	}
	return m, cmd
}

func (m *mcpSelectModel) selectedKey() string {
	if li := m.list.SelectedItem(); li != nil {
		if it, ok := li.(mcpSelectItem); ok {
			if it.kind == mcpSelectItemKindNormal {
				return it.label
			}
		}
	}
	return ""
}

func splitToolDetail(content string) (string, []string) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	idx := strings.Index(content, ":param ")
	if idx < 0 {
		return strings.TrimSpace(content), nil
	}

	desc := strings.TrimSpace(content[:idx])
	paramBlob := content[idx:]
	parts := strings.Split(paramBlob, ":param ")
	params := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		params = append(params, ":param "+p)
	}
	return desc, params
}

func (m *mcpSelectModel) previewContent() string {
	key := m.selectedKey()
	if key == "" || m.opts.Details == nil {
		return ""
	}
	content, ok := m.opts.Details[key]
	if !ok {
		return ""
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}

	w := m.vp.Width
	if w <= 0 {
		w = m.winW - m.listW - 1
	}
	if w <= 0 {
		w = 80
	}

	titleStyle := lipgloss.NewStyle().Bold(true)
	sectionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true)
	sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	paramStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))

	desc, params := splitToolDetail(content)

	var b strings.Builder
	b.WriteString(titleStyle.Render(key))
	b.WriteString("\n")
	b.WriteString(sepStyle.Render(strings.Repeat("─", w)))

	if strings.TrimSpace(desc) != "" {
		b.WriteString("\n\n")
		b.WriteString(sectionStyle.Render("Description"))
		b.WriteString("\n")
		b.WriteString(desc)
	}

	if len(params) > 0 {
		b.WriteString("\n\n")
		b.WriteString(sectionStyle.Render("Parameters"))
		b.WriteString("\n")
		for i, p := range params {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(paramStyle.Render(p))
		}
	}

	return lipgloss.NewStyle().Width(w).Render(b.String())
}

func (m *mcpSelectModel) updatePreviewOnSelectionChange() {
	key := m.selectedKey()
	if key == m.lastSelected {
		return
	}
	m.lastSelected = key
	m.vp.GotoTop()
	m.vp.SetContent(m.previewContent())
}

func (m mcpSelectModel) View() string {
	selectedCount := 0
	for _, it := range m.items {
		if m.selected[it] {
			selectedCount++
		}
	}

	page := m.list.Paginator.Page + 1
	totalPages := m.list.Paginator.TotalPages
	if totalPages < 1 {
		totalPages = 1
	}

	footerText := "Enter/Space: toggle  c/Confirm: confirm  PgUp/PgDn: page  /: filter  Tab: focus details  Esc/Ctrl+C: cancel  a: all/none"
	if m.opts.Compact {
		footerText = "Enter/Space: toggle  c: confirm  PgUp/PgDn: page  /: filter  Tab: focus details  Esc/Ctrl+C: cancel  a: all/none"
	}
	footerText = fmt.Sprintf("%s  Selected: %d/%d  Page: %d/%d", footerText, selectedCount, len(m.items), page, totalPages)

	w := m.winW
	if w <= 0 {
		w = m.list.Width()
	}
	if w > 0 {
		footerText = truncateToWidth(footerText, w)
	}
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(footerText)

	if m.previewEnabled() && m.vp.Height > 0 {
		left := m.list.View()

		sep := strings.Repeat("│\n", m.contentH)
		sep = strings.TrimSuffix(sep, "\n")
		sep = lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(sep)

		right := m.vp.View()
		if !m.focusPreview {
			right = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(right)
		}

		content := lipgloss.JoinHorizontal(lipgloss.Top, left, sep, right)
		return content + "\n" + footer
	}

	return m.list.View() + "\n" + footer
}

// InteractiveSelect 使用 Bubble Tea 实现交互式多选功能
// items: 可选择的项目列表
// preSelected: 预先选中的项目（map[string]struct{}格式）
func InteractiveSelect(items []string, preSelected map[string]struct{}) ([]string, error) {
	return interactiveSelect(items, preSelected, mcpSelectOptions{Title: "Select Items"})
}

func interactiveSelect(items []string, preSelected map[string]struct{}, opts mcpSelectOptions) ([]string, error) {
	if len(items) == 0 {
		return nil, errors.New("no items to select")
	}

	selected := make(map[string]bool)
	for _, it := range items {
		if _, ok := preSelected[it]; ok {
			selected[it] = true
		}
	}

	listItems := make([]list.Item, 0, len(items)+3)
	listItems = append(listItems,
		mcpSelectItem{label: "✓ Confirm Selection", kind: mcpSelectItemKindConfirm},
		mcpSelectItem{label: "❌ Exit", kind: mcpSelectItemKindExit},
		mcpSelectItem{label: "---", kind: mcpSelectItemKindSep},
	)
	for _, it := range items {
		listItems = append(listItems, mcpSelectItem{label: it, kind: mcpSelectItemKindNormal})
	}

	d := mcpSelectDelegate{
		selected: selected,
		active:   lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true),
		normal:   lipgloss.NewStyle(),
	}
	l := list.New(listItems, d, 0, 0)
	if opts.Title != "" {
		l.Title = opts.Title
	} else {
		l.Title = "Select Items"
	}

	l.DisableQuitKeybindings()
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)

	vp := viewport.New(0, 0)
	p := tea.NewProgram(mcpSelectModel{list: l, items: items, selected: selected, opts: opts, vp: vp}, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	fm, ok := finalModel.(mcpSelectModel)
	if !ok {
		return nil, errors.New("could not assert model")
	}
	if fm.cancelled || !fm.done {
		return nil, ErrUserCancelledSelection
	}

	var out []string
	for _, it := range items {
		if fm.selected[it] {
			out = append(out, it)
		}
	}
	return out, nil
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

	selectedServerNames, err := interactiveSelect(serverNames, preSelectedServers, mcpSelectOptions{Title: "Select MCP Servers"})
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

		var toolDisplayItems []string
		toolMap := make(map[string]string)
		details := make(map[string]string)
		for _, tool := range tools {
			display := tool.Name
			toolDisplayItems = append(toolDisplayItems, display)
			toolMap[display] = tool.Name

			full := strings.TrimSpace(tool.Description)
			if full == "" {
				full = "No description"
			}
			details[display] = full
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

		selectedToolDisplays, err := interactiveSelect(toolDisplayItems, preSelectedToolDisplays, mcpSelectOptions{Title: fmt.Sprintf("Select MCP Tools (%s)", serverName), Compact: true, Details: details, PreviewLines: 6})
		if err != nil {
			return nil, fmt.Errorf("error selecting tools for server '%s': %w", serverName, err)
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
