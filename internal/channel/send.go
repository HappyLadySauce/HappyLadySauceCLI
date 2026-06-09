package channel

import (
	"bufio"
	"context"
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

// ReadLoop reads messages from reader and sends them to ContentCh until EOF or ctx cancellation.
// Close ContentCh when the loop exits.
// ReadLoop 从 reader 读取消息并写入 ContentCh，直到 EOF 或 ctx 取消；退出时关闭 ContentCh。
//
// Multiline input is supported in two ways:
// 多行输入支持两种方式：
//  1. Trailing "\" on a line continues reading on the next line.
//     Use "\\" at EOL for a literal backslash (e.g. Windows paths like C:\\).
//     行尾单个 "\" 表示续行；行尾 "\\" 表示字面量反斜杠（如 C:\\）。
//     Mid-line "\n" is two literal characters, not an escape sequence.
//     行内的 "\n" 是两个普通字符，不会变成换行。
//  2. A line containing only """ starts a block that ends on the next """ line.
//     单独一行的 """ 开启块模式，直到遇到下一个 """ 行结束。
//     "/" has no special meaning and is passed through unchanged.
//     "/" 无特殊含义，原样保留。
func (i *ContentChannel) readLoop(ctx context.Context, reader io.Reader) {
	defer close(i.contentCh)

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 1024*1024*10) // 10MB

	for {
		if ctx.Err() != nil {
			return
		}

		text, ok, err := readMessage(scanner)
		if err != nil {
			i.trySend(ctx, contentResult{Error: err})
			return
		}
		if !ok {
			return
		}
		if text == "" {
			continue
		}

		if !i.trySend(ctx, contentResult{Text: text}) {
			return
		}
	}
}

func (i *ContentChannel) trySend(ctx context.Context, result contentResult) bool {
	select {
	case i.contentCh <- result:
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
