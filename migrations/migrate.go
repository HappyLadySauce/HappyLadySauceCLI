// Package migrations applies all service database migrations.
// Package migrations 统一执行所有服务数据库迁移。
package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	contextmigrations "github.com/HappyLadySauce/HappyLadySauceCLI/migrations/context"
)

// Apply executes every registered service migration against the shared database.
// Services must register here instead of running migrations from repositories.
//
// Apply 对共享数据库执行所有已注册服务迁移。
// 各服务必须在这里注册迁移，不能从 repository 内部自行迁移。
func Apply(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("database is nil")
	}
	if err := contextmigrations.Apply(ctx, db); err != nil {
		return fmt.Errorf("apply context migrations: %w", err)
	}
	return nil
}
