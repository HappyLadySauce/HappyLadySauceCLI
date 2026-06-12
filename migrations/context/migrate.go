// Package context embeds and applies context module SQL migrations.
// Package context 嵌入并应用 context 模块的 SQL 迁移。
package context

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed *.sql
var files embed.FS

// Apply executes embedded SQL migrations in lexicographic file order.
// It is safe to call multiple times during startup.
//
// Apply 按文件名字典序执行嵌入的 SQL 迁移。
// 启动阶段可安全重复调用。
func Apply(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("context store database is nil")
	}

	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return fmt.Errorf("list context migrations: %w", err)
	}
	sort.Strings(names)

	for _, name := range names {
		sqlBytes, readErr := files.ReadFile(name)
		if readErr != nil {
			return fmt.Errorf("read context migration %q: %w", name, readErr)
		}
		if _, execErr := db.ExecContext(ctx, string(sqlBytes)); execErr != nil {
			return fmt.Errorf("apply context migration %q: %w", name, execErr)
		}
	}
	return nil
}
