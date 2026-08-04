package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"huayan/internal/engine"
	"huayan/internal/formatter"
)

const version = "0.3.0"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("华言 " + version)
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "check" || os.Args[1] == "检查") {
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "用法：huayan check 文件.hua")
			os.Exit(64)
		}
		e := engine.New("", os.Stdout, os.Stdin, nil)
		if _, ds := e.CheckFile(os.Args[2]); len(ds) > 0 {
			fmt.Fprintln(os.Stderr, engine.FormatDiagnostics(ds))
			os.Exit(2)
		}
		fmt.Println("检查通过：" + os.Args[2])
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "dis" || os.Args[1] == "字节码") {
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "用法：huayan dis 文件.hua")
			os.Exit(64)
		}
		e := engine.New("", os.Stdout, os.Stdin, nil)
		ch, ds := e.CheckFile(os.Args[2])
		if len(ds) > 0 {
			fmt.Fprintln(os.Stderr, engine.FormatDiagnostics(ds))
			os.Exit(2)
		}
		fmt.Print(ch.Disassemble())
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "fmt" || os.Args[1] == "格式化") {
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "用法：huayan fmt 文件.hua")
			os.Exit(64)
		}
		check := os.Args[2] == "--check"
		fileArg := 2
		if check {
			fileArg++
			if len(os.Args) <= fileArg {
				fmt.Fprintln(os.Stderr, "用法：huayan fmt [--check] 文件.hua")
				os.Exit(64)
			}
		}
		b, e := os.ReadFile(os.Args[fileArg])
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(2)
		}
		formatted := formatter.Format(string(b))
		if check {
			if formatted != string(b) {
				fmt.Fprintln(os.Stderr, "需要格式化："+os.Args[fileArg])
				os.Exit(1)
			}
			return
		}
		fmt.Print(formatted)
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "test" || os.Args[1] == "测试") {
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "用法：huayan test 目录")
			os.Exit(64)
		}
		runTests(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "-c" {
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "用法：huayan -c '打印(\"你好\")'")
			os.Exit(64)
		}
		runSource(os.Args[2], "命令行代码", os.Args[3:])
		return
	}
	if len(os.Args) == 1 {
		repl()
		return
	}
	path := os.Args[1]
	args := []string{}
	if i := indexOf(os.Args, "--"); i >= 0 {
		args = os.Args[i+1:]
	}
	e := engine.New("", os.Stdout, os.Stdin, args)
	_, re, ds := e.RunFile(path)
	if len(ds) > 0 {
		fmt.Fprintln(os.Stderr, engine.FormatDiagnostics(ds))
		os.Exit(2)
	}
	if re != nil {
		fmt.Fprintln(os.Stderr, engine.FormatRuntime(re))
		if code, ok := engine.RuntimeExitCode(re); ok {
			os.Exit(code)
		}
		os.Exit(1)
	}
}

func runTests(paths []string) {
	e := engine.New("", os.Stdout, os.Stdin, nil)
	count := 0
	for _, root := range paths {
		files := []string{}
		if info, err := os.Stat(root); err == nil && !info.IsDir() {
			files = append(files, root)
		} else {
			_ = walkHua(root, &files)
		}
		for _, file := range files {
			count++
			if _, re, ds := e.RunFile(file); len(ds) > 0 {
				fmt.Fprintln(os.Stderr, engine.FormatDiagnostics(ds))
				os.Exit(2)
			} else if re != nil {
				fmt.Fprintln(os.Stderr, engine.FormatRuntime(re))
				os.Exit(1)
			}
		}
	}
	if count == 0 {
		fmt.Fprintln(os.Stderr, "没有找到 .hua 测试文件")
		os.Exit(64)
	}
	fmt.Printf("测试通过：%d 个文件\n", count)
}

func walkHua(root string, files *[]string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			if err := walkHua(path, files); err != nil {
				return err
			}
		} else if filepath.Ext(path) == ".hua" || filepath.Ext(path) == ".华" {
			*files = append(*files, path)
		}
	}
	return nil
}
func indexOf(a []string, w string) int {
	for i, x := range a {
		if x == w {
			return i
		}
	}
	return -1
}
func runSource(text, name string, args []string) {
	_, ch, ds := engine.Compile(name, text)
	if len(ds) > 0 {
		fmt.Fprintln(os.Stderr, engine.FormatDiagnostics(ds))
		os.Exit(2)
	}
	e := engine.New("", os.Stdout, os.Stdin, args)
	_, re := e.VM.Execute(ch, e.VM.Globals())
	if re != nil {
		fmt.Fprintln(os.Stderr, engine.FormatRuntime(re))
		if code, ok := engine.RuntimeExitCode(re); ok {
			os.Exit(code)
		}
		os.Exit(1)
	}
}
func repl() {
	fmt.Println("华言 " + version)
	fmt.Println("输入“退出”离开")
	e := engine.New("", os.Stdout, os.Stdin, nil)
	in := bufio.NewScanner(os.Stdin)
	pending := ""
	depth := 0
	for {
		if depth > 0 {
			fmt.Print("... ")
		} else {
			fmt.Print(">>> ")
		}
		if !in.Scan() {
			return
		}
		line := in.Text()
		if pending == "" && strings.TrimSpace(line) == "退出" {
			return
		}
		if pending == "" && strings.TrimSpace(line) == "" {
			continue
		}
		pending += line + "\n"
		depth += replBlockDelta(line)
		if depth > 0 || replDelimiterDepth(pending) > 0 {
			continue
		}
		_, ch, ds := engine.CompileREPL("<交互>", pending, e.VM.Globals().Names())
		pending = ""
		if len(ds) > 0 {
			fmt.Fprintln(os.Stderr, engine.FormatDiagnostics(ds))
			continue
		}
		v, re := e.VM.Execute(ch, e.VM.Globals())
		if re != nil {
			fmt.Fprintln(os.Stderr, engine.FormatRuntime(re))
			continue
		}
		if v.Kind != "空" {
			fmt.Println(v.String())
		}
	}
}

// replDelimiterDepth counts open delimiters without treating text inside a
// string or comment as syntax. This lets users enter a multi-line collection
// or call expression even when no block keyword is present.
func replDelimiterDepth(text string) int {
	depth := 0
	inString, escaped, lineComment, blockComment := false, false, false, false
	previous := rune(0)
	for _, c := range text {
		if lineComment {
			if c == '\n' {
				lineComment = false
			}
			previous = c
			continue
		}
		if blockComment {
			if previous == '*' && c == '/' {
				blockComment = false
			}
			previous = c
			continue
		}
		if inString {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			previous = c
			continue
		}
		if c == '"' {
			inString = true
			previous = c
			continue
		}
		if previous == '/' && c == '/' {
			lineComment = true
			previous = c
			continue
		}
		if previous == '/' && c == '*' {
			blockComment = true
			previous = c
			continue
		}
		switch c {
		case '(', '[', '{', '（', '［', '｛':
			depth++
		case ')', ']', '}', '）', '］', '｝':
			if depth > 0 {
				depth--
			}
		}
		previous = c
	}
	return depth
}

func replBlockDelta(line string) int {
	word := strings.TrimSpace(line)
	if word == "" {
		return 0
	}
	first := word
	if i := strings.IndexAny(first, " \t（("); i >= 0 {
		first = first[:i]
	}
	switch first {
	case "函数", "如果", "当", "遍历", "尝试":
		return 1
	case "结束":
		return -1
	default:
		return 0
	}
}
