package token

import (
	"huayan/internal/source"
	"testing"
)

func TestKindStringAndTokenNames(t *testing.T) {
	for kind, want := range map[Kind]string{EOF: "文件结束", Identifier: "标识符", Record: "记录"} {
		if got := kind.String(); got != want {
			t.Fatalf("kind %d = %q, want %q", kind, got, want)
		}
	}
	if got := Kind(999).String(); got != "Token(999)" {
		t.Fatalf("unknown kind = %q", got)
	}
	tok := Token{Kind: Identifier, Literal: "原文", Normalized: "规范", Span: source.Span{}}
	if tok.Name() != "规范" || tok.String() != "标识符(\"原文\")" {
		t.Fatalf("token formatting = %q / %q", tok.Name(), tok.String())
	}
	plain := Token{Kind: Integer, Literal: "42"}
	if plain.Name() != "42" {
		t.Fatalf("literal name = %q", plain.Name())
	}
}

func TestKeywordsAreChineseLanguageKeywords(t *testing.T) {
	if Keywords["函数"] != Function || Keywords["记录"] != Record {
		t.Fatal("core keywords missing")
	}
	if _, ok := Keywords["function"]; ok {
		t.Fatal("English keyword unexpectedly accepted")
	}
}
