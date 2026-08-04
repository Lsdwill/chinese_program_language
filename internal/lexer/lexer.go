package lexer

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"huayan/internal/diagnostic"
	"huayan/internal/source"
	"huayan/internal/token"
)

type Lexer struct {
	file      *source.File
	text      string
	pos       int
	lineStart int
	depth     int
	last      token.Kind
	tokens    []token.Token
	errors    []diagnostic.Diagnostic
}

func Lex(file *source.File) ([]token.Token, []diagnostic.Diagnostic) {
	if file == nil {
		file = &source.File{}
	}
	start := 0
	if strings.HasPrefix(file.Text, "\ufeff") {
		start = len("\ufeff")
	}
	l := &Lexer{file: file, text: file.Text, pos: start, lineStart: 0, last: token.Newline}
	for l.pos < len(l.text) {
		l.scan()
	}
	end := source.Span{File: file, Start: len(file.Text), End: len(file.Text)}
	l.tokens = append(l.tokens, token.Token{Kind: token.EOF, Span: end})
	return l.tokens, l.errors
}

func (l *Lexer) span(start int) source.Span {
	return source.Span{File: l.file, Start: start, End: l.pos}
}

func (l *Lexer) add(k token.Kind, lit string, start int) {
	l.tokens = append(l.tokens, token.Token{Kind: k, Literal: lit, Span: l.span(start)})
	if k != token.Newline {
		l.last = k
	}
}

func (l *Lexer) error(start, end int, msg, hint string) {
	l.errors = append(l.errors, diagnostic.Diagnostic{Severity: diagnostic.Error, Code: "E1001", Message: msg, Hint: hint, Span: source.Span{File: l.file, Start: start, End: end}})
}

func (l *Lexer) scan() {
	start := l.pos
	r, size := utf8.DecodeRuneInString(l.text[l.pos:])
	if r == utf8.RuneError && size == 1 {
		l.pos++
		l.error(start, l.pos, "源文件包含无效的 UTF-8 字节", "")
		return
	}
	if r == '\r' {
		l.pos += size
		if l.pos < len(l.text) && l.text[l.pos] == '\n' {
			l.pos++
		}
		l.newline(start)
		return
	}
	if r == '\n' {
		l.pos++
		l.newline(start)
		return
	}
	if unicode.IsSpace(r) {
		l.pos += size
		return
	}
	if r == '/' && l.pos+1 < len(l.text) && l.text[l.pos+1] == '/' {
		l.pos += 2
		for l.pos < len(l.text) && l.text[l.pos] != '\n' && l.text[l.pos] != '\r' {
			_, n := utf8.DecodeRuneInString(l.text[l.pos:])
			l.pos += n
		}
		return
	}
	if r == '/' && l.pos+1 < len(l.text) && l.text[l.pos+1] == '*' {
		l.scanBlockComment(start)
		return
	}
	if r == '"' {
		l.scanString(start)
		return
	}
	if r >= '0' && r <= '9' {
		l.scanNumber(start)
		return
	}
	if isIdentStart(r) {
		l.scanIdentifier(start)
		return
	}
	if mapped, ok := fullWidth[r]; ok {
		r = mapped
	}
	if r == '\n' {
		l.pos += size
		l.newline(start)
		return
	}
	l.pos += size
	// The switch below is intentionally explicit: Chinese punctuation is
	// normalized only here, never by rewriting the source string.
	var k token.Kind
	switch r {
	case '(':
		k = token.LeftParen
		l.depth++
	case ')':
		k = token.RightParen
		if l.depth > 0 {
			l.depth--
		}
	case '[':
		k = token.LeftBracket
		l.depth++
	case ']':
		k = token.RightBracket
		if l.depth > 0 {
			l.depth--
		}
	case '{':
		k = token.LeftBrace
		l.depth++
	case '}':
		k = token.RightBrace
		if l.depth > 0 {
			l.depth--
		}
	case ',':
		k = token.Comma
	case ':':
		k = token.Colon
	case '.':
		k = token.Dot
	case ';':
		l.newline(start)
		return
	case '+':
		k = token.Plus
	case '-':
		k = token.Minus
	case '*':
		k = token.Star
	case '%':
		k = token.Percent
	case '=':
		if l.pos < len(l.text) && l.text[l.pos] == '=' {
			l.pos++
			k = token.Equal
		} else {
			k = token.Assign
		}
	case '!':
		if l.pos < len(l.text) && l.text[l.pos] == '=' {
			l.pos++
			k = token.NotEqual
		} else {
			l.error(start, l.pos, "感叹号只能与等号组成 !=", "")
			return
		}
	case '<':
		if l.pos < len(l.text) && l.text[l.pos] == '=' {
			l.pos++
			k = token.LessEqual
		} else {
			k = token.Less
		}
	case '>':
		if l.pos < len(l.text) && l.text[l.pos] == '=' {
			l.pos++
			k = token.GreaterEqual
		} else {
			k = token.Greater
		}
	case '/':
		k = token.Slash
	default:
		l.error(start, l.pos, "无法识别的字符："+string(r), "")
		return
	}
	l.add(k, l.text[start:l.pos], start)
}

// A dummy map lookup above only keeps delimiter handling separate from the
// punctuation switch; no token is emitted there.
func (l *Lexer) newline(start int) {
	if l.depth > 0 || continues(l.last) {
		return
	}
	if len(l.tokens) > 0 && l.tokens[len(l.tokens)-1].Kind == token.Newline {
		return
	}
	l.add(token.Newline, "\n", start)
}

func continues(k token.Kind) bool {
	switch k {
	case token.LeftParen, token.LeftBracket, token.LeftBrace, token.Comma, token.Colon, token.Dot, token.Plus, token.Minus, token.Star, token.Slash, token.Percent, token.Assign, token.Equal, token.NotEqual, token.Less, token.LessEqual, token.Greater, token.GreaterEqual, token.And, token.Or:
		return true
	}
	return false
}

func (l *Lexer) scanBlockComment(start int) {
	l.pos += 2
	for l.pos < len(l.text) {
		if l.pos+1 < len(l.text) && l.text[l.pos] == '*' && l.text[l.pos+1] == '/' {
			l.pos += 2
			return
		}
		l.pos++
	}
	l.error(start, l.pos, "多行注释没有结束", "补充 */")
}

func (l *Lexer) scanString(start int) {
	l.pos++
	escaped := false
	for l.pos < len(l.text) {
		r, n := utf8.DecodeRuneInString(l.text[l.pos:])
		if r == '\n' || r == '\r' {
			l.error(start, l.pos, "文字没有结束", "补充右引号")
			return
		}
		l.pos += n
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			raw := l.text[start:l.pos]
			value, err := strconv.Unquote(raw)
			if err != nil {
				l.error(start, l.pos, "文字中的转义序列无效", "支持 \\n、\\t、\\r、\\\\、\\\" 和 \\u 转义")
				return
			}
			l.add(token.String, value, start)
			return
		}
	}
	l.error(start, l.pos, "文字没有结束", "补充右引号")
}

func (l *Lexer) scanNumber(start int) {
	for l.pos < len(l.text) && l.text[l.pos] >= '0' && l.text[l.pos] <= '9' {
		l.pos++
	}
	k := token.Integer
	if l.pos+1 < len(l.text) && l.text[l.pos] == '.' && l.text[l.pos+1] >= '0' && l.text[l.pos+1] <= '9' {
		k = token.Float
		l.pos++
		for l.pos < len(l.text) && l.text[l.pos] >= '0' && l.text[l.pos] <= '9' {
			l.pos++
		}
	}
	l.add(k, l.text[start:l.pos], start)
}

func (l *Lexer) scanIdentifier(start int) {
	for l.pos < len(l.text) {
		r, n := utf8.DecodeRuneInString(l.text[l.pos:])
		if !isIdentContinue(r) {
			break
		}
		l.pos += n
	}
	lit := l.text[start:l.pos]
	normalized := norm.NFC.String(lit)
	if k, ok := token.Keywords[normalized]; ok {
		l.add(k, lit, start)
	} else {
		l.tokens = append(l.tokens, token.Token{Kind: token.Identifier, Literal: lit, Normalized: normalized, Span: l.span(start)})
		l.last = token.Identifier
	}
}

func isIdentStart(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.In(r, unicode.Nl) }
func isIdentContinue(r rune) bool {
	return isIdentStart(r) || unicode.IsDigit(r) || unicode.IsMark(r) || unicode.In(r, unicode.Pc) || r == '\u200c' || r == '\u200d'
}

var fullWidth = map[rune]rune{'（': '(', '）': ')', '［': '[', '］': ']', '｛': '{', '｝': '}', '，': ',', '：': ':', '；': ';'}
