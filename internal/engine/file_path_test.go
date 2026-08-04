package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStandardFileUsesProjectRootForRelativePaths(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	e := New(root, &out, strings.NewReader(""), nil)
	_, ch, ds := Compile("<文件>", "从 标准.文件 导入 写入文字, 读取文字\n写入文字(\"相对.txt\", \"华言\")\n打印(读取文字(\"相对.txt\"))")
	if len(ds) != 0 {
		t.Fatal(FormatDiagnostics(ds))
	}
	if _, re := e.VM.Execute(ch, e.VM.Globals()); re != nil {
		t.Fatal(FormatRuntime(re))
	}
	if out.String() != "华言\n" {
		t.Fatalf("output=%q", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "相对.txt")); err != nil {
		t.Fatalf("file not rooted at project: %v", err)
	}
}
