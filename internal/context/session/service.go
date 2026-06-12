// Package session owns context session lifecycle for interactive agent runs.
// Package session 管理交互式 agent 运行期间的 context session 生命周期。
package session

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/cloudwego/eino/schema"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/contextstore"
	contextmodel "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/model"
	contextstatus "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/status"
	contexttracker "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/tracker"
	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/logger"
	"github.com/HappyLadySauce/HappyLadySauceCLI/migrations"
	storagesqlite "github.com/HappyLadySauce/HappyLadySauceCLI/pkg/storage/sqlite"
)

// Service coordinates tracking, persistence, migrations, and database lifecycle for context sessions.
// Callers only receive stable Status values and never need contextstore/model details.
//
// Service 统一协调 context session 的 tracking、持久化、迁移与数据库生命周期。
// 调用方只接收稳定 Status，不需要感知 contextstore/model 细节。
type Service struct {
	db      *sql.DB
	store   *contextstore.Repository
	tracker *contexttracker.Tracker
	status  contextstatus.Status
}

// Open creates a context session service using the default context database.
// It applies root migrations before returning.
//
// Open 使用默认 context 数据库创建 context session 服务。
// 返回前会先执行根迁移。
func Open(ctx context.Context) (*Service, error) {
	db, err := storagesqlite.OpenDefault(ctx, "context.sqlite")
	if err != nil {
		return nil, fmt.Errorf("open context database: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = db.Close()
		}
	}()

	if err := migrations.Apply(ctx, db); err != nil {
		return nil, fmt.Errorf("apply database migrations: %w", err)
	}

	committed = true
	service := &Service{
		db:      db,
		store:   contextstore.New(db),
		tracker: contexttracker.New(),
	}
	logger.Info(ctx, 1, "Context session opened",
		"phase", "session_open",
		"session_id", service.tracker.Session().ID)
	return service, nil
}

// BeginTurn starts context tracking for one user interaction and returns the tracking context.
// BeginTurn 为一次用户交互启动 context tracking，并返回带 tracking 的 context。
func (s *Service) BeginTurn(ctx context.Context, userPrompt string) context.Context {
	if s == nil || s.tracker == nil {
		return ctx
	}
	conversation := s.tracker.BeginConversation()
	ctx = contexttracker.WithTracker(ctx, s.tracker)
	session := s.tracker.Session()
	return logger.AttachTurn(ctx, session.ID, conversation.ID, conversation.Sequence)
}

// CurrentTurnCount returns the number of model-call turns in the active conversation.
// CurrentTurnCount 返回当前活跃 conversation 中的模型调用 turn 数。
func (s *Service) CurrentTurnCount() int {
	if s == nil || s.tracker == nil {
		return 0
	}
	conversation := s.tracker.CurrentConversation()
	if conversation == nil {
		return 0
	}
	return len(conversation.Turns)
}

// FinishTurn finalizes tracking, persists snapshots, and returns a stable render status.
// FinishTurn 结束 tracking、持久化快照，并返回稳定的渲染状态。
func (s *Service) FinishTurn(ctx context.Context, messages []*schema.Message, runErr error) (contextstatus.Status, error) {
	if s == nil || s.tracker == nil {
		return contextstatus.Status{}, nil
	}
	s.tracker.SetMessages(messages)
	conversation := s.tracker.FinishConversation(runErr)
	contextTokens := s.tracker.TotalTokens()
	s.status = statusFromConversation(conversation, contextTokens)
	if err := s.persist(ctx, conversation); err != nil {
		logger.Error(ctx, err, "Could not save context turn", "phase", "persistence")
		return s.status, err
	}
	return s.status, nil
}

// Status returns the most recent stable post-turn status.
// Status 返回最近一次回合结束后的稳定状态。
func (s *Service) Status() contextstatus.Status {
	if s == nil {
		return contextstatus.Status{}
	}
	return s.status
}

// Close releases the underlying database connection.
// Close 释放底层数据库连接。
func (s *Service) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	sessionID := ""
	if s.tracker != nil && s.tracker.Session() != nil {
		sessionID = s.tracker.Session().ID
	}
	err := s.db.Close()
	s.db = nil
	if err != nil {
		logger.Error(context.Background(), err, "Could not close context session",
			"phase", "session_close",
			"session_id", sessionID)
		return err
	}
	logger.Info(context.Background(), 1, "Context session closed",
		"phase", "session_close",
		"session_id", sessionID)
	return nil
}

func (s *Service) persist(ctx context.Context, conversation *contextmodel.Conversation) error {
	if s == nil || s.store == nil || s.tracker == nil {
		return nil
	}
	if err := s.store.SaveSession(ctx, s.tracker.Session()); err != nil {
		return err
	}
	if err := s.store.SaveConversation(ctx, conversation); err != nil {
		return err
	}
	savedTurns := 0
	savedMessages := 0
	if conversation != nil {
		savedTurns = len(conversation.Turns)
		savedMessages = len(conversation.Messages)
	}
	logger.Info(ctx, 2, "Context conversation persisted",
		"phase", "persistence",
		"saved_turns", savedTurns,
		"saved_messages", savedMessages)
	return nil
}

func statusFromConversation(conversation *contextmodel.Conversation, contextTokens int) contextstatus.Status {
	if conversation == nil {
		return contextstatus.Status{}
	}
	return contextstatus.Status{
		Elapsed:       conversation.Elapsed,
		Prompt:        conversation.Prompt,
		Completion:    conversation.Completion,
		Total:         conversation.Total,
		ContextTokens: contextTokens,
	}
}
