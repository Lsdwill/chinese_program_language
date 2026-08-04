package lexer

import (
	"strings"
	"testing"

	"huayan/internal/source"
	"huayan/internal/token"
)

func TestChineseAndFullWidthPunctuation(t *testing.T) {
	ts, ds := Lex(&source.File{Name: "测试.hua", Text: "让 名字 =（\"华言\"）\n打印（名字）"})
	if len(ds) != 0 {
		t.Fatalf("unexpected diagnostics: %v", ds)
	}
	want := []token.Kind{token.Let, token.Identifier, token.Assign, token.LeftParen, token.String, token.RightParen, token.Newline, token.Identifier, token.LeftParen, token.Identifier, token.RightParen, token.EOF}
	if len(ts) != len(want) {
		t.Fatalf("got %d tokens, want %d: %#v", len(ts), len(want), ts)
	}
	for i, k := range want {
		if ts[i].Kind != k {
			t.Fatalf("token %d: got %s, want %s", i, ts[i].Kind, k)
		}
	}
}

func TestStringPreservesFullWidthCharacters(t *testing.T) {
	ts, ds := Lex(&source.File{Name: "文字.hua", Text: "打印(\"（保留）\")"})
	if len(ds) != 0 || len(ts) < 3 {
		t.Fatalf("lex failed: %v %#v", ds, ts)
	}
	if ts[2].Literal != "（保留）" {
		t.Fatalf("string was normalized: %q", ts[2].Literal)
	}
}

func TestUnclosedStringHasPosition(t *testing.T) {
	f := &source.File{Name: "错误.hua", Text: "打印(\"未结束"}
	_, ds := Lex(f)
	if len(ds) != 1 {
		t.Fatalf("got %d diagnostics", len(ds))
	}
	line, col := ds[0].Span.File.LineColumn(ds[0].Span.Start)
	if line != 1 || col != 4 {
		t.Fatalf("got position %d:%d", line, col)
	}
}

func TestLexerReportsInvalidPunctuationAndComments(t *testing.T) {
	_, ds := Lex(&source.File{Name: "错误.hua", Text: "!\n/* 未结束"})
	if len(ds) != 2 {
		t.Fatalf("diagnostics=%v", ds)
	}
	if !strings.Contains(ds[0].Message, "感叹号") || !strings.Contains(ds[1].Message, "多行注释") {
		t.Fatalf("unexpected diagnostics=%v", ds)
	}
}

func TestLexerNumbersSemicolonAndEscapes(t *testing.T) {
	ts, ds := Lex(&source.File{Name: "数字.hua", Text: "1 2.5; 打印(\"a\\n\")"})
	if len(ds) != 0 {
		t.Fatalf("diagnostics=%v", ds)
	}
	if ts[0].Kind != token.Integer || ts[1].Kind != token.Float || ts[2].Kind != token.Newline {
		t.Fatalf("tokens=%#v", ts)
	}
	if ts[5].Literal != "a\n" {
		t.Fatalf("escape literal=%q", ts[5].Literal)
	}
}

func TestLexerOperatorsCommentsAndInvalidEscapes(t *testing.T) {
	text := "让 x = 1 + 2 - 3 * 4 / 5 % 2\n" +
		"x == 1 且 x != 0 或 x <= 3 且 x >= 1\n" +
		"（［｛｝］），：； // 注释\r\n/* 完整注释 */\n"
	ts, ds := Lex(&source.File{Name: "operators.hua", Text: text})
	if len(ds) != 0 || len(ts) < 20 {
		t.Fatalf("tokens=%#v diagnostics=%v", ts, ds)
	}
	_, ds = Lex(&source.File{Name: "escape.hua", Text: "打印(\"\\q\")"})
	if len(ds) != 1 || !strings.Contains(ds[0].Message, "转义") {
		t.Fatalf("invalid escape diagnostics=%v", ds)
	}
}

func TestContinuesRecognizesExpressionTerminators(t *testing.T) {
	for _, kind := range []token.Kind{token.LeftParen, token.LeftBracket, token.LeftBrace, token.Comma, token.Colon, token.Dot, token.Plus, token.Minus, token.Star, token.Slash, token.Percent, token.Assign, token.Equal, token.NotEqual, token.Less, token.LessEqual, token.Greater, token.GreaterEqual, token.And, token.Or} {
		if !continues(kind) {
			t.Fatalf("%s should continue expression", kind)
		}
	}
	for _, kind := range []token.Kind{token.Identifier, token.Newline, token.RightParen, token.Integer} {
		if continues(kind) {
			t.Fatalf("%s should terminate expression", kind)
		}
	}
}

func TestBOMIsIgnoredWithoutShiftingSpans(t *testing.T) {
	f := &source.File{Name: "bom.hua", Text: "\ufeff打印(1)"}
	ts, ds := Lex(f)
	if len(ds) != 0 || ts[0].Kind != token.Identifier || ts[0].Literal != "打印" {
		t.Fatalf("BOM was not ignored: %#v %v", ts, ds)
	}
	if ts[0].Span.Start != len("\ufeff") {
		t.Fatalf("unexpected span start: %d", ts[0].Span.Start)
	}
}

func TestIdentifierNFCNormalization(t *testing.T) {
	sourceText := "让 cafe\u0301 = 7\n打印(café)"
	ts, ds := Lex(&source.File{Name: "nfc.hua", Text: sourceText})
	if len(ds) != 0 {
		t.Fatalf("unexpected diagnostics: %v", ds)
	}
	if ts[1].Literal == ts[1].Normalized || ts[1].Normalized != "café" {
		t.Fatalf("NFC was not retained: %#v", ts[1])
	}
}

func FuzzLexerNeverPanics(f *testing.F) {
	f.Add([]byte("让 名字 = [1, 2, 3]"))
	f.Add([]byte{0xff, 0xfe, 0x00, '\n'})
	f.Fuzz(func(t *testing.T, data []byte) { Lex(&source.File{Name: "fuzz.hua", Text: string(data)}) })
}
