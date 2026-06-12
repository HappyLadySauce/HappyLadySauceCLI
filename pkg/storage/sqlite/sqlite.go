// Package sqlite provides shared SQLite connection helpers.
// Package sqlite 提供共享的 SQLite 连接辅助能力。
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/HappyLadySauce/HappyLadySauceCLI/pkg/appdirs"
)

// DefaultDir returns the shared application data directory under the user home.
// It returns ~/.HAPPLADYSAUCECLI and does not create the directory.
//
// DefaultDir 返回用户 home 下的共享应用数据目录。
// 它返回 ~/.HAPPLADYSAUCECLI，但不会创建目录。
func DefaultDir() (string, error) {
	return appdirs.DefaultDir()
}

// DefaultPath returns a database path under ~/.HAPPLADYSAUCECLI.
// The filename must be a relative file name such as "context.sqlite".
//
// DefaultPath 返回 ~/.HAPPLADYSAUCECLI 下的数据库路径。
// filename 必须是类似 "context.sqlite" 的相对文件名。
func DefaultPath(filename string) (string, error) {
	if filename == "" {
		return "", errors.New("sqlite database filename is required")
	}
	if filepath.IsAbs(filename) || filepath.Base(filename) != filename {
		return "", fmt.Errorf("sqlite database filename must be a file name: %q", filename)
	}
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filename), nil
}

// OpenDefault opens a SQLite database under ~/.HAPPLADYSAUCECLI.
// The caller owns the returned connection and must close it.
//
// OpenDefault 打开 ~/.HAPPLADYSAUCECLI 下的 SQLite 数据库。
// 调用方拥有返回的连接并负责关闭。
func OpenDefault(ctx context.Context, filename string) (*sql.DB, error) {
	dbPath, err := DefaultPath(filename)
	if err != nil {
		return nil, err
	}
	return Open(ctx, dbPath)
}

// Open opens a SQLite database and applies shared connection pragmas.
// Parent directories are created with user-only permissions before opening.
//
// Open 打开 SQLite 数据库并应用共享连接 PRAGMA。
// 打开前会以仅当前用户可访问的权限创建父目录。
func Open(ctx context.Context, dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		return nil, errors.New("sqlite database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, fmt.Errorf("create sqlite database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := configure(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func configure(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("sqlite database is nil")
	}
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite database: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;
`); err != nil {
		return fmt.Errorf("configure sqlite database: %w", err)
	}
	return nil
}
