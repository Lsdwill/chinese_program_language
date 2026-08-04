package compiler

import (
	"huayan/internal/ast"
	"huayan/internal/lexer"
	"huayan/internal/parser"
	"huayan/internal/source"
	"testing"
)

func compileTest(t *testing.T, text string) *ast.Program {
	t.Helper()
	f := &source.File{Name: "compiler.hua", Text: text}
	ts, le := lexer.Lex(f)
	if len(le) != 0 {
		t.Fatal(le)
	}
	p, pe := parser.Parse(ts)
	if len(pe) != 0 {
		t.Fatal(pe)
	}
	ch, ce := Compile(p)
	if len(ce) != 0 {
		t.Fatal(ce)
	}
	if err := ch.Validate(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCompilerAllStatementForms(t *testing.T) {
	compileTest(t, `
模块 示例.主
公开 常量 版本 = 1
让 值 = [1, 2]
函数 运行(参数)
    让 局部 = 参数
    如果 局部 > 0
        返回 局部
    否则如果 局部 == 0
        返回 0
    否则
        返回 -局部
    结束
结束
当 假
    跳出
结束
遍历 项 于 值
    继续
结束
尝试
    抛出 错误("x")
捕获 原因
    打印(原因)
最后
    打印("清理")
结束
运行(1)
`)
}

func TestCompilerRejectsControlTransferAcrossFinally(t *testing.T) {
	f := &source.File{Name: "finally.hua", Text: "函数 f()\n尝试\n返回 1\n最后\n打印(1)\n结束\n结束"}
	ts, _ := lexer.Lex(f)
	p, _ := parser.Parse(ts)
	_, ds := Compile(p)
	if len(ds) == 0 {
		t.Fatal("return inside try/finally was accepted")
	}
	found := false
	for _, d := range ds {
		if d.Code == "E2010" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics=%v", ds)
	}
}

func TestCompilerResolverDiagnostics(t *testing.T) {
	f := &source.File{Name: "bad.hua", Text: "打印(未声明)\n返回 1\n跳出"}
	ts, _ := lexer.Lex(f)
	p, _ := parser.Parse(ts)
	_, ds := Compile(p)
	if len(ds) < 3 {
		t.Fatalf("diagnostics=%v", ds)
	}
	if _, ds = CompileREPL(p, map[string]bool{"未声明": true}); len(ds) == 0 {
		t.Fatal("REPL globals were ignored")
	}
}

func TestCompilerREPLExpression(t *testing.T) {
	f := &source.File{Name: "repl", Text: "1 + 2"}
	ts, _ := lexer.Lex(f)
	p, _ := parser.Parse(ts)
	ch, ds := CompileREPL(p, nil)
	if len(ds) != 0 || ch == nil || len(ch.Code) == 0 {
		t.Fatalf("chunk=%v diagnostics=%v", ch, ds)
	}
}

func TestCompilerAcceptsREPLGlobalsAndRejectsInvalidTargets(t *testing.T) {
	f := &source.File{Name: "repl", Text: "旧名 + 1"}
	ts, _ := lexer.Lex(f)
	p, _ := parser.Parse(ts)
	if _, ds := CompileREPL(p, map[string]bool{"旧名": true}); len(ds) != 0 {
		t.Fatalf("REPL global rejected: %v", ds)
	}
	f = &source.File{Name: "target", Text: "1 = 2"}
	ts, _ = lexer.Lex(f)
	p, _ = parser.Parse(ts)
	if _, ds := Compile(p); len(ds) == 0 {
		t.Fatal("invalid assignment target accepted")
	}
}

func TestResolverDiagnosticMatrix(t *testing.T) {
	cases := []struct {
		name, text, code string
	}{
		{"重复声明", "让 名字 = 1\n让 名字 = 2", "E2002"},
		{"声明期间读取自身", "让 名字 = 名字", "E2007"},
		{"未声明赋值", "未声明 = 1", "E2001"},
		{"非法赋值目标", "1 = 2", "E2006"},
		{"函数参数重复", "函数 计算(x, x)\n返回 x\n结束", "E2002"},
		{"顶层返回", "返回 1", "E2003"},
		{"循环外跳出", "跳出", "E2004"},
		{"循环外继续", "继续", "E2005"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &source.File{Name: "resolver.hua", Text: tc.text}
			ts, le := lexer.Lex(f)
			if len(le) != 0 {
				t.Fatal(le)
			}
			p, pe := parser.Parse(ts)
			if len(pe) != 0 {
				t.Fatal(pe)
			}
			_, ds := Compile(p)
			found := false
			for _, d := range ds {
				if d.Code == tc.code {
					found = true
					if d.Span.File == nil || !d.Span.Valid() {
						t.Fatalf("diagnostic has no source position: %#v", d)
					}
				}
			}
			if !found {
				t.Fatalf("missing %s in %#v", tc.code, ds)
			}
		})
	}
}

func TestCompilerAllExpressionForms(t *testing.T) {
	compileTest(t, `
	让 库 = 空
	让 查询结果 = 选择 图书 从 库
	    其中 编号 等于 "B001"
	    排序 书名 升序
	    限制 5
	结束
让 空值 = 空
让 真值 = 真
让 假值 = 假
让 文字 = "华言"
让 整数 = 1
让 小数 = 1.5
让 列表 = [空值, 真值, 文字, 整数, 小数]
让 字典 = {"甲": 整数}
让 记录值 = 记录 {名: 文字}
让 逻辑且 = 真 且 假
让 逻辑或 = 假 或 真
让 负数 = -整数
让 取值 = 列表[0]
让 字段 = 记录值.名
列表[0] = 空
字典["乙"] = 2
记录值.名 = "新名"
函数 调用(x, y)
    返回 x + y
结束
让 结果 = 调用(整数, 小数)
`)
}

func TestCompilerHelperBoundaries(t *testing.T) {
	f := &source.File{Name: "globals.hua", Text: "旧名 + 1"}
	ts, _ := lexer.Lex(f)
	p, _ := parser.Parse(ts)
	if ch, ds := CompileWithGlobals(p, map[string]bool{"旧名": true}); len(ds) != 0 || ch == nil {
		t.Fatalf("globals compile failed: %#v %v", ch, ds)
	}
	c := &Compiler{}
	s := &scope{names: map[string]binding{"已有": {initialized: true}}}
	if !c.lookup(s, "已有") || c.lookup(s, "没有") {
		t.Fatal("scope lookup failed")
	}
	c.declare(s, "新名", source.Span{})
	c.declare(s, "新名", source.Span{})
	if len(c.errors) != 1 {
		t.Fatalf("duplicate helper declaration errors=%v", c.errors)
	}
	if c.upvalueIndex("值") != 0 || c.upvalueIndex("值") != 0 || c.upvalueIndex("另一个") != 1 {
		t.Fatalf("upvalue index allocation failed: %#v", c.upvalues)
	}
	if suggest("打印") == "" || suggest("完全不同") == "" {
		t.Fatal("suggestion was empty")
	}
	if !spanOf(nil).Valid() && spanOf(nil).File != nil {
		t.Fatal("empty body span unexpectedly points to a file")
	}
}
