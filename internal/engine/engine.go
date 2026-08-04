package engine

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"huayan/internal/ast"
	"huayan/internal/bytecode"
	"huayan/internal/compiler"
	"huayan/internal/diagnostic"
	"huayan/internal/lexer"
	"huayan/internal/parser"
	"huayan/internal/source"
	"huayan/internal/vm"
)

type Engine struct {
	VM        *vm.VM
	Root      string
	Sources   *source.Manager
	loading   map[string]bool
	loadStack []string
}

func New(root string, out io.Writer, in io.Reader, args []string) *Engine {
	if root == "" {
		root = "."
	}
	root, _ = filepath.Abs(root)
	e := &Engine{Root: root, Sources: source.NewManager(), loading: map[string]bool{}}
	e.VM = vm.New(out, in, args)
	e.VM.WorkingDir = root
	e.VM.Loader = e.loadModule
	return e
}

func Compile(name, text string) (*ast.Program, *bytecode.Chunk, []diagnostic.Diagnostic) {
	return compileSource(name, text, nil, false)
}
func CompileREPL(name, text string, globals map[string]bool) (*ast.Program, *bytecode.Chunk, []diagnostic.Diagnostic) {
	return compileSource(name, text, globals, true)
}
func compileSource(name, text string, globals map[string]bool, repl bool) (*ast.Program, *bytecode.Chunk, []diagnostic.Diagnostic) {
	return compileFile(&source.File{Name: name, Text: text}, globals, repl)
}
func compileFile(f *source.File, globals map[string]bool, repl bool) (*ast.Program, *bytecode.Chunk, []diagnostic.Diagnostic) {
	tokens, le := lexer.Lex(f)
	program, pe := parser.Parse(tokens)
	errs := append(le, pe...)
	if len(errs) > 0 {
		return program, nil, errs
	}
	var chunk *bytecode.Chunk
	var ce []diagnostic.Diagnostic
	if repl {
		chunk, ce = compiler.CompileREPL(program, globals)
	} else if globals != nil {
		chunk, ce = compiler.CompileWithGlobals(program, globals)
	} else {
		chunk, ce = compiler.Compile(program)
	}
	return program, chunk, append(errs, ce...)
}
func (e *Engine) RunFile(path string) (vm.Value, *vm.RuntimeError, []diagnostic.Diagnostic) {
	abs, _ := filepath.Abs(path)
	file, err := e.Sources.Load(abs)
	if err != nil {
		return vm.Nil(), nil, []diagnostic.Diagnostic{{Severity: diagnostic.Error, Code: "E3001", Message: "读取源文件失败：" + err.Error()}}
	}
	e.Root = inferProjectRoot(e.Root, abs, file.Text)
	if err := validateModuleName(e.Root, abs, file.Text); err != nil {
		return vm.Nil(), nil, []diagnostic.Diagnostic{{Severity: diagnostic.Error, Code: "E3004", Message: err.Error()}}
	}
	program, ch, ds := compileFile(file, nil, false)
	if len(ds) > 0 {
		return vm.Nil(), nil, ds
	}
	ch.Name = abs
	mod := vm.Value{Kind: vm.ModuleKind, Data: &vm.ModuleObject{Path: abs, Exports: map[string]vm.Value{}}}
	env := vm.NewEnv(e.VM.Globals(), mod.Data.(*vm.ModuleObject))
	_ = program
	value, re := e.VM.Execute(ch, env)
	return value, re, nil
}

func inferProjectRoot(current, file, text string) string {
	first := ""
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "模块 ") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "模块 "))
			first = strings.Split(path, ".")[0]
			break
		}
	}
	if first == "" {
		return current
	}
	dir := filepath.Dir(file)
	for {
		if filepath.Base(dir) == first {
			return filepath.Dir(dir)
		}
		candidate := filepath.Join(dir, first)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return current
}
func (e *Engine) CheckFile(path string) (*bytecode.Chunk, []diagnostic.Diagnostic) {
	abs, _ := filepath.Abs(path)
	f, err := e.Sources.Load(abs)
	if err != nil {
		return nil, []diagnostic.Diagnostic{{Severity: diagnostic.Error, Code: "E3001", Message: "读取源文件失败：" + err.Error()}}
	}
	e.Root = inferProjectRoot(e.Root, abs, f.Text)
	if err := validateModuleName(e.Root, abs, f.Text); err != nil {
		return nil, []diagnostic.Diagnostic{{Severity: diagnostic.Error, Code: "E3004", Message: err.Error(), Span: source.Span{File: f, Start: 0, End: 0}}}
	}
	_, ch, ds := compileFile(f, nil, false)
	if ch != nil {
		ch.Name = abs
	}
	return ch, ds
}

func (e *Engine) loadModule(path, from string) (vm.Value, error) {
	file, err := e.resolve(path, from)
	if err != nil {
		return vm.Nil(), err
	}
	if e.loading[file] {
		chain := append(append([]string(nil), e.loadStack...), file)
		return vm.Nil(), errors.New("检测到循环导入：" + strings.Join(chain, " -> "))
	}
	e.loading[file] = true
	e.loadStack = append(e.loadStack, file)
	defer func() {
		delete(e.loading, file)
		e.loadStack = e.loadStack[:len(e.loadStack)-1]
	}()
	b, err := os.ReadFile(file)
	if err != nil {
		return vm.Nil(), err
	}
	if err := validateModuleName(e.Root, file, string(b)); err != nil {
		return vm.Nil(), err
	}
	moduleSource := e.Sources.Add(file, string(b))
	_, ch, ds := compileFile(moduleSource, nil, false)
	if len(ds) > 0 {
		return vm.Nil(), errors.New(formatDiagnostics(ds))
	}
	ch.Name = file
	mod := vm.Value{Kind: vm.ModuleKind, Data: &vm.ModuleObject{Path: file, Exports: map[string]vm.Value{}}}
	env := vm.NewEnv(e.VM.Globals(), mod.Data.(*vm.ModuleObject))
	if _, re := e.VM.Execute(ch, env); re != nil {
		return vm.Nil(), errors.New(formatRuntime(re))
	}
	return mod, nil
}

func validateModuleName(root, file, text string) error {
	declared := ""
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "模块 ") {
			declared = strings.TrimSpace(strings.TrimPrefix(line, "模块 "))
			break
		}
	}
	if declared == "" || strings.HasPrefix(declared, "标准.") {
		return nil
	}
	rel, err := filepath.Rel(root, file)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("模块文件不在项目根目录内：%s", file)
	}
	rel = strings.TrimSuffix(rel, filepath.Ext(rel))
	expected := strings.ReplaceAll(rel, string(filepath.Separator), ".")
	if declared != expected {
		return fmt.Errorf("模块名不匹配：声明为“%s”，文件应声明为“%s”", declared, expected)
	}
	return nil
}
func (e *Engine) resolve(path, from string) (string, error) {
	if path == "" || filepath.IsAbs(path) || strings.ContainsRune(path, '\x00') {
		return "", fmt.Errorf("模块路径非法：%q", path)
	}
	parts := strings.Split(path, ".")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, `/\\`) {
			return "", fmt.Errorf("模块路径非法：%q", path)
		}
	}
	rel := filepath.Join(parts...)
	candidates := []string{filepath.Join(e.Root, rel+".hua"), filepath.Join(e.Root, rel+".华"), filepath.Join(e.Root, rel, "主.hua")}
	if from != "" {
		dir := filepath.Dir(from)
		candidates = append(candidates, filepath.Join(dir, rel+".hua"), filepath.Join(dir, rel, "主.hua"))
		first := parts[0]
		for d := dir; d != filepath.Dir(d); d = filepath.Dir(d) {
			if filepath.Base(d) == first && len(parts) > 1 {
				rest := filepath.Join(parts[1:]...)
				candidates = append(candidates, filepath.Join(d, rest+".hua"), filepath.Join(d, rest, "主.hua"))
			}
		}
	}
	root, err := filepath.EvalSymlinks(e.Root)
	if err != nil {
		root = filepath.Clean(e.Root)
	}
	for _, p := range candidates {
		if s, err := os.Stat(p); err == nil && !s.IsDir() {
			clean, err := filepath.EvalSymlinks(p)
			if err != nil {
				continue
			}
			rel, err := filepath.Rel(root, clean)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return "", fmt.Errorf("模块“%s”位于项目根目录之外", path)
			}
			return clean, nil
		}
	}
	return "", fmt.Errorf("找不到模块“%s”，搜索路径以 %s 为根", path, e.Root)
}
func formatDiagnostics(ds []diagnostic.Diagnostic) string {
	var b strings.Builder
	for i, d := range ds {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(d.Format())
	}
	return b.String()
}
func formatRuntime(e *vm.RuntimeError) string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	if e.Value.Kind == vm.ErrorKind {
		obj := e.Value.Data.(*vm.ErrorObject)
		fmt.Fprintf(&b, "运行时错误：%s：%s", obj.Category, obj.Message)
		if obj.Span.Valid() {
			line, col := obj.Span.File.LineColumn(obj.Span.Start)
			fmt.Fprintf(&b, "\n  --> %s:%d:%d", obj.Span.File.Name, line, col)
		}
		if len(obj.Stack) > 0 {
			b.WriteString("\n\n华言调用栈：")
			for i := len(obj.Stack) - 1; i >= 0; i-- {
				b.WriteString("\n  " + obj.Stack[i].Name)
			}
		}
	} else {
		b.WriteString(e.Error())
	}
	return b.String()
}
func FormatDiagnostics(ds []diagnostic.Diagnostic) string { return formatDiagnostics(ds) }
func FormatRuntime(e *vm.RuntimeError) string             { return formatRuntime(e) }

func RuntimeExitCode(e *vm.RuntimeError) (int, bool) {
	if e == nil || e.Value.Kind != vm.ErrorKind {
		return 0, false
	}
	obj, ok := e.Value.Data.(*vm.ErrorObject)
	if !ok || obj.ExitCode == nil {
		return 0, false
	}
	return int(*obj.ExitCode), true
}
