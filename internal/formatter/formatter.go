package formatter

import "strings"

// Format performs the deliberately conservative first-version formatting:
// line endings are normalized, statement semicolons outside strings/comments
// become newlines, and trailing whitespace is removed. It never rewrites
// string contents or comment contents.
func Format(text string) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	var out strings.Builder
	inString, escaped, lineComment, blockComment := false, false, false, false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if lineComment {
			out.WriteByte(c)
			if c == '\n' {
				lineComment = false
			}
			continue
		}
		if blockComment {
			out.WriteByte(c)
			if c == '*' && i+1 < len(text) && text[i+1] == '/' {
				out.WriteByte('/')
				i++
				blockComment = false
			}
			continue
		}
		if inString {
			out.WriteByte(c)
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			out.WriteByte(c)
			continue
		}
		if c == '/' && i+1 < len(text) && text[i+1] == '/' {
			out.WriteString("//")
			i++
			lineComment = true
			continue
		}
		if c == '/' && i+1 < len(text) && text[i+1] == '*' {
			out.WriteString("/*")
			i++
			blockComment = true
			continue
		}
		if c == ';' {
			out.WriteByte('\n')
			continue
		}
		out.WriteByte(c)
	}
	return formatIndentation(out.String())
}

func formatIndentation(text string) string {
	lines := strings.Split(text, "\n")
	var out strings.Builder
	blockDepth, delimiterDepth := 0, 0
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			if i < len(lines)-1 {
				out.WriteByte('\n')
			}
			continue
		}
		if closesDelimiter(trimmed) && delimiterDepth > 0 {
			delimiterDepth--
		}
		if isDedentLine(trimmed) && blockDepth > 0 {
			blockDepth--
		}
		level := blockDepth + delimiterDepth
		out.WriteString(strings.Repeat("    ", level))
		out.WriteString(trimmed)
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
		delimiterDepth += delimiterDelta(trimmed)
		if delimiterDepth < 0 {
			delimiterDepth = 0
		}
		if isBlockOpener(trimmed) {
			blockDepth++
		}
	}
	return out.String()
}

func firstWord(line string) string {
	for i, r := range line {
		if r == ' ' || r == '\t' || r == '（' || r == '(' {
			return line[:i]
		}
	}
	return line
}

func isDedentLine(line string) bool {
	word := firstWord(line)
	return word == "结束" || word == "否则" || word == "否则如果" || strings.HasPrefix(line, "否则 ") || word == "捕获"
}
func isBlockOpener(line string) bool {
	word := firstWord(line)
	return word == "函数" || word == "如果" || word == "当" || word == "遍历" || word == "尝试" || word == "否则" || word == "否则如果" || strings.HasPrefix(line, "否则 ") || word == "捕获" || strings.HasPrefix(line, "公开 函数")
}
func closesDelimiter(line string) bool {
	return strings.HasPrefix(line, ")") || strings.HasPrefix(line, "]") || strings.HasPrefix(line, "}") || strings.HasPrefix(line, "）") || strings.HasPrefix(line, "］") || strings.HasPrefix(line, "｝")
}
func delimiterDelta(line string) int {
	depth := 0
	inString, escaped, lineComment, blockComment := false, false, false, false
	var previous rune
	for _, c := range line {
		if lineComment {
			previous = c
			continue
		}
		if blockComment {
			if previous == '*' && c == '/' {
				blockComment = false
			}
			previous = c
			continue
		}
		if inString {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			previous = c
			continue
		}
		if c == '"' {
			inString = true
			previous = c
			continue
		}
		if previous == '/' && c == '/' {
			lineComment = true
			previous = c
			continue
		}
		if previous == '/' && c == '*' {
			blockComment = true
			previous = c
			continue
		}
		switch c {
		case '(', '[', '{', '（', '［', '｛':
			depth++
		case ')', ']', '}', '）', '］', '｝':
			depth--
		}
		previous = c
	}
	return depth
}
