package parser

import (
	"strings"
	"testing"

	"huayan/internal/lexer"
	"huayan/internal/source"
)

func TestParserRecoveryAndExpressionForms(t *testing.T) {
	cases := []string{
		"模块\n导入\n从 模块 导入\n让 =\n函数 f(\n返回\n结束",
		"如果 真\n否则如果 假\n否则\n结束\n当 真\n结束\n遍历 x 于 []\n结束",
		"尝试\n抛出 错误(\"x\")\n捕获 原因\n打印(原因.消息)\n结束",
		"尝试\n打印(\"工作\")\n最后\n打印(\"清理\")\n结束",
		"尝试\n抛出 错误(\"x\")\n捕获 原因\n打印(原因.消息)\n最后\n打印(\"清理\")\n结束",
		"记录 {名: 1, 嵌套: [真, 空]}[\"名\"] = 2\n",
	}
	for _, text := range cases {
		file := &source.File{Name: "recover.hua", Text: text}
		ts, _ := lexer.Lex(file)
		_, _ = Parse(ts)
	}
	valid := "从 标准.JSON 导入 解析\n让 x = (1 + 2) * 3\n打印(解析(\"null\"))"
	ts, _ := lexer.Lex(&source.File{Name: "valid.hua", Text: valid})
	if _, ds := Parse(ts); len(ds) != 0 || !strings.Contains(valid, "解析") {
		t.Fatalf("valid parse diagnostics=%v", ds)
	}
	query := "让 结果 = 选择 图书 从 库\n其中 编号 等于 \"B001\"\n排序 书名 降序\n限制 5\n结束"
	qt, _ := lexer.Lex(&source.File{Name: "query.hua", Text: query})
	if program, ds := Parse(qt); len(ds) != 0 || len(program.Statements) != 1 {
		t.Fatalf("query parse diagnostics=%v program=%#v", ds, program)
	}
}

func FuzzParserNeverPanics(f *testing.F) {
	f.Add([]byte("如果 真\n打印(1)\n结束"))
	f.Add([]byte("函数 f(x)\n返回 x + 1\n结束"))
	f.Fuzz(func(t *testing.T, data []byte) {
		file := &source.File{Name: "fuzz.hua", Text: string(data)}
		tokens, _ := lexer.Lex(file)
		Parse(tokens)
	})
}
