package tui

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

const interruptedStatusMsg = "已中断"

// Model 是 Bubble Tea 聊天界面的状态机。
type Model struct {
	transcript  []TranscriptEntry
	textarea    textarea.Model
	viewport    viewport.Model
	spinner     spinner.Model
	mdRenderer  MarkdownRenderer
	width       int
	height      int
	quitting    bool
	busy        bool
	lastError   string
	statusMsg   string
	statusLabel string
	interrupted bool
	pendingToolName string
	pendingToolArgs string
	approval    *pendingApproval
	runner      Runner
	streamCh    <-chan any
	turnCancel  context.CancelFunc
}

// New 构造初始 TUI model。
func New() Model {
	return Model{
		textarea:   newTextarea(),
		viewport:   newViewport(),
		spinner:    newSpinner(),
		mdRenderer: NewGlamourRenderer(),
	}
}

// NewWithRunner 构造带会话执行器的 TUI model。
func NewWithRunner(runner Runner) Model {
	m := New()
	m.runner = runner
	return m
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
		m = m.syncLayout()
		m = m.rerenderTranscript()
		m = m.syncViewportContent()
		return m, nil

	case StreamChunkMsg:
		if !m.busy {
			return m, nil
		}
		m = m.appendAssistantChunk(msg.Text)
		m = m.syncViewportContent()
		if m.streamCh != nil {
			return m, waitStreamMsg(m.streamCh)
		}
		return m, nil

	case ToolStartMsg:
		if !m.busy {
			return m, nil
		}
		m.statusLabel = "running " + msg.Name + "..."
		m.pendingToolName = msg.Name
		m.pendingToolArgs = msg.Args
		if m.streamCh != nil {
			return m, waitStreamMsg(m.streamCh)
		}
		return m, nil

	case ToolEndMsg:
		if !m.busy {
			return m, nil
		}
		name := msg.Name
		if name == "" {
			name = m.pendingToolName
		}
		args := m.pendingToolArgs
		w := m.contentWidth()
		if msg.IsError {
			m = m.appendEntry(TranscriptEntry{
				Kind:    EntryToolCard,
				Content: renderToolCardError(name, msg.Result, w),
				Raw:     encodeToolCardRaw(name, msg.Result, true),
			})
		} else {
			m = m.appendEntry(TranscriptEntry{
				Kind:    EntryToolCard,
				Content: renderToolCard(name, args, w),
				Raw:     encodeToolCardRaw(name, args, false),
			})
			if msg.Result != "" {
				m = m.appendEntry(TranscriptEntry{
					Kind:    EntryToolOutput,
					Content: renderToolOutput(msg.Result, toolOutputCollapseLines),
					Raw:     msg.Result,
				})
			}
		}
		m.pendingToolName = ""
		m.pendingToolArgs = ""
		m.statusLabel = "thinking"
		m = m.syncViewportContent()
		if m.streamCh != nil {
			return m, waitStreamMsg(m.streamCh)
		}
		return m, nil

	case StreamDoneMsg:
		m.busy = false
		m.streamCh = nil
		m.turnCancel = nil
		m.interrupted = false
		m.statusLabel = ""
		m.pendingToolName = ""
		m.pendingToolArgs = ""
		m = m.syncViewportContent()
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
		m = m.syncLayout()
		return m, nil

	case streamClosedMsg:
		m.busy = false
		m.streamCh = nil
		return m, nil

	case ApprovalRequestMsg:
		if !m.busy {
			return m, nil
		}
		m.approval = &pendingApproval{toolName: msg.Name, args: msg.Args, respond: msg.Respond}
		if m.streamCh != nil {
			return m, waitStreamMsg(m.streamCh)
		}
		return m, nil

	case spinner.TickMsg:
		if !m.busy {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.MouseWheelMsg, tea.MouseMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		if m.approval != nil {
			switch msg.String() {
			case "y", "Y", "n", "N":
				return m.handleApprovalKey(msg)
			case "esc":
				m = m.interruptTurn()
				m.approval = nil
				return m, nil
			default:
				return m, nil
			}
		}

		switch {
		case msg.String() == "ctrl+c":
			m = m.interruptTurn()
			m.approval = nil
			m.quitting = true
			return m, tea.Quit

		case msg.String() == "esc":
			return m.interruptTurn(), nil

		case isSubmitKey(msg):
			return m.submit()

		case m.shouldRouteScrollToViewport(msg):
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd

		default:
			if m.busy {
				return m, nil
			}
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			m = m.syncLayout()
			return m, cmd
		}
	}

	return m, nil
}

func isSubmitKey(msg tea.KeyPressMsg) bool {
	if msg.Mod.Contains(tea.ModShift) || msg.Mod.Contains(tea.ModAlt) {
		return false
	}
	switch msg.Key().Code {
	case tea.KeyEnter:
		return true
	default:
		return false
	}
}

func (m Model) shouldRouteScrollToViewport(msg tea.KeyPressMsg) bool {
	if strings.TrimSpace(m.textarea.Value()) != "" {
		switch msg.String() {
		case "j", "k", "up", "down":
			return false
		}
	}
	switch msg.String() {
	case "up", "down", "j", "k", "pgup", "pgdown", "b", "f", "u", "d":
		return true
	default:
		return key.Matches(msg, m.viewport.KeyMap.Up) ||
			key.Matches(msg, m.viewport.KeyMap.Down) ||
			key.Matches(msg, m.viewport.KeyMap.PageUp) ||
			key.Matches(msg, m.viewport.KeyMap.PageDown)
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	text := strings.TrimSpace(m.textarea.Value())
	if m.busy || text == "" || m.runner == nil {
		return m, nil
	}

	m.textarea.Reset()
	m = m.syncLayout()
	m.busy = true
	m.lastError = ""
	m.statusMsg = ""
	m.statusLabel = ""
	m.interrupted = false
	m = m.appendUserMessage(text)
	m = m.appendEntry(TranscriptEntry{Kind: EntryAssistantChunk})
	m = m.syncViewportContent()

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
	return m, tea.Batch(waitStreamMsg(ch), m.spinner.Tick)
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
	if len(m.transcript) == 0 || m.transcript[len(m.transcript)-1].Kind != EntryAssistantChunk {
		e := TranscriptEntry{Kind: EntryAssistantChunk, Raw: text}
		e = m.renderEntry(e)
		m.transcript = append(m.transcript, e)
		return m
	}
	last := len(m.transcript) - 1
	m.transcript[last].Raw += text
	m.transcript[last] = m.renderEntry(m.transcript[last])
	return m
}

func (m Model) syncViewportContent() Model {
	wasAtBottom := m.viewport.AtBottom() || len(m.transcript) == 0
	content := m.renderTranscriptContent()
	m.viewport.SetContent(content)
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
	return m
}

func (m Model) syncLayout() Model {
	contentWidth := m.width
	if contentWidth <= 0 {
		contentWidth = 80
	}
	m.textarea.SetWidth(contentWidth)
	m.viewport.SetWidth(contentWidth)

	overhead := 4 // title + gaps + help
	if m.lastError != "" {
		overhead++
	}
	if m.statusMsg != "" {
		overhead++
	}
	if m.busy {
		overhead++
	}
	if m.approval != nil {
		overhead++
	}
	overhead += m.textarea.Height()

	viewportHeight := m.height - overhead
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	m.viewport.SetHeight(viewportHeight)
	return m
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
	m.approval = nil
	m.statusMsg = interruptedStatusMsg
	m.interrupted = true
	m = m.syncLayout()
	return m
}

