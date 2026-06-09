package input

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

const (
	// multilineDelimiter opens/closes explicit multiline input blocks.
	// multilineDelimiter 用于开启/结束显式多行输入块。
	multilineDelimiter = `"""`
	// continuationSuffix marks a line that continues on the next physical line.
	// continuationSuffix 标记当前行在下一物理行继续。
	continuationSuffix = `\`
)

// PromptResult contains one complete user prompt or a read error.
// PromptResult 包含一条完整用户输入或读取错误。
type PromptResult struct {
	Text  string
	Error error
}

// PromptReader reads complete prompts from an io.Reader.
// PromptReader 从 io.Reader 读取完整 prompt。
type PromptReader struct {
	results chan PromptResult
}

// NewPromptReader creates a PromptReader and starts its single producer goroutine.
// NewPromptReader 创建 PromptReader，并启动唯一的 producer goroutine。
func NewPromptReader(ctx context.Context, reader io.Reader) *PromptReader {
	promptReader := &PromptReader{
		results: make(chan PromptResult),
	}
	go promptReader.readLoop(ctx, reader)
	return promptReader
}

// Receive waits for the next complete prompt or context cancellation.
// Receive 等待下一条完整 prompt 或上下文取消。
func (r *PromptReader) Receive(ctx context.Context) (PromptResult, bool) {
	select {
	case result, ok := <-r.results:
		return result, ok
	case <-ctx.Done():
		return PromptResult{Error: fmt.Errorf("input cancelled: %w", ctx.Err())}, true
	}
}

// readLoop reads prompts until EOF, scanner error, or context cancellation.
// The results channel is closed exactly once by this producer.
// Standard stdin line mode cannot reliably distinguish Enter from Shift+Enter;
// explicit multiline input is handled by "\" continuation or """ blocks.
// readLoop 读取 prompt，直到 EOF、scanner 错误或上下文取消；结果通道仅由该 producer 关闭一次。
// 标准 stdin 行模式无法可靠区分 Enter 与 Shift+Enter；显式多行输入通过 "\" 续行或 """ 块处理。
func (r *PromptReader) readLoop(ctx context.Context, reader io.Reader) {
	defer close(r.results)

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 1024*1024*10) // 10MB

	for {
		if ctx.Err() != nil {
			return
		}

		text, ok, err := readMessage(scanner)
		if err != nil {
			r.trySend(ctx, PromptResult{Error: err})
			return
		}
		if !ok {
			return
		}
		if !r.trySend(ctx, PromptResult{Text: text}) {
			return
		}
	}
}

func (r *PromptReader) trySend(ctx context.Context, result PromptResult) bool {
	select {
	case r.results <- result:
		return true
	case <-ctx.Done():
		return false
	}
}

func readMessage(scanner *bufio.Scanner) (string, bool, error) {
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", false, err
		}
		return "", false, nil
	}

	line := scanner.Text()
	if strings.TrimSpace(line) == multilineDelimiter {
		return readMultilineBlock(scanner)
	}

	return readContinuedLines(scanner, line)
}

func readMultilineBlock(scanner *bufio.Scanner) (string, bool, error) {
	var lines []string
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == multilineDelimiter {
			return strings.Join(lines, "\n"), true, nil
		}
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", false, err
	}
	return strings.Join(lines, "\n"), true, nil
}

func readContinuedLines(scanner *bufio.Scanner, firstLine string) (string, bool, error) {
	var builder strings.Builder
	current := firstLine

	for {
		part, continues := splitLineContinuation(current)
		builder.WriteString(part)
		if !continues {
			break
		}
		builder.WriteString("\n")

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", false, err
			}
			return strings.TrimSpace(builder.String()), true, nil
		}
		current = scanner.Text()
	}

	return strings.TrimSpace(builder.String()), true, nil
}

// splitLineContinuation decides whether a physical line continues on the next line.
// An odd number of trailing "\" (after trimming trailing spaces/tabs) means continuation;
// pairs of "\" encode literal backslashes at end-of-line.
// splitLineContinuation 判断物理行是否续行；行尾奇数个 "\" 表示续行，成对 "\\" 表示字面量反斜杠。
func splitLineContinuation(line string) (content string, continues bool) {
	trimmedRight := strings.TrimRight(line, " \t")
	suffix := line[len(trimmedRight):]

	trailingBackslashes := 0
	for i := len(trimmedRight) - 1; i >= 0 && trimmedRight[i] == '\\'; i-- {
		trailingBackslashes++
	}
	if trailingBackslashes == 0 {
		return line, false
	}

	prefix := trimmedRight[:len(trimmedRight)-trailingBackslashes]
	if trailingBackslashes%2 == 1 {
		literalBackslashes := strings.Repeat(continuationSuffix, trailingBackslashes/2)
		return prefix + literalBackslashes, true
	}

	literalBackslashes := strings.Repeat(continuationSuffix, trailingBackslashes/2)
	return prefix + literalBackslashes + suffix, false
}
