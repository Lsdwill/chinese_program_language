package lexer

import (
	"huayan/internal/source"
	"testing"
)

func FuzzLexerUnicodeNeverPanics(f *testing.F) {
	f.Add([]byte("让 名称 = \"华言\"\n打印(名称)"))
	f.Add([]byte{0xff, '/', '*', 'x'})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = Lex(&source.File{Name: "fuzz.hua", Text: string(data)})
	})
}
