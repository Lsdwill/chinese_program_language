package diagnostic

import (
	"huayan/internal/source"
	"strings"
	"testing"
)

func TestFormatValidAndInvalidSpans(t *testing.T) {
	if got := (Diagnostic{Severity: Error, Code: "E1", Message: "坏"}).Format(); !strings.Contains(got, "错误 E1：坏") {
		t.Fatal(got)
	}
	f := &source.File{Name: "x.hua", Text: "甲\r乙"}
	got := (Diagnostic{Severity: Error, Code: "E2", Message: "错", Hint: "修复", Span: source.Span{File: f, Start: len("甲\r"), End: len("甲\r乙")}}).Format()
	if !strings.Contains(got, "x.hua:2:1") || !strings.Contains(got, "建议：修复") {
		t.Fatal(got)
	}
}

func TestFormatClampsOutOfRangeSpanWithoutPanic(t *testing.T) {
	f := &source.File{Name: "边界.hua", Text: "甲"}
	for _, span := range []source.Span{{File: f, Start: 99, End: 120}, {File: f, Start: 0, End: -4}} {
		got := (Diagnostic{Severity: Error, Code: "E9", Message: "边界", Span: span}).Format()
		if !strings.Contains(got, "边界.hua") {
			t.Fatalf("formatted diagnostic=%q", got)
		}
	}
}
