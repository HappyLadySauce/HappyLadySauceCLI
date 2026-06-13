// Package files provides local filesystem execution primitives for built-in tools.
// Package files 提供内置工具使用的本地文件系统执行基础能力。
package files

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// DefaultReadMaxLines is the default file_read line budget.
	// DefaultReadMaxLines 是 file_read 默认读取行数上限。
	DefaultReadMaxLines = 200
	// MaxReadLines is the maximum file_read line budget.
	// MaxReadLines 是 file_read 单次允许读取的最大行数。
	MaxReadLines = 1000
	// DefaultListMaxEntries is the default file_list entry budget.
	// DefaultListMaxEntries 是 file_list 默认条目数上限。
	DefaultListMaxEntries = 200
	// MaxListEntries is the maximum file_list entry budget.
	// MaxListEntries 是 file_list 单次允许返回的最大条目数。
	MaxListEntries = 1000
	// DefaultMaxFileBytes is the default input file byte limit for file tools.
	// DefaultMaxFileBytes 是文件工具默认输入文件字节上限。
	DefaultMaxFileBytes = 16 << 20
	// DefaultMaxLineBytes is the default single-line byte limit for file_read.
	// DefaultMaxLineBytes 是 file_read 默认单行字节上限。
	DefaultMaxLineBytes = 64 << 10
	// DefaultMaxOutputBytes is the default tool output budget used by file tools.
	// DefaultMaxOutputBytes 是文件工具默认输出预算。
	DefaultMaxOutputBytes = 1 << 20
)

// Config contains safety limits for file execution.
// Config 包含文件执行的安全限制。
type Config struct {
	MaxFileBytes   int64
	MaxLineBytes   int
	MaxOutputBytes int
}

// Service executes file operations on already-authorized canonical paths.
// Service 在已授权的规范化路径上执行文件操作。
type Service struct {
	config Config
}

// ReadRequest contains one line-range text read.
// ReadRequest 表示一次按行范围读取文本文件的请求。
type ReadRequest struct {
	Path      string
	StartLine int
	MaxLines  int
}

// ReadResult contains bounded UTF-8 text content and range metadata.
// ReadResult 包含有界 UTF-8 文本内容与范围元数据。
type ReadResult struct {
	Content    string `json:"content"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	TotalLines int    `json:"total_lines"`
	Truncated  bool   `json:"truncated"`
}

// ListRequest contains one single-level directory listing request.
// ListRequest 表示一次单层目录列举请求。
type ListRequest struct {
	Path       string
	MaxEntries int
}

// ListEntry describes one directory child.
// ListEntry 描述一个目录子项。
type ListEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Type     string `json:"type"`
	Readable bool   `json:"readable"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

// ListResult contains bounded single-level directory entries.
// ListResult 包含有界单层目录条目。
type ListResult struct {
	Entries         []ListEntry `json:"entries"`
	ReturnedEntries int         `json:"returned_entries"`
	Truncated       bool        `json:"truncated"`
}

// EditRequest contains one exact text replacement.
// EditRequest 表示一次精确文本替换请求。
type EditRequest struct {
	Path    string
	OldText string
	NewText string
}

// EditResult describes a successful file edit.
// EditResult 描述一次成功的文件编辑。
type EditResult struct {
	Replacements  int    `json:"replacements"`
	BytesWritten  int64  `json:"bytes_written"`
	ContentSHA256 string `json:"content_sha256"`
}

// CreateRequest contains one new text file creation.
// CreateRequest 表示一次新文本文件创建请求。
type CreateRequest struct {
	Path    string
	Content string
}

// CreateResult describes a successful file creation.
// CreateResult 描述一次成功的文件创建。
type CreateResult struct {
	BytesWritten  int64  `json:"bytes_written"`
	ContentSHA256 string `json:"content_sha256"`
}

// DeleteRequest contains one regular file deletion.
// DeleteRequest 表示一次普通文件删除请求。
type DeleteRequest struct {
	Path string
}

// DeleteResult describes a successful file deletion.
// DeleteResult 描述一次成功的文件删除。
type DeleteResult struct {
	Deleted bool `json:"deleted"`
}

// NewService creates a file execution service.
// NewService 创建文件执行服务。
func NewService(config Config) *Service {
	return &Service{config: normalizeConfig(config)}
}

// ReadText reads a bounded UTF-8 line range from a regular file.
// ReadText 从普通文件读取有界 UTF-8 行范围。
func (s *Service) ReadText(ctx context.Context, req ReadRequest) (ReadResult, error) {
	startLine, maxLines, err := normalizeReadLimits(req.StartLine, req.MaxLines)
	if err != nil {
		return ReadResult{}, err
	}
	file, info, err := openVerifiedRegularFile(req.Path, s.config.MaxFileBytes)
	if err != nil {
		return ReadResult{}, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var builder strings.Builder
	var lineBuf bytes.Buffer
	var lineBytes, currentLine, endLine int
	var truncated bool
	outputBudget := newOutputBudget(s.config.MaxOutputBytes)
	for {
		if err := ctx.Err(); err != nil {
			return ReadResult{}, err
		}
		part, isPrefix, readErr := reader.ReadLine()
		if len(part) > 0 {
			lineBytes += len(part)
			if lineBytes > s.config.MaxLineBytes {
				return ReadResult{}, fmt.Errorf("file line exceeds security.file_max_line_bytes: %d > %d", lineBytes, s.config.MaxLineBytes)
			}
			lineBuf.Write(part)
		}
		if isPrefix {
			continue
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return ReadResult{}, fmt.Errorf("read file: %w", readErr)
		}
		if lineBuf.Len() > 0 || readErr == nil {
			currentLine++
			line := lineBuf.String()
			if !utf8.ValidString(line) {
				return ReadResult{}, fmt.Errorf("file is not valid UTF-8: %s", req.Path)
			}
			if currentLine >= startLine && currentLine < startLine+maxLines {
				if err := outputBudget.Add(len(line) + 1); err != nil {
					return ReadResult{}, err
				}
				builder.WriteString(line)
				if readErr == nil {
					builder.WriteByte('\n')
				}
				endLine = currentLine
			} else if currentLine >= startLine+maxLines {
				truncated = true
			}
			lineBuf.Reset()
			lineBytes = 0
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
	}

	return ReadResult{
		Content:    builder.String(),
		StartLine:  startLine,
		EndLine:    endLine,
		TotalLines: currentLine,
		Truncated:  truncated || readWasTruncated(startLine, endLine, currentLine, info.Size()),
	}, nil
}

// ListDirectory lists direct children of a directory without recursion.
// ListDirectory 非递归列举目录的直接子项。
func (s *Service) ListDirectory(ctx context.Context, req ListRequest) (ListResult, error) {
	maxEntries, err := normalizeListLimit(req.MaxEntries)
	if err != nil {
		return ListResult{}, err
	}
	dir, _, err := openVerifiedDirectory(req.Path)
	if err != nil {
		return ListResult{}, err
	}
	defer dir.Close()
	children, err := dir.ReadDir(maxEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return ListResult{}, fmt.Errorf("list directory: %w", err)
	}
	truncated := len(children) > maxEntries
	if truncated {
		children = children[:maxEntries]
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })

	outputBudget := newOutputBudget(s.config.MaxOutputBytes)
	entries := make([]ListEntry, 0, len(children))
	for _, child := range children {
		if err := ctx.Err(); err != nil {
			return ListResult{}, err
		}
		entry, err := describeEntry(req.Path, child)
		if err != nil {
			return ListResult{}, err
		}
		if err := outputBudget.Add(estimateListEntryBytes(entry)); err != nil {
			return ListResult{}, err
		}
		entries = append(entries, entry)
	}
	return ListResult{
		Entries:         entries,
		ReturnedEntries: len(entries),
		Truncated:       truncated,
	}, nil
}

// EditText applies a unique exact text replacement and writes atomically.
// EditText 执行唯一精确文本替换并原子写入。
func (s *Service) EditText(ctx context.Context, req EditRequest) (EditResult, error) {
	if err := ctx.Err(); err != nil {
		return EditResult{}, err
	}
	if req.OldText == "" {
		return EditResult{}, errors.New("old_text is required")
	}
	if !utf8.ValidString(req.OldText) || !utf8.ValidString(req.NewText) {
		return EditResult{}, errors.New("old_text and new_text must be valid UTF-8")
	}
	file, info, err := openVerifiedRegularFile(req.Path, s.config.MaxFileBytes)
	if err != nil {
		return EditResult{}, err
	}
	defer func() {
		if file != nil {
			_ = file.Close()
		}
	}()
	data, err := io.ReadAll(io.LimitReader(file, s.config.MaxFileBytes+1))
	if err != nil {
		return EditResult{}, fmt.Errorf("read file: %w", err)
	}
	if err := file.Close(); err != nil {
		return EditResult{}, fmt.Errorf("close file: %w", err)
	}
	file = nil
	if int64(len(data)) > s.config.MaxFileBytes {
		return EditResult{}, fmt.Errorf("file exceeds security.file_max_bytes: %d > %d", len(data), s.config.MaxFileBytes)
	}
	if err := ctx.Err(); err != nil {
		return EditResult{}, err
	}
	content := string(data)
	if !utf8.ValidString(content) {
		return EditResult{}, fmt.Errorf("file is not valid UTF-8: %s", req.Path)
	}
	count := strings.Count(content, req.OldText)
	if count != 1 {
		return EditResult{}, fmt.Errorf("old_text must match exactly once; matches=%d", count)
	}
	next := strings.Replace(content, req.OldText, req.NewText, 1)
	if int64(len(next)) > s.config.MaxFileBytes {
		return EditResult{}, fmt.Errorf("edited content exceeds security.file_max_bytes: %d > %d", len(next), s.config.MaxFileBytes)
	}
	if err := writeFileAtomically(req.Path, []byte(next), info.Mode().Perm()); err != nil {
		return EditResult{}, err
	}
	return EditResult{
		Replacements:  1,
		BytesWritten:  int64(len(next)),
		ContentSHA256: sha256String(next),
	}, nil
}

// CreateText creates a new UTF-8 text file.
// CreateText 创建新的 UTF-8 文本文件。
func (s *Service) CreateText(ctx context.Context, req CreateRequest) (CreateResult, error) {
	if err := ctx.Err(); err != nil {
		return CreateResult{}, err
	}
	if !utf8.ValidString(req.Content) {
		return CreateResult{}, errors.New("content must be valid UTF-8")
	}
	if int64(len(req.Content)) > s.config.MaxFileBytes {
		return CreateResult{}, fmt.Errorf("content exceeds security.file_max_bytes: %d > %d", len(req.Content), s.config.MaxFileBytes)
	}
	parent := filepath.Dir(req.Path)
	parentInfo, err := os.Stat(parent)
	if err != nil {
		if os.IsNotExist(err) {
			return CreateResult{}, fmt.Errorf("parent directory does not exist: %s", parent)
		}
		return CreateResult{}, fmt.Errorf("stat parent directory: %w", err)
	}
	if !parentInfo.IsDir() {
		return CreateResult{}, fmt.Errorf("parent path is not a directory: %s", parent)
	}
	if _, err := os.Lstat(req.Path); err == nil {
		return CreateResult{}, fmt.Errorf("file already exists: %s", req.Path)
	} else if !os.IsNotExist(err) {
		return CreateResult{}, fmt.Errorf("stat target file: %w", err)
	}
	file, err := os.OpenFile(req.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return CreateResult{}, fmt.Errorf("create file: %w", err)
	}
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = os.Remove(req.Path)
		}
	}()
	if _, err := file.WriteString(req.Content); err != nil {
		return CreateResult{}, fmt.Errorf("write file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return CreateResult{}, fmt.Errorf("sync file: %w", err)
	}
	if err := file.Close(); err != nil {
		return CreateResult{}, fmt.Errorf("close file: %w", err)
	}
	committed = true
	return CreateResult{
		BytesWritten:  int64(len(req.Content)),
		ContentSHA256: sha256String(req.Content),
	}, nil
}

// DeleteFile deletes a regular file and refuses directories.
// DeleteFile 删除普通文件并拒绝目录。
func (s *Service) DeleteFile(ctx context.Context, req DeleteRequest) (DeleteResult, error) {
	if err := ctx.Err(); err != nil {
		return DeleteResult{}, err
	}
	if _, err := lstatRegularFile(req.Path); err != nil {
		return DeleteResult{}, err
	}
	if err := os.Remove(req.Path); err != nil {
		return DeleteResult{}, fmt.Errorf("delete file: %w", err)
	}
	return DeleteResult{Deleted: true}, nil
}

func normalizeReadLimits(startLine, maxLines int) (int, int, error) {
	if startLine == 0 {
		startLine = 1
	}
	if maxLines == 0 {
		maxLines = DefaultReadMaxLines
	}
	if startLine < 1 {
		return 0, 0, errors.New("start_line must be greater than 0")
	}
	if maxLines < 1 || maxLines > MaxReadLines {
		return 0, 0, fmt.Errorf("max_lines must be between 1 and %d", MaxReadLines)
	}
	return startLine, maxLines, nil
}

func normalizeListLimit(maxEntries int) (int, error) {
	if maxEntries == 0 {
		maxEntries = DefaultListMaxEntries
	}
	if maxEntries < 1 || maxEntries > MaxListEntries {
		return 0, fmt.Errorf("max_entries must be between 1 and %d", MaxListEntries)
	}
	return maxEntries, nil
}

func normalizeConfig(config Config) Config {
	if config.MaxFileBytes == 0 {
		config.MaxFileBytes = DefaultMaxFileBytes
	}
	if config.MaxLineBytes == 0 {
		config.MaxLineBytes = DefaultMaxLineBytes
	}
	if config.MaxOutputBytes == 0 {
		config.MaxOutputBytes = DefaultMaxOutputBytes
	}
	return config
}

func readWasTruncated(startLine, endLine, totalLines int, fileSize int64) bool {
	if totalLines == 0 {
		return fileSize > 0
	}
	if startLine > 1 {
		return true
	}
	if endLine == 0 {
		return true
	}
	return endLine < totalLines
}

func openVerifiedRegularFile(path string, maxFileBytes int64) (*os.File, os.FileInfo, error) {
	before, err := lstatRegularFile(path)
	if err != nil {
		return nil, nil, err
	}
	file, err := openPathNoFollow(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open file: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, fmt.Errorf("stat opened file: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, fmt.Errorf("opened path is not a regular file: %s", path)
	}
	if !os.SameFile(before, info) {
		_ = file.Close()
		return nil, nil, fmt.Errorf("file changed during open: %s", path)
	}
	if maxFileBytes > 0 && info.Size() > maxFileBytes {
		_ = file.Close()
		return nil, nil, fmt.Errorf("file exceeds security.file_max_bytes: %d > %d", info.Size(), maxFileBytes)
	}
	return file, info, nil
}

func openVerifiedDirectory(path string) (*os.File, os.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("stat directory: %w", err)
	}
	if err := rejectLinkOrReparse(path, before); err != nil {
		return nil, nil, err
	}
	if !before.IsDir() {
		return nil, nil, fmt.Errorf("path is not a directory: %s", path)
	}
	file, err := openDirectoryNoFollow(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open directory: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, fmt.Errorf("stat opened directory: %w", err)
	}
	if !info.IsDir() {
		_ = file.Close()
		return nil, nil, fmt.Errorf("opened path is not a directory: %s", path)
	}
	if !os.SameFile(before, info) {
		_ = file.Close()
		return nil, nil, fmt.Errorf("directory changed during open: %s", path)
	}
	return file, info, nil
}

func lstatRegularFile(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	if err := rejectLinkOrReparse(path, info); err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path is not a regular file: %s", path)
	}
	return info, nil
}

func describeEntry(parent string, entry os.DirEntry) (ListEntry, error) {
	info, err := entry.Info()
	if err != nil {
		return ListEntry{}, fmt.Errorf("stat directory entry %q: %w", entry.Name(), err)
	}
	return ListEntry{
		Name:     entry.Name(),
		Path:     filepath.Join(parent, entry.Name()),
		Type:     entryType(parent, entry),
		Readable: entryReadable(parent, entry),
		Size:     info.Size(),
		Modified: info.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

func entryReadable(parent string, entry os.DirEntry) bool {
	if pathHasReparsePoint(filepath.Join(parent, entry.Name())) {
		return false
	}
	return entry.Type().IsRegular()
}

func entryType(parent string, entry os.DirEntry) string {
	if pathHasReparsePoint(filepath.Join(parent, entry.Name())) {
		return "symlink"
	}
	mode := entry.Type()
	switch {
	case mode.IsRegular():
		return "file"
	case mode.IsDir():
		return "directory"
	case mode&os.ModeSymlink != 0:
		return "symlink"
	default:
		return "other"
	}
}

func writeFileAtomically(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	temp, err := os.CreateTemp(dir, "."+base+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempName := temp.Name()
	committed := false
	defer func() {
		_ = temp.Close()
		if !committed {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(mode); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace file atomically: %w", err)
	}
	committed = true
	return nil
}

func sha256String(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

type outputBudget struct {
	max  int
	used int
}

func newOutputBudget(max int) *outputBudget {
	return &outputBudget{max: max}
}

func (b *outputBudget) Add(size int) error {
	if b == nil || b.max <= 0 {
		return nil
	}
	b.used += size
	if b.used > b.max {
		return fmt.Errorf("file tool output exceeds security.max_tool_output_bytes: %d > %d", b.used, b.max)
	}
	return nil
}

func estimateListEntryBytes(entry ListEntry) int {
	return len(entry.Name) + len(entry.Path) + len(entry.Type) + len(entry.Modified) + 96
}
