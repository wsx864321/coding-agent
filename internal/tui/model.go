package tui

import (
	"context"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

const interruptedStatusMsg = "已中断"

// Model 是 Bubble Tea 聊天界面的状态机。
type Model struct {
	messages     []Message
	input        string
	scrollOffset int
	width        int
	height       int
	quitting     bool
	busy         bool
	lastError    string
	statusMsg    string
	interrupted  bool
	runner       Runner
	streamCh     <-chan any
	turnCancel   context.CancelFunc
}

// New 构造初始 TUI model。
func New() Model {
	return Model{}
}

// NewWithRunner 构造带会话执行器的 TUI model。
func NewWithRunner(runner Runner) Model {
	return Model{runner: runner}
}

// Init 启动时不发送额外命令。
func (m Model) Init() tea.Cmd {
	return nil
}

// Update 处理窗口尺寸、输入编辑、滚动、流式事件与 Ctrl+C 退出。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.scrollOffset = m.clampScroll(m.scrollOffset)
	case StreamChunkMsg:
		if !m.busy {
			return m, nil
		}
		m = m.appendAssistantChunk(msg.Text)
		m = m.clampScrollToBottom()
		if m.streamCh != nil {
			return m, waitStreamMsg(m.streamCh)
		}
		return m, nil
	case StreamDoneMsg:
		m.busy = false
		m.streamCh = nil
		m.turnCancel = nil
		m.interrupted = false
		m = m.clampScrollToBottom()
		return m, nil
	case StreamErrorMsg:
		m.busy = false
		m.streamCh = nil
		m.turnCancel = nil
		if m.interrupted {
			m.interrupted = false
			return m, nil
		}
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
		}
		return m, nil
	case streamClosedMsg:
		m.busy = false
		m.streamCh = nil
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m = m.interruptTurn()
			m.quitting = true
			return m, tea.Quit
		case "esc":
			return m.interruptTurn(), nil
		case "enter":
			return m.submit()
		case "k":
			if m.input == "" {
				m.scrollOffset = m.clampScroll(m.scrollOffset - 1)
			} else if !m.busy {
				m.input += "k"
			}
		case "j":
			if m.input == "" {
				m.scrollOffset = m.clampScroll(m.scrollOffset + 1)
			} else if !m.busy {
				m.input += "j"
			}
		case "backspace":
			if !m.busy && m.input != "" {
				_, size := utf8.DecodeLastRuneInString(m.input)
				m.input = m.input[:len(m.input)-size]
			}
		case "up":
			m.scrollOffset = m.clampScroll(m.scrollOffset - 1)
		case "down":
			m.scrollOffset = m.clampScroll(m.scrollOffset + 1)
		default:
			if !m.busy {
				if s := msg.String(); len(s) == 1 {
					m.input += s
				}
			}
		}
	}
	return m, nil
}

func (m Model) submit() (Model, tea.Cmd) {
	text := strings.TrimSpace(m.input)
	if m.busy || text == "" || m.runner == nil {
		return m, nil
	}

	m.input = ""
	m.busy = true
	m.lastError = ""
	m.statusMsg = ""
	m.interrupted = false
	m = m.withMessage(RoleUser, text)
	m = m.withMessage(RoleAssistant, "")

	ch := make(chan any, 16)
	runner := m.runner
	ctx, cancel := context.WithCancel(context.Background())
	m.turnCancel = cancel
	go func() {
		defer close(ch)
		emit := chanEmitter{ch: ch}
		_ = runner.RunTurn(ctx, text, emit)
	}()

	m.streamCh = ch
	return m, waitStreamMsg(ch)
}

func waitStreamMsg(ch <-chan any) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return streamClosedMsg{}
		}
		if teaMsg, ok := msg.(tea.Msg); ok {
			return teaMsg
		}
		return streamClosedMsg{}
	}
}

func (m Model) appendAssistantChunk(text string) Model {
	if text == "" {
		return m
	}
	if len(m.messages) == 0 || m.messages[len(m.messages)-1].Role != RoleAssistant {
		return m.withMessage(RoleAssistant, text)
	}
	last := len(m.messages) - 1
	m.messages[last].Content += text
	return m
}

// withMessage 追加一条消息（供测试与后续桥接层使用）。
func (m Model) withMessage(role Role, content string) Model {
	m.messages = append(m.messages, Message{Role: role, Content: content})
	return m
}

// clampScrollToBottom 将滚动位置对齐到消息区底部。
func (m Model) clampScrollToBottom() Model {
	m.scrollOffset = m.maxScrollOffset()
	return m
}

func (m Model) maxScrollOffset() int {
	total := len(m.renderMessageLines())
	viewport := m.messageViewportHeight()
	if total <= viewport {
		return 0
	}
	return total - viewport
}

func (m Model) clampScroll(offset int) int {
	max := m.maxScrollOffset()
	if offset < 0 {
		return 0
	}
	if offset > max {
		return max
	}
	return offset
}

func (m Model) interruptTurn() Model {
	if !m.busy {
		return m
	}
	if m.turnCancel != nil {
		m.turnCancel()
		m.turnCancel = nil
	}
	m.busy = false
	m.streamCh = nil
	m.statusMsg = interruptedStatusMsg
	m.interrupted = true
	return m
}

func (m Model) messageViewportHeight() int {
	overhead := 5 // title block + input + help
	if m.lastError != "" {
		overhead++
	}
	if m.statusMsg != "" {
		overhead++
	}
	if m.height <= overhead {
		return 1
	}
	return m.height - overhead
}

func (m Model) renderMessageLines() []string {
	if m.width <= 0 {
		m.width = 80
	}
	contentWidth := m.width - 2
	if contentWidth < 10 {
		contentWidth = 10
	}

	var lines []string
	for _, msg := range m.messages {
		prefix := roleLabel(msg.Role) + ": "
		wrapped := wrapText(msg.Content, contentWidth-len(prefix))
		for i, line := range wrapped {
			if i == 0 {
				lines = append(lines, prefix+line)
			} else {
				lines = append(lines, strings.Repeat(" ", len(prefix))+line)
			}
		}
	}
	return lines
}

func roleLabel(role Role) string {
	switch role {
	case RoleUser:
		return "user"
	case RoleAssistant:
		return "assistant"
	case RoleSystem:
		return "system"
	default:
		return "unknown"
	}
}

func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	if text == "" {
		return []string{""}
	}

	var lines []string
	var current strings.Builder
	currentLen := 0

	flush := func() {
		if currentLen > 0 || current.Len() > 0 {
			lines = append(lines, current.String())
			current.Reset()
			currentLen = 0
		}
	}

	for _, r := range text {
		runeLen := utf8.RuneLen(r)
		if currentLen+runeLen > width {
			flush()
		}
		current.WriteRune(r)
		currentLen += runeLen
	}
	flush()
	return lines
}
