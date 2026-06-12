// Package status defines stable context status DTOs for UI and orchestration layers.
// Package status 定义 UI 与编排层使用的稳定 context 状态 DTO。
package status

import "time"

// Status is the stable post-turn status exposed outside the context domain.
// It intentionally does not expose persistence models such as Conversation.
//
// Status 是 context 域对外暴露的稳定回合状态。
// 它故意不暴露 Conversation 等持久化模型。
type Status struct {
	Elapsed       time.Duration
	Prompt        int
	Completion    int
	Total         int
	ContextTokens int
}

// IsZero reports whether the status has no renderable values.
// IsZero 判断状态是否没有可渲染数据。
func (s Status) IsZero() bool {
	return s.Elapsed == 0 && s.Prompt == 0 && s.Completion == 0 && s.Total == 0 && s.ContextTokens == 0
}
