package diagnostic

import (
	"fmt"
	"strings"

	"huayan/internal/source"
)

type Severity string

const (
	Error   Severity = "错误"
	Warning Severity = "警告"
)

type Diagnostic struct {
	Severity Severity
	Code     string
	Message  string
	Hint     string
	Span     source.Span
}

func (d Diagnostic) Error() string { return d.Format() }

func (d Diagnostic) Format() string {
	if !d.Span.Valid() {
		return fmt.Sprintf("%s %s：%s", d.Severity, d.Code, d.Message)
	}
	start := d.Span.Start
	if start < 0 {
		start = 0
	}
	if start > len(d.Span.File.Text) {
		start = len(d.Span.File.Text)
	}
	end := d.Span.End
	if end < start {
		end = start
	}
	if end > len(d.Span.File.Text) {
		end = len(d.Span.File.Text)
	}
	line, col := d.Span.File.LineColumn(start)
	text := d.Span.File.LineText(line)
	width := end - start
	if width < 1 {
		width = 1
	}
	// A byte span is converted to a rune underline width.
	lineStart := start
	for lineStart > 0 && d.Span.File.Text[lineStart-1] != '\n' && d.Span.File.Text[lineStart-1] != '\r' {
		lineStart--
	}
	underline := len([]rune(d.Span.File.Text[lineStart:start]))
	runes := len([]rune(d.Span.File.Text[start:end]))
	if runes < 1 {
		runes = 1
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s：%s\n  --> %s:%d:%d\n\n%d | %s\n   | %s^", d.Severity, d.Code, d.Message, d.Span.File.Name, line, col, line, text, strings.Repeat(" ", underline)+strings.Repeat("^", runes))
	if d.Hint != "" {
		b.WriteString("\n\n建议：" + d.Hint)
	}
	return b.String()
}
