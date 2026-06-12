// Package session owns context session lifecycle for interactive agent runs.
// Package session 管理交互式 agent 运行期间的 context session 生命周期。
package session

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/cloudwego/eino/schema"
	"k8s.io/klog/v2"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/contextstore"
	contextmodel "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/model"
	contextstatus "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/status"
	contexttracker "github.com/HappyLadySauce/HappyLadySauceCLI/internal/context/tracker"
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
	return &Service{
		db:      db,
		store:   contextstore.New(db),
		tracker: contexttracker.New(),
	}, nil
}

// BeginTurn starts context tracking for one user interaction and returns the tracking context.
// BeginTurn 为一次用户交互启动 context tracking，并返回带 tracking 的 context。
func (s *Service) BeginTurn(ctx context.Context) context.Context {
	if s == nil || s.tracker == nil {
		return ctx
	}
	s.tracker.BeginConversation()
	return contexttracker.WithTracker(ctx, s.tracker)
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
	klog.V(1).Infof(
		"context turn finished prompt=%d completion=%d total=%d content=%d elapsed_ms=%d messages=%d error=%t",
		s.status.Prompt,
		s.status.Completion,
		s.status.Total,
		s.status.ContextTokens,
		s.status.Elapsed.Milliseconds(),
		len(messages),
		runErr != nil,
	)
	if err := s.persist(ctx, conversation); err != nil {
		klog.Errorf("save context turn failed: %v", err)
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
	return s.db.Close()
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
