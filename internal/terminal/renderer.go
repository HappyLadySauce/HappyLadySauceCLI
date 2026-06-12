package terminal

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/tools/toolresult"
)

const thinkingFrameInterval = 120 * time.Millisecond

const (
	colorReset           = "\x1b[0m"
	colorUser            = "\x1b[32m"
	colorAgent           = "\x1b[36m"
	colorThinking        = "\x1b[33m"
	colorTool            = "\x1b[35m"
	colorError           = "\x1b[31m"
	colorStats           = "\x1b[90m"
	colorStatsElapsed    = "\x1b[36m"
	colorStatsPrompt     = "\x1b[32m"
	colorStatsCompletion = "\x1b[35m"
	colorStatsContent    = "\x1b[37;1m"
	colorStatsWindow     = "\x1b[33m"
)

// Renderer writes interactive CLI output.
// Renderer 负责写入交互式 CLI 输出。
type Renderer struct {
	out              io.Writer
	errOut           io.Writer
	colorEnabled     bool
	mu               sync.Mutex
	thinkingStopCh   chan struct{}
	thinkingDoneCh   chan struct{}
	thinkingAnimated bool
}

// NewRenderer creates a terminal renderer with separate stdout and stderr writers.
// NewRenderer 使用独立 stdout/stderr writer 创建终端渲染器。
func NewRenderer(out, errOut io.Writer) *Renderer {
	return &Renderer{
		out:          out,
		errOut:       errOut,
		colorEnabled: isTerminalWriter(out) || isTerminalWriter(errOut),
	}
}

// Prompt renders the user prompt marker.
// Prompt 渲染用户输入提示符。
func (r *Renderer) Prompt() {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = fmt.Fprint(r.out, r.colorize(colorUser, "User> "))
}

// AfterUserInput separates a completed user prompt from the next agent output.
// AfterUserInput 将已完成用户输入与下一段 agent 输出分隔开。
func (r *Renderer) AfterUserInput() {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = fmt.Fprintln(r.out)
}

// FinishTurn separates a completed agent turn from the next user prompt.
// FinishTurn 将已完成 agent 轮次与下一次用户输入分隔开。
func (r *Renderer) FinishTurn() {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = fmt.Fprintln(r.out)
}

// AgentLabel renders an agent label before assistant content.
// AgentLabel 在 assistant 内容前渲染 agent 标签。
func (r *Renderer) AgentLabel(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agentLabelLocked(name)
}

// ThinkingLabel renders an agent thinking label before reasoning content.
// ThinkingLabel 在 reasoning 内容前渲染 agent thinking 标签。
func (r *Renderer) ThinkingLabel(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name == "" {
		name = "agent"
	}
	label := fmt.Sprintf("%s[thinking]> ", name)
	_, _ = fmt.Fprint(r.out, r.colorize(colorThinking, label))
}

// StartThinkingAnimation renders a lightweight spinner while waiting for model output.
// StartThinkingAnimation 在等待模型输出时渲染轻量 thinking 动画。
func (r *Renderer) StartThinkingAnimation(name string) {
	if !isTerminalWriter(r.out) {
		return
	}

	r.mu.Lock()
	if r.thinkingStopCh != nil {
		r.mu.Unlock()
		return
	}
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	r.thinkingStopCh = stopCh
	r.thinkingDoneCh = doneCh
	r.thinkingAnimated = true
	r.mu.Unlock()

	go r.runThinkingAnimation(name, stopCh, doneCh)
}

// StopThinkingAnimation stops and clears the current thinking animation.
// StopThinkingAnimation 停止并清除当前 thinking 动画。
func (r *Renderer) StopThinkingAnimation() {
	r.mu.Lock()
	stopCh := r.thinkingStopCh
	doneCh := r.thinkingDoneCh
	animated := r.thinkingAnimated
	r.thinkingStopCh = nil
	r.thinkingDoneCh = nil
	r.thinkingAnimated = false
	r.mu.Unlock()

	if stopCh == nil {
		return
	}
	close(stopCh)
	<-doneCh

	if animated {
		r.mu.Lock()
		_, _ = fmt.Fprint(r.out, "\r"+strings.Repeat(" ", 80)+"\r")
		r.mu.Unlock()
	}
}

func (r *Renderer) runThinkingAnimation(name string, stopCh <-chan struct{}, doneCh chan<- struct{}) {
	defer close(doneCh)
	if name == "" {
		name = "agent"
	}
	frames := []string{"|", "/", "-", "\\"}
	ticker := time.NewTicker(thinkingFrameInterval)
	defer ticker.Stop()

	index := 0
	for {
		r.mu.Lock()
		label := fmt.Sprintf("%s[thinking]> %s", name, frames[index%len(frames)])
		_, _ = fmt.Fprint(r.out, "\r"+r.colorize(colorThinking, label))
		r.mu.Unlock()
		index++

		select {
		case <-stopCh:
			return
		case <-ticker.C:
		}
	}
}

func (r *Renderer) agentLabelLocked(name string) {
	if name == "" {
		name = "agent"
	}
	label := fmt.Sprintf("%s> ", name)
	_, _ = fmt.Fprint(r.out, r.colorize(colorAgent, label))
}

// Token renders one streaming content chunk without forcing a newline.
// Token 渲染一个流式内容片段，不强制换行。
func (r *Renderer) Token(content string) {
	_, _ = r.Write([]byte(content))
}

// Write writes raw stream bytes to the renderer output.
// Write 将原始流字节写入 renderer 输出。
func (r *Renderer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.out.Write(p)
}

// ToolMessage renders a completed tool message.
// ToolMessage 渲染一条完整工具消息。
func (r *Renderer) ToolMessage(toolName, content string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if toolName == "" {
		toolName = "tool"
	}
	label := fmt.Sprintf("%s> ", toolName)
	if toolresult.IsErrorPayload(content) {
		_, _ = fmt.Fprintf(r.out, "%s%s\n", r.colorize(colorError, label+"[tool error] "), r.colorize(colorError, content))
		return
	}
	_, _ = fmt.Fprintf(r.out, "%s%s\n", r.colorize(colorTool, label), content)
}

// ApprovalPrompt renders a security approval prompt without reading input.
// ApprovalPrompt 渲染安全审批提示，但不读取输入。
func (r *Renderer) ApprovalPrompt(message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = fmt.Fprint(r.errOut, r.colorize(colorStats, message))
}

// EmitAgentEvent handles a structured agent stream event.
// kind uses the string constants defined by internal/agents.AgentStreamEvent*.
// The method intentionally accepts strings instead of importing internal/agents
// so terminal stays a pure output adapter and avoids a package import cycle.
// EmitAgentEvent 处理结构化 agent 流事件；kind 使用 internal/agents.AgentStreamEvent* 字符串常量。
// 该方法故意使用字符串而不是导入 internal/agents，避免 terminal 与 agents 形成循环依赖。
func (r *Renderer) EmitAgentEvent(kind string, agentName string, toolName string, content string, err error) {
	switch kind {
	case "thinking_started":
		r.StartThinkingAnimation(agentName)
	case "thinking_stopped":
		r.StopThinkingAnimation()
	case "thinking_content_started":
		r.ThinkingLabel(agentName)
	case "answer_content_started":
		r.AgentLabel(agentName)
	case "message_finished":
		r.FinishMessage()
	case "tool_message":
		r.ToolMessage(toolName, content)
	case "error":
		r.Error(err)
	case "exit":
		r.Exit()
	}
}

// Error renders an error message to stderr.
// Error 将错误消息渲染到 stderr。
func (r *Renderer) Error(err error) {
	if err == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = fmt.Fprintf(r.errOut, "%s\n", r.colorize(colorError, fmt.Sprintf("Error: %v", err)))
}

// FinishMessage terminates the current rendered message.
// FinishMessage 结束当前渲染消息。
func (r *Renderer) FinishMessage() {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = fmt.Fprintln(r.out)
}

// Exit renders an agent exit notice.
// Exit 渲染 agent 退出提示。
func (r *Renderer) Exit() {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = fmt.Fprintln(r.out, "Agent exited.")
}

func (r *Renderer) colorize(color, content string) string {
	if !r.colorEnabled {
		return content
	}
	return color + content + colorReset
}

func isTerminalWriter(writer io.Writer) bool {
	_, ok := writer.(*os.File)
	return ok
}
