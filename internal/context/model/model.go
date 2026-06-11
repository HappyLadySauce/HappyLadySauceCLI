package model

import (
	"time"
)

// turn 与 conversation 是一对天然的层次关系：多个 turn 组成一个 conversation。

// Turn 记录一轮对话的耗时、输入输出 token 数和上下文 token 数。
// 细度：具体到Eino架构, 需采用ChatModel级别计量(具体表达: ReAct循环中的一次模型调用, 一次模型回复)
type Turn struct {
	Elapsed 	time.Duration	// 本轮耗时
	Prompt 	 int				// 本轮输入 token 数
	Completion int				// 本轮输出 token 数
	Total		int				// 本轮上下文 token 数
}

// Conversation 记录一次对话的耗时、输入输出 token 数和上下文 token 数。
// 细度：具体到Eino架构, 采用ChatModelAgent级别计量(具体表达: 一次ReAct循环, 一次用户对话交互)
type Conversation struct {
	Turns []*Turn				// 本轮对话的回合列表
	Elapsed time.Duration		// 本轮对话的总耗时
	Prompt int					// 本轮对话的总输入 token 数
	Completion int				// 本轮对话的总输出 token 数
	Total int					// 本轮对话的总上下文 token 数
}

// Session 记录一次会话的耗时、输入输出 token 数和上下文 token 数。
// 细度：无法具体到Eino架构, 采用自定义Session级别计量(具体表达: 用户总的对话交互)
type Session struct {
	Conversations []*Conversation	// 本会话的对话列表
	Elapsed time.Duration			// 本会话的总耗时
	Prompt int						// 本会话的总输入 token 数
	Completion int					// 本会话的总输出 token 数
	Total int						// 本会话的总上下文 token 数
}