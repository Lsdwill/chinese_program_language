package lexer

import (
	"huayan/internal/source"
	"huayan/internal/token"
	"testing"
)

func TestIdentifierJoinControlsAndNFC(t *testing.T) {
	text := "让 e\u0301 = 1\n打印(e\u0301)\n让 a\u200cb = 2"
	ts, ds := Lex(&source.File{Name: "xid", Text: text})
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	if ts[1].Kind != token.Identifier || ts[1].Normalized != "é" {
		t.Fatalf("token=%#v", ts[1])
	}
	found := false
	for _, tok := range ts {
		if tok.Kind == token.Identifier && tok.Literal == "a\u200cb" {
			found = true
		}
	}
	if !found {
		t.Fatalf("join-control identifier not accepted: %#v", ts)
	}
}
