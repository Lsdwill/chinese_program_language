package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplDelimiterDepthIgnoresStringsAndComments(t *testing.T) {
	if got := replDelimiterDepth("打印(\"(\") // ]\n"); got != 0 {
		t.Fatalf("depth = %d", got)
	}
	if got := replDelimiterDepth("让 x = [\n"); got != 1 {
		t.Fatalf("depth = %d", got)
	}
	if got := replDelimiterDepth("/* { */ 让 x = 1\n"); got != 0 {
		t.Fatalf("depth = %d", got)
	}
}

func TestReplBlockDeltaAndIndexOf(t *testing.T) {
	for _, line := range []string{"函数 加法()", "如果 真", "当 真", "遍历 项 于 []", "尝试"} {
		if replBlockDelta(line) != 1 {
			t.Fatalf("opening block %q was not counted", line)
		}
	}
	if replBlockDelta("结束") != -1 || replBlockDelta("打印(1)") != 0 {
		t.Fatal("block delta boundary failed")
	}
	if indexOf([]string{"甲", "乙"}, "乙") != 1 || indexOf([]string{"甲"}, "丙") != -1 {
		t.Fatal("indexOf boundary failed")
	}
}

func TestWalkHuaFindsBothExtensions(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "子目录")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"一.hua", "二.华", "忽略.txt"} {
		if err := os.WriteFile(filepath.Join(sub, name), []byte(""), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var files []string
	if err := walkHua(dir, &files); err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("files=%v", files)
	}
}
