package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagerCachesAndNumbersFiles(t *testing.T) {
	m := NewManager()
	a := m.Add("<交互>", "打印(1)")
	if a.ID != 1 {
		t.Fatalf("first source id = %d", a.ID)
	}
	if again := m.Add("<交互>", "不同内容"); again != a {
		t.Fatal("same source was not cached")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "a.hua")
	if err := os.WriteFile(path, []byte("让 a = 1"), 0644); err != nil {
		t.Fatal(err)
	}
	b, err := m.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if b.ID != 2 || b.Text != "让 a = 1" {
		t.Fatalf("loaded source = %#v", b)
	}
}

func TestLineColumnTreatsCRAsNewline(t *testing.T) {
	f := &File{Text: "甲\r乙\r\n丙"}
	if line, col := f.LineColumn(len("甲\r乙\r\n")); line != 3 || col != 1 {
		t.Fatalf("got %d:%d", line, col)
	}
	if got := f.LineText(2); got != "乙" {
		t.Fatalf("line text=%q", got)
	}
}

func TestManagerGetAndSpanValidityBoundaries(t *testing.T) {
	m := NewManager()
	if _, ok := m.Get("不存在"); ok {
		t.Fatal("missing source was found")
	}
	f := m.Add("a", "甲")
	got, ok := m.Get("a")
	if !ok || got != f {
		t.Fatalf("Get returned %#v, %v", got, ok)
	}
	for _, sp := range []Span{{}, {File: f, Start: -1}, {File: f, Start: 0}} {
		if sp.Valid() != (sp.File != nil && sp.Start >= 0) {
			t.Fatalf("span validity mismatch: %#v", sp)
		}
	}
	if line, col := f.LineColumn(-4); line != 1 || col != 1 {
		t.Fatalf("negative offset = %d:%d", line, col)
	}
	if got := f.LineText(0); got != "" || f.LineText(9) != "" {
		t.Fatalf("invalid line text: %q / %q", got, f.LineText(9))
	}
}

func TestManagerLoadReportsMissingFile(t *testing.T) {
	if _, err := NewManager().Load("/definitely/missing/华言.hua"); err == nil {
		t.Fatal("missing file was accepted")
	}
}
