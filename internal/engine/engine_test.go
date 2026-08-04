package engine

import (
	"bytes"
	"huayan/internal/bytecode"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompilerEmitsLocalAndUpvalueInstructions(t *testing.T) {
	_, ch, ds := Compile("<槽位>", "函数 创建()\n让 次数 = 0\n函数 增加()\n次数 = 次数 + 1\n返回 次数\n结束\n返回 增加\n结束")
	if len(ds) != 0 {
		t.Fatal(FormatDiagnostics(ds))
	}
	var outer *bytecode.FunctionProto
	for _, constant := range ch.Constants {
		if p, ok := constant.(*bytecode.FunctionProto); ok {
			outer = p
			break
		}
	}
	if outer == nil || outer.Chunk.Slots == 0 {
		t.Fatalf("outer function has no slots: %#v", outer)
	}
	hasLocal := false
	for _, ins := range outer.Chunk.Code {
		if ins.Op == bytecode.OpLoadLocal || ins.Op == bytecode.OpDeclareLocal {
			hasLocal = true
		}
	}
	if !hasLocal {
		t.Fatal("outer function did not emit local-slot instruction")
	}
	var inner *bytecode.FunctionProto
	for _, constant := range outer.Chunk.Constants {
		if p, ok := constant.(*bytecode.FunctionProto); ok {
			inner = p
			break
		}
	}
	hasUpvalue := false
	if inner != nil {
		for _, ins := range inner.Chunk.Code {
			if ins.Op == bytecode.OpLoadUpvalue || ins.Op == bytecode.OpStoreUpvalue {
				hasUpvalue = true
			}
		}
	}
	if inner == nil || !hasUpvalue || len(inner.Chunk.Upvalues) != 1 || inner.Chunk.Upvalues[0] != "次数" {
		t.Fatal("inner function did not emit upvalue instruction")
	}
}

func runText(t *testing.T, text string) string {
	t.Helper()
	var out bytes.Buffer
	e := New("", &out, strings.NewReader(""), nil)
	_, ch, ds := Compile("<测试>", text)
	if len(ds) != 0 {
		t.Fatalf("compile failed: %s", FormatDiagnostics(ds))
	}
	if _, re := e.VM.Execute(ch, e.VM.Globals()); re != nil {
		t.Fatalf("run failed: %s", FormatRuntime(re))
	}
	return out.String()
}

func TestCoreExecution(t *testing.T) {
	got := runText(t, "函数 阶乘(n)\n如果 n <= 1\n返回 1\n结束\n返回 n * 阶乘(n - 1)\n结束\n打印(阶乘(5))")
	if got != "120\n" {
		t.Fatalf("got %q", got)
	}
}

func TestClosureAndCollection(t *testing.T) {
	got := runText(t, "函数 创建()\n让 n = 0\n函数 增加()\nn = n + 1\n返回 n\n结束\n返回 增加\n结束\n让 f = 创建()\n让 xs = []\nxs.追加(f())\nxs.追加(f())\n打印(xs)")
	if got != "[1, 2]\n" {
		t.Fatalf("got %q", got)
	}
}

func TestModuleImportAndExport(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "工具.hua"), []byte("公开 函数 加一(n)\n返回 n + 1\n结束\n"), 0644); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "主.hua")
	if err := os.WriteFile(main, []byte("从 工具 导入 加一\n打印(加一(4))\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	e := New(dir, &out, strings.NewReader(""), nil)
	if _, re, ds := e.RunFile(main); len(ds) != 0 || re != nil {
		t.Fatalf("run failed: %v %v", ds, re)
	}
	if out.String() != "5\n" {
		t.Fatalf("got %q", out.String())
	}
}

func TestModuleRejectsPathEscapeAndReportsCycle(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(filepath.Dir(dir), "逃逸.hua")
	if err := os.WriteFile(outside, []byte("公开 让 秘密 = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e := New(dir, nil, strings.NewReader(""), nil)
	if _, err := e.resolve("../逃逸", ""); err == nil || !strings.Contains(err.Error(), "模块路径非法") {
		t.Fatalf("expected path rejection, error=%v", err)
	}
	main := filepath.Join(dir, "主.hua")
	if err := os.WriteFile(filepath.Join(dir, "甲.hua"), []byte("导入 乙\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "乙.hua"), []byte("导入 甲\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(main, []byte("导入 甲\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, re, ds := e.RunFile(main); len(ds) != 0 || re == nil || !strings.Contains(FormatRuntime(re), "检测到循环导入") {
		t.Fatalf("expected cycle rejection, diagnostics=%v runtime=%v", ds, re)
	}
}

func TestModuleNameMustMatchProjectPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "工具.hua")
	if err := os.WriteFile(path, []byte("模块 错误名\n打印(1)\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e := New(dir, nil, strings.NewReader(""), nil)
	if _, re, ds := e.RunFile(path); re != nil || len(ds) == 0 || !strings.Contains(FormatDiagnostics(ds), "模块名不匹配") {
		t.Fatalf("expected module-name diagnostic, diagnostics=%v runtime=%v", ds, re)
	}
}

func TestModuleInitializesOnceAndHidesPrivateNames(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "工具.hua"), []byte("模块 工具\n让 私有 = 1\n公开 函数 值()\n返回 私有\n结束\n打印(\"初始化\")\n"), 0644); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "主.hua")
	text := "模块 主\n导入 工具 为 一\n导入 工具 为 二\n打印(一.值())\n"
	if err := os.WriteFile(main, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	e := New(dir, &out, strings.NewReader(""), nil)
	if _, re, ds := e.RunFile(main); re != nil || len(ds) != 0 {
		t.Fatalf("run=%v diagnostics=%v", re, ds)
	}
	if out.String() != "初始化\n1\n" {
		t.Fatalf("output=%q", out.String())
	}
	privateMain := filepath.Join(dir, "主2.hua")
	if err := os.WriteFile(privateMain, []byte("模块 主2\n从 工具 导入 私有\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, re, ds := e.RunFile(privateMain); len(ds) != 0 || re == nil || !strings.Contains(FormatRuntime(re), "模块没有公开名称") {
		t.Fatalf("private import re=%v ds=%v", re, ds)
	}
}

func TestModuleRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "秘密.hua"), []byte("公开 让 值 = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "外部")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	e := New(root, nil, strings.NewReader(""), nil)
	if _, err := e.resolve("外部.秘密", ""); err == nil || !strings.Contains(err.Error(), "项目根目录之外") {
		t.Fatalf("err=%v", err)
	}
}

func TestCaughtError(t *testing.T) {
	got := runText(t, "尝试\n抛出 错误(\"坏了\")\n捕获 原因\n打印(原因.消息)\n结束")
	if got != "坏了\n" {
		t.Fatalf("got %q", got)
	}
}

func TestTimeStandardLibrary(t *testing.T) {
	got := runText(t, "从 标准.时间 导入 格式化, 日期差\n打印(格式化(\"2026-01-02T00:00:00Z\", \"2006/01/02\"))\n打印(日期差(\"2026-01-02T00:00:00Z\", \"2026-01-02T01:02:03Z\"))")
	if got != "2026/01/02\n3723\n" {
		t.Fatalf("got %q", got)
	}
}

func TestUnicodeTextLength(t *testing.T) {
	got := runText(t, "导入 标准.文字 为 文字\n打印(文字.长度(\"华😀言\"))")
	if got != "3\n" {
		t.Fatalf("got %q", got)
	}
}

func TestConstantAndOverflow(t *testing.T) {
	var out bytes.Buffer
	e := New("", &out, strings.NewReader(""), nil)
	_, ch, ds := Compile("<常量>", "常量 上限 = 9223372036854775807\n打印(上限 + 1)")
	if len(ds) != 0 {
		t.Fatalf("compile failed: %s", FormatDiagnostics(ds))
	}
	if _, re := e.VM.Execute(ch, e.VM.Globals()); re == nil || !strings.Contains(FormatRuntime(re), "整数溢出") {
		t.Fatalf("expected overflow, got %v", re)
	}

	_, ch, ds = Compile("<常量>", "常量 名字 = \"华言\"\n名字 = \"修改\"")
	if len(ds) != 0 {
		t.Fatalf("compile failed: %s", FormatDiagnostics(ds))
	}
	if _, re := e.VM.Execute(ch, e.VM.Globals()); re == nil || !strings.Contains(FormatRuntime(re), "常量") {
		t.Fatalf("expected constant error, got %v", re)
	}
}

func TestListLengthMutationDuringIteration(t *testing.T) {
	var out bytes.Buffer
	e := New("", &out, strings.NewReader(""), nil)
	_, ch, ds := Compile("<遍历>", "让 列表 = [1]\n遍历 项 于 列表\n列表.追加(2)\n结束")
	if len(ds) != 0 {
		t.Fatalf("compile failed: %s", FormatDiagnostics(ds))
	}
	if _, re := e.VM.Execute(ch, e.VM.Globals()); re == nil || !strings.Contains(FormatRuntime(re), "修改列表长度") {
		t.Fatalf("expected iteration error, got %v", re)
	}
}

func TestEngineCompileREPLAndMissingSourceDiagnostics(t *testing.T) {
	_, ch, ds := CompileREPL("<交互>", "旧名 + 1", map[string]bool{"旧名": true})
	if len(ds) != 0 || ch == nil {
		t.Fatalf("REPL compile failed: chunk=%v diagnostics=%v", ch, ds)
	}
	e := New("", nil, strings.NewReader(""), nil)
	if _, ds := e.CheckFile(filepath.Join(t.TempDir(), "不存在.hua")); len(ds) != 1 || ds[0].Code != "E3001" {
		t.Fatalf("missing source diagnostics=%v", ds)
	}
}

func TestRuntimeExitCodeAndFormattingBoundaries(t *testing.T) {
	if code, ok := RuntimeExitCode(nil); ok || code != 0 {
		t.Fatalf("nil exit code=%d,%v", code, ok)
	}
	if got := FormatRuntime(nil); got != "" {
		t.Fatalf("nil runtime=%q", got)
	}
	_, ch, ds := Compile("<退出>", "导入 标准.程序 为 程序\n程序.退出(7)")
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	e := New("", nil, strings.NewReader(""), nil)
	_, re := e.VM.Execute(ch, e.VM.Globals())
	if code, ok := RuntimeExitCode(re); !ok || code != 7 {
		t.Fatalf("exit code=%d,%v runtime=%v", code, ok, re)
	}
	if !strings.Contains(FormatRuntime(re), "程序请求退出") {
		t.Fatalf("runtime formatting=%q", FormatRuntime(re))
	}
}

func TestModuleRootInferenceAndNameValidationBoundaries(t *testing.T) {
	root := t.TempDir()
	service := filepath.Join(root, "服务")
	if err := os.Mkdir(service, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(service, "用户.华")
	if got := inferProjectRoot(root, file, "模块 服务.用户\r\n"); got != root {
		t.Fatalf("inferred root=%q, want %q", got, root)
	}
	if err := validateModuleName(root, file, "模块 服务.用户\r\n"); err != nil {
		t.Fatalf("valid module rejected: %v", err)
	}
	if err := validateModuleName(root, file, "模块 标准.用户\n"); err != nil {
		t.Fatalf("standard module rejected: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "用户.hua")
	if err := validateModuleName(root, outside, "模块 用户"); err == nil {
		t.Fatal("outside module accepted")
	}
	if got := inferProjectRoot(root, file, "打印(1)"); got != root {
		t.Fatalf("root changed without module: %q", got)
	}
}
