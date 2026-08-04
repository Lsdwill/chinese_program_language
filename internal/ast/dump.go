package ast

import (
	"fmt"
	"strings"
)

// Dump returns a stable, source-independent tree representation useful for
// parser diagnostics, golden tests and small editor integrations.
func Dump(p *Program) string {
	if p == nil {
		return "<空程序>\n"
	}
	var b strings.Builder
	dumpProgram(&b, p, "")
	return b.String()
}

func line(b *strings.Builder, indent, name string, extra ...string) {
	b.WriteString(indent)
	b.WriteString(name)
	for _, x := range extra {
		b.WriteByte(' ')
		b.WriteString(x)
	}
	b.WriteByte('\n')
}
func dumpProgram(b *strings.Builder, p *Program, in string) {
	if p.Module != "" {
		line(b, in, "程序", "模块="+p.Module)
	} else {
		line(b, in, "程序")
	}
	for _, s := range p.Statements {
		dumpStmt(b, s, in+"  ")
	}
}
func dumpStmt(b *strings.Builder, s Stmt, in string) {
	switch x := s.(type) {
	case *ImportStmt:
		line(b, in, "导入", x.Path)
	case *VarDecl:
		if x.Constant {
			line(b, in, "常量", x.Name)
		} else {
			line(b, in, "变量", x.Name)
		}
		if !x.Constant {
			b.WriteString(in + "  变量值\n")
		} else {
			b.WriteString(in + "  常量值\n")
		}
		dumpExpr(b, x.Value, in+"    ")
	case *FunctionDecl:
		line(b, in, "函数", x.Name)
		if len(x.Params) > 0 {
			line(b, in+"  ", "参数", strings.Join(x.Params, ","))
		}
		for _, v := range x.Body {
			dumpStmt(b, v, in+"  ")
		}
	case *ExprStmt:
		line(b, in, "表达式")
		dumpExpr(b, x.Expression, in+"  ")
	case *ReturnStmt:
		line(b, in, "返回")
		if x.Value != nil {
			dumpExpr(b, x.Value, in+"  ")
		}
	case *ThrowStmt:
		line(b, in, "抛出")
		dumpExpr(b, x.Value, in+"  ")
	case *BreakStmt:
		line(b, in, "跳出")
	case *ContinueStmt:
		line(b, in, "继续")
	case *IfStmt:
		line(b, in, "条件")
		for _, br := range x.Branches {
			line(b, in+"  ", "分支")
			dumpExpr(b, br.Condition, in+"    ")
			for _, v := range br.Body {
				dumpStmt(b, v, in+"    ")
			}
		}
		if x.Else != nil {
			line(b, in+"  ", "否则")
			for _, v := range x.Else {
				dumpStmt(b, v, in+"    ")
			}
		}
	case *WhileStmt:
		line(b, in, "循环")
		dumpExpr(b, x.Condition, in+"  ")
		for _, v := range x.Body {
			dumpStmt(b, v, in+"  ")
		}
	case *ForStmt:
		line(b, in, "遍历", x.Name)
		dumpExpr(b, x.Iterable, in+"  ")
		for _, v := range x.Body {
			dumpStmt(b, v, in+"  ")
		}
	case *TryStmt:
		line(b, in, "尝试", x.CatchName)
		for _, v := range x.Body {
			dumpStmt(b, v, in+"  ")
		}
		if x.CatchName != "" {
			line(b, in, "捕获")
			for _, v := range x.CatchBody {
				dumpStmt(b, v, in+"  ")
			}
		}
		if x.HasFinally {
			line(b, in, "最后")
			for _, v := range x.Finally {
				dumpStmt(b, v, in+"  ")
			}
		}
	default:
		line(b, in, fmt.Sprintf("<未知语句 %T>", s))
	}
}
func dumpExpr(b *strings.Builder, e Expr, in string) {
	if e == nil {
		line(b, in, "<空>")
		return
	}
	switch x := e.(type) {
	case *Literal:
		line(b, in, "字面量", x.Kind, fmt.Sprint(x.Value))
	case *Name:
		line(b, in, "名称", x.Value)
	case *Unary:
		line(b, in, "一元", x.Operator)
		dumpExpr(b, x.Right, in+"  ")
	case *Binary:
		line(b, in, "二元", x.Operator)
		dumpExpr(b, x.Left, in+"  ")
		dumpExpr(b, x.Right, in+"  ")
	case *Assign:
		line(b, in, "赋值")
		dumpExpr(b, x.Target, in+"  ")
		dumpExpr(b, x.Value, in+"  ")
	case *Call:
		line(b, in, "调用")
		dumpExpr(b, x.Callee, in+"  ")
		for _, a := range x.Args {
			dumpExpr(b, a, in+"  ")
		}
	case *Index:
		line(b, in, "索引")
		dumpExpr(b, x.Object, in+"  ")
		dumpExpr(b, x.Key, in+"  ")
	case *Field:
		line(b, in, "字段", x.Name)
		dumpExpr(b, x.Object, in+"  ")
	case *List:
		line(b, in, "列表")
		for _, a := range x.Items {
			dumpExpr(b, a, in+"  ")
		}
	case *Dict:
		if x.Record {
			line(b, in, "记录")
		} else {
			line(b, in, "字典")
		}
		for _, p := range x.Pairs {
			dumpExpr(b, p.Key, in+"  ")
			dumpExpr(b, p.Value, in+"  ")
		}
	case *Query:
		line(b, in, "查询", x.Table)
		dumpExpr(b, x.Database, in+"  ")
		if x.WhereField != "" {
			line(b, in+"  ", "其中", x.WhereField)
			dumpExpr(b, x.WhereValue, in+"    ")
		}
		if x.OrderField != "" {
			line(b, in+"  ", "排序", x.OrderField)
		}
		if x.Limit != nil {
			line(b, in+"  ", "限制")
			dumpExpr(b, x.Limit, in+"    ")
		}
	default:
		line(b, in, fmt.Sprintf("<未知表达式 %T>", e))
	}
}
