package ast_test

import (
	"huayan/internal/ast"
	"huayan/internal/lexer"
	"huayan/internal/parser"
	"huayan/internal/source"
	"strings"
	"testing"
)

func TestDumpStable(t *testing.T) {
	f := &source.File{Name: "golden.hua", Text: "让 数字 = 1 + 2\n打印(数字)"}
	ts, le := lexer.Lex(f)
	if len(le) != 0 {
		t.Fatal(le)
	}
	p, pe := parser.Parse(ts)
	if len(pe) != 0 {
		t.Fatal(pe)
	}
	want := "程序\n  变量 数字\n    变量值\n      二元 +\n        字面量 int 1\n        字面量 int 2\n  表达式\n    调用\n      名称 打印\n      名称 数字\n"
	if got := ast.Dump(p); got != want {
		t.Fatalf("dump mismatch:\n%s", strings.ReplaceAll(got, "\n", "\\n\n"))
	}
}

func TestDumpCoversAllLanguageNodeShapes(t *testing.T) {
	if ast.Dump(nil) != "<空程序>\n" {
		t.Fatal("nil program dump changed")
	}
	f := &source.File{Name: "all.hua", Text: `模块 示例.主
导入 标准.文字 为 文字
公开 常量 版本 = 1
让 值 = [1, 2]
让 字典 = {"甲": 1}
让 记录值 = 记录 {名: "华言"}
函数 运行(x)
    如果 x > 0
        返回 x
    否则
        返回 -x
    结束
结束
当 真
    跳出
结束
遍历 项 于 值
    继续
结束
尝试
    抛出 错误("错误")
捕获 原因
    打印(原因)
最后
    打印("清理")
结束
值[0] = 2
记录值.名 = "新名"
`}
	ts, le := lexer.Lex(f)
	if len(le) != 0 {
		t.Fatal(le)
	}
	p, pe := parser.Parse(ts)
	if len(pe) != 0 {
		t.Fatal(pe)
	}
	got := ast.Dump(p)
	for _, want := range []string{"模块=示例.主", "导入 标准.文字", "常量 版本", "函数 运行", "条件", "循环", "遍历 项", "尝试 原因", "抛出", "列表", "字典", "记录", "索引", "字段", "赋值"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dump missing %q:\n%s", want, got)
		}
	}
}
