package compiler

import (
	"fmt"

	"huayan/internal/ast"
	"huayan/internal/bytecode"
	"huayan/internal/diagnostic"
	"huayan/internal/source"
)

type binding struct {
	constant      bool
	initialized   bool
	slot          int
	functionDepth int
}
type scope struct {
	names         map[string]binding
	parent        *scope
	nextSlot      *int
	functionDepth int
}

var builtins = map[string]bool{"打印": true, "输入": true, "长度": true, "类型": true, "转文字": true, "转整数": true, "转小数": true, "范围": true, "错误": true, "断言": true}

func Compile(program *ast.Program) (*bytecode.Chunk, []diagnostic.Diagnostic) {
	return compile(program, nil, false)
}

// CompileWithGlobals is used by the REPL. Names already defined by an
// earlier interactive submission are visible to the resolver, while normal
// source files remain strictly declaration-checked.
func CompileWithGlobals(program *ast.Program, globals map[string]bool) (*bytecode.Chunk, []diagnostic.Diagnostic) {
	return compile(program, globals, false)
}

// CompileREPL keeps the value of a final expression so the interactive shell
// can display `1 + 2` just like the language design promises.
func CompileREPL(program *ast.Program, globals map[string]bool) (*bytecode.Chunk, []diagnostic.Diagnostic) {
	return compile(program, globals, true)
}

func compile(program *ast.Program, globals map[string]bool, repl bool) (*bytecode.Chunk, []diagnostic.Diagnostic) {
	c := &Compiler{errors: []diagnostic.Diagnostic{}, repl: repl}
	root := &scope{names: map[string]binding{}, functionDepth: 0}
	for n := range builtins {
		root.names[n] = binding{initialized: true, slot: -1, functionDepth: 0}
	}
	for n := range globals {
		root.names[n] = binding{initialized: true, slot: -1, functionDepth: 0}
	}
	c.resolveStatements(program.Statements, root, 0, 0)
	if len(c.errors) > 0 {
		return nil, c.errors
	}
	chunk := c.compileChunk(program.Statements, program.Module, program.Span.File)
	return chunk, c.errors
}

type Compiler struct {
	errors         []diagnostic.Diagnostic
	loopStack      []loopInfo
	runtimeDepth   int
	repl           bool
	slots          int
	upvalueIndexes map[string]int
	upvalues       []string
	finallyStack   []finallyContext
	finallyLimit   int
}
type loopInfo struct {
	continueTarget int
	baseDepth      int
	breaks         []int
}
type finallyContext struct {
	body []ast.Stmt
	span source.Span
}

func (c *Compiler) resolveStatements(stmts []ast.Stmt, s *scope, fnDepth, loopDepth int) {
	for _, st := range stmts {
		switch x := st.(type) {
		case *ast.VarDecl:
			if _, ok := s.names[x.Name]; ok {
				c.err(x.GetSpan(), "E2002", fmt.Sprintf("名称“%s”在此作用域中重复声明", x.Name), "")
			} else {
				b := binding{constant: x.Constant, slot: -1, functionDepth: s.functionDepth}
				if s.functionDepth > 0 {
					b.slot = *s.nextSlot
					*s.nextSlot++
					x.ResolvedSlot, x.ResolvedLocal = b.slot, true
				}
				s.names[x.Name] = b
			}
		case *ast.FunctionDecl:
			if _, ok := s.names[x.Name]; ok {
				c.err(x.GetSpan(), "E2002", fmt.Sprintf("名称“%s”在此作用域中重复声明", x.Name), "")
			} else {
				b := binding{initialized: true, slot: -1, functionDepth: s.functionDepth}
				if s.functionDepth > 0 {
					b.slot = *s.nextSlot
					*s.nextSlot++
					x.ResolvedSlot, x.ResolvedLocal = b.slot, true
				}
				s.names[x.Name] = b
			}
		case *ast.ImportStmt:
			if len(x.Names) > 0 {
				for _, n := range x.Names {
					c.declare(s, n, x.GetSpan())
				}
			} else {
				c.declare(s, x.Alias, x.GetSpan())
			}
		}
	}
	for _, st := range stmts {
		c.resolveStmt(st, s, fnDepth, loopDepth)
	}
}
func (c *Compiler) declare(s *scope, n string, sp source.Span) {
	if n == "" {
		return
	}
	if _, ok := s.names[n]; ok {
		c.err(sp, "E2002", fmt.Sprintf("名称“%s”在此作用域中重复声明", n), "")
	} else {
		b := binding{initialized: true, slot: -1, functionDepth: s.functionDepth}
		if s.functionDepth > 0 {
			b.slot = *s.nextSlot
			*s.nextSlot++
		}
		s.names[n] = b
	}
}
func (c *Compiler) resolveStmt(st ast.Stmt, s *scope, fnDepth, loopDepth int) {
	switch x := st.(type) {
	case *ast.VarDecl:
		c.resolveExpr(x.Value, s)
		b := s.names[x.Name]
		b.initialized = true
		s.names[x.Name] = b
	case *ast.FunctionDecl:
		nextSlot := 0
		child := &scope{names: map[string]binding{}, parent: s, nextSlot: &nextSlot, functionDepth: fnDepth + 1}
		for _, n := range x.Params {
			if _, ok := child.names[n]; ok {
				c.err(x.GetSpan(), "E2002", fmt.Sprintf("参数“%s”重复", n), "")
			} else {
				child.names[n] = binding{initialized: true, slot: nextSlot, functionDepth: fnDepth + 1}
				nextSlot++
			}
		}
		x.LocalSlots = nextSlot
		c.resolveStatements(x.Body, child, fnDepth+1, loopDepth)
		x.LocalSlots = nextSlot
	case *ast.ImportStmt:
	case *ast.ExprStmt:
		c.resolveExpr(x.Expression, s)
	case *ast.IfStmt:
		for _, b := range x.Branches {
			c.resolveExpr(b.Condition, s)
			child := c.childScope(s)
			c.resolveStatements(b.Body, child, fnDepth, loopDepth)
		}
		if x.Else != nil {
			child := c.childScope(s)
			c.resolveStatements(x.Else, child, fnDepth, loopDepth)
		}
	case *ast.WhileStmt:
		c.resolveExpr(x.Condition, s)
		child := c.childScope(s)
		c.resolveStatements(x.Body, child, fnDepth, loopDepth+1)
	case *ast.ForStmt:
		c.resolveExpr(x.Iterable, s)
		child := c.childScope(s)
		b := binding{initialized: true, slot: -1, functionDepth: s.functionDepth}
		if s.functionDepth > 0 {
			b.slot = *s.nextSlot
			*s.nextSlot++
		}
		child.names[x.Name] = b
		if b.slot >= 0 {
			x.ResolvedSlot, x.ResolvedLocal = b.slot, true
		}
		c.resolveStatements(x.Body, child, fnDepth, loopDepth+1)
	case *ast.TryStmt:
		child := c.childScope(s)
		c.resolveStatements(x.Body, child, fnDepth, loopDepth)
		if x.HasFinally {
			c.rejectFinallyControl(x.Body)
		}
		if x.CatchName != "" {
			catch := c.childScope(s)
			b := binding{initialized: true, slot: -1, functionDepth: s.functionDepth}
			if s.functionDepth > 0 {
				b.slot = *s.nextSlot
				*s.nextSlot++
			}
			catch.names[x.CatchName] = b
			if b.slot >= 0 {
				x.ResolvedSlot, x.ResolvedLocal = b.slot, true
			}
			c.resolveStatements(x.CatchBody, catch, fnDepth, loopDepth)
			if x.HasFinally {
				c.rejectFinallyControl(x.CatchBody)
			}
		}
		if x.HasFinally {
			c.resolveStatements(x.Finally, s, fnDepth, loopDepth)
			c.rejectFinallyControl(x.Finally)
		}
	case *ast.ReturnStmt:
		if fnDepth == 0 {
			c.err(x.GetSpan(), "E2003", "返回只能出现在函数内", "")
		}
		if x.Value != nil {
			c.resolveExpr(x.Value, s)
		}
	case *ast.BreakStmt:
		if loopDepth == 0 {
			c.err(x.GetSpan(), "E2004", "跳出只能出现在循环内", "")
		}
	case *ast.ContinueStmt:
		if loopDepth == 0 {
			c.err(x.GetSpan(), "E2005", "继续只能出现在循环内", "")
		}
	case *ast.ThrowStmt:
		c.resolveExpr(x.Value, s)
	}
}
func (c *Compiler) rejectFinallyControl(stmts []ast.Stmt) {
	for _, st := range stmts {
		switch x := st.(type) {
		case *ast.ReturnStmt:
			c.err(x.GetSpan(), "E2010", "包含‘最后’的尝试语句中暂不允许返回", "请把返回值保存后，在尝试语句之后返回")
		case *ast.BreakStmt:
			c.err(x.GetSpan(), "E2011", "包含‘最后’的尝试语句中暂不允许跳出", "")
		case *ast.ContinueStmt:
			c.err(x.GetSpan(), "E2012", "包含‘最后’的尝试语句中暂不允许继续", "")
		case *ast.FunctionDecl:
			// The function runs later, so its control flow is unrelated to the
			// surrounding try/finally statement.
		case *ast.IfStmt:
			for _, branch := range x.Branches {
				c.rejectFinallyControl(branch.Body)
			}
			c.rejectFinallyControl(x.Else)
		case *ast.WhileStmt:
			c.rejectFinallyControl(x.Body)
		case *ast.ForStmt:
			c.rejectFinallyControl(x.Body)
		case *ast.TryStmt:
			c.rejectFinallyControl(x.Body)
			c.rejectFinallyControl(x.CatchBody)
			c.rejectFinallyControl(x.Finally)
		}
	}
}
func (c *Compiler) childScope(s *scope) *scope {
	return &scope{names: map[string]binding{}, parent: s, nextSlot: s.nextSlot, functionDepth: s.functionDepth}
}
func (c *Compiler) resolveExpr(e ast.Expr, s *scope) {
	if e == nil {
		return
	}
	switch x := e.(type) {
	case *ast.Name:
		if b, ok := c.bindingFor(s, x.Value); !ok {
			c.err(x.GetSpan(), "E2001", fmt.Sprintf("变量“%s”尚未声明", x.Value), suggest(x.Value))
		} else if !b.initialized {
			c.err(x.GetSpan(), "E2007", fmt.Sprintf("变量“%s”尚未完成初始化", x.Value), "先完成声明，再读取这个变量")
		} else if b.slot >= 0 {
			x.ResolvedSlot = b.slot
			if b.functionDepth == s.functionDepth {
				x.ResolvedKind = ast.NameLocal
			} else {
				x.ResolvedKind = ast.NameUpvalue
			}
		} else {
			x.ResolvedKind = ast.NameGlobal
		}
	case *ast.Literal:
	case *ast.Unary:
		c.resolveExpr(x.Right, s)
	case *ast.Binary:
		c.resolveExpr(x.Left, s)
		c.resolveExpr(x.Right, s)
	case *ast.Assign:
		c.resolveTarget(x.Target, s)
		c.resolveExpr(x.Value, s)
	case *ast.Call:
		c.resolveExpr(x.Callee, s)
		for _, a := range x.Args {
			c.resolveExpr(a, s)
		}
	case *ast.Index:
		c.resolveExpr(x.Object, s)
		c.resolveExpr(x.Key, s)
	case *ast.Field:
		c.resolveExpr(x.Object, s)
	case *ast.List:
		for _, i := range x.Items {
			c.resolveExpr(i, s)
		}
	case *ast.Dict:
		for _, p := range x.Pairs {
			c.resolveExpr(p.Key, s)
			c.resolveExpr(p.Value, s)
		}
	}
}
func (c *Compiler) resolveTarget(e ast.Expr, s *scope) {
	switch x := e.(type) {
	case *ast.Name:
		b, ok := c.bindingFor(s, x.Value)
		if !ok {
			c.err(x.GetSpan(), "E2001", fmt.Sprintf("变量“%s”尚未声明", x.Value), "")
		} else if b.slot >= 0 {
			x.ResolvedSlot = b.slot
			if b.functionDepth == s.functionDepth {
				x.ResolvedKind = ast.NameLocal
			} else {
				x.ResolvedKind = ast.NameUpvalue
			}
		} else {
			x.ResolvedKind = ast.NameGlobal
		}
	case *ast.Index:
		c.resolveExpr(x.Object, s)
		c.resolveExpr(x.Key, s)
	case *ast.Field:
		c.resolveExpr(x.Object, s)
	default:
		c.err(e.GetSpan(), "E2006", "赋值左侧必须是变量、索引或记录字段", "")
	}
}
func (c *Compiler) lookup(s *scope, n string) bool {
	_, ok := c.bindingFor(s, n)
	return ok
}
func (c *Compiler) bindingFor(s *scope, n string) (binding, bool) {
	for s != nil {
		if b, ok := s.names[n]; ok {
			return b, true
		}
		s = s.parent
	}
	return binding{}, false
}
func suggest(n string) string {
	for b := range builtins {
		if len([]rune(b)) == len([]rune(n)) {
			return "可以检查名称是否拼写正确"
		}
	}
	return "先使用‘让’或‘常量’声明名称"
}
func (c *Compiler) err(sp source.Span, code, msg, hint string) {
	c.errors = append(c.errors, diagnostic.Diagnostic{Severity: diagnostic.Error, Code: code, Message: msg, Hint: hint, Span: sp})
}

func (c *Compiler) compileChunk(stmts []ast.Stmt, name string, file *source.File) *bytecode.Chunk {
	ch := &bytecode.Chunk{Name: name, File: file, Slots: c.slots, SlotNames: map[int]string{}}
	lastExpression := c.repl && len(stmts) > 0
	if lastExpression {
		_, lastExpression = stmts[len(stmts)-1].(*ast.ExprStmt)
	}
	for i, st := range stmts {
		if lastExpression && i == len(stmts)-1 {
			c.compileExpr(ch, st.(*ast.ExprStmt).Expression)
		} else {
			c.compileStmt(ch, st)
		}
	}
	if lastExpression {
		ch.Emit(bytecode.OpReturn, 0, "", source.Span{File: file})
		return ch
	}
	ch.Emit(bytecode.OpNil, 0, "", source.Span{File: file})
	ch.Emit(bytecode.OpReturn, 0, "", source.Span{File: file})
	return ch
}
func (c *Compiler) compileStmt(ch *bytecode.Chunk, st ast.Stmt) {
	switch x := st.(type) {
	case *ast.ImportStmt:
		if len(x.Names) == 0 {
			ch.Emit(bytecode.OpImport, ch.AddConstant(x.Path), "", x.Span)
			ch.Emit(bytecode.OpDeclareName, ch.AddConstant(x.Alias), "", x.Span)
		} else {
			for _, n := range x.Names {
				ch.Emit(bytecode.OpImport, ch.AddConstant(x.Path), "", x.Span)
				ch.Emit(bytecode.OpGetExport, ch.AddConstant(n), "", x.Span)
				ch.Emit(bytecode.OpDeclareName, ch.AddConstant(n), "", x.Span)
			}
		}
	case *ast.VarDecl:
		c.compileExpr(ch, x.Value)
		if x.ResolvedLocal {
			ch.SlotNames[x.ResolvedSlot] = x.Name
			at := ch.Emit(bytecode.OpDeclareLocal, x.ResolvedSlot, x.Name, x.Span)
			ch.Code[at].Constant = x.Constant
		} else {
			declareText := ""
			if x.Constant {
				declareText = "常量"
			}
			ch.Emit(bytecode.OpDeclareName, ch.AddConstant(x.Name), declareText, x.Span)
		}
		if x.Public {
			ch.Emit(bytecode.OpExportName, ch.AddConstant(x.Name), "", x.Span)
		}
	case *ast.FunctionDecl:
		proto := c.compileFunction(x)
		ch.Emit(bytecode.OpMakeClosure, ch.AddConstant(proto), "", x.Span)
		if x.ResolvedLocal {
			ch.SlotNames[x.ResolvedSlot] = x.Name
			ch.Emit(bytecode.OpDeclareLocal, x.ResolvedSlot, x.Name, x.Span)
		} else {
			ch.Emit(bytecode.OpDeclareName, ch.AddConstant(x.Name), "", x.Span)
		}
		if x.Public {
			ch.Emit(bytecode.OpExportName, ch.AddConstant(x.Name), "", x.Span)
		}
	case *ast.ExprStmt:
		c.compileExpr(ch, x.Expression)
		ch.Emit(bytecode.OpPop, 0, "", x.Span)
	case *ast.ReturnStmt:
		if x.Value == nil {
			ch.Emit(bytecode.OpNil, 0, "", x.Span)
		} else {
			c.compileExpr(ch, x.Value)
		}
		c.compileActiveFinally(ch)
		ch.Emit(bytecode.OpReturn, 0, "", x.Span)
	case *ast.ThrowStmt:
		c.compileExpr(ch, x.Value)
		ch.Emit(bytecode.OpThrow, 0, "", x.Span)
	case *ast.BreakStmt:
		if len(c.loopStack) > 0 {
			li := &c.loopStack[len(c.loopStack)-1]
			for n := c.runtimeDepth; n > li.baseDepth; n-- {
				ch.Emit(bytecode.OpExitScope, 0, "", x.Span)
			}
			li.breaks = append(li.breaks, ch.Emit(bytecode.OpJump, 0, "", x.Span))
		}
	case *ast.ContinueStmt:
		if len(c.loopStack) > 0 {
			li := c.loopStack[len(c.loopStack)-1]
			for n := c.runtimeDepth; n > li.baseDepth+1; n-- {
				ch.Emit(bytecode.OpExitScope, 0, "", x.Span)
			}
			ch.Emit(bytecode.OpJump, li.continueTarget, "", x.Span)
		}
	case *ast.IfStmt:
		c.compileIf(ch, x)
	case *ast.WhileStmt:
		c.compileWhile(ch, x)
	case *ast.ForStmt:
		c.compileFor(ch, x)
	case *ast.TryStmt:
		c.compileTry(ch, x)
	}
}
func (c *Compiler) compileFunction(x *ast.FunctionDecl) *bytecode.FunctionProto {
	child := &Compiler{errors: c.errors, repl: false, upvalueIndexes: map[string]int{}}
	child.slots = x.LocalSlots
	c.errors = child.errors
	ch := child.compileChunk(x.Body, x.Name, x.Span.File)
	for i, name := range x.Params {
		ch.SlotNames[i] = name
	}
	ch.Upvalues = append([]string(nil), child.upvalues...)
	c.errors = child.errors
	return &bytecode.FunctionProto{Name: x.Name, Params: x.Params, Chunk: ch, Slots: x.LocalSlots, Upvalues: append([]string(nil), child.upvalues...), Span: x.Span}
}

func (c *Compiler) compileActiveFinally(ch *bytecode.Chunk) {
	for i := c.finallyLimit - 1; i >= 0; i-- {
		c.compileFinallyAt(ch, i)
	}
}

func (c *Compiler) compileFinallyAt(ch *bytecode.Chunk, index int) {
	if index < 0 || index >= len(c.finallyStack) {
		return
	}
	oldLimit := c.finallyLimit
	c.finallyLimit = index
	c.compileBlock(ch, c.finallyStack[index].body)
	c.finallyLimit = oldLimit
}

func (c *Compiler) upvalueIndex(name string) int {
	if c.upvalueIndexes == nil {
		c.upvalueIndexes = map[string]int{}
	}
	if i, ok := c.upvalueIndexes[name]; ok {
		return i
	}
	i := len(c.upvalues)
	c.upvalueIndexes[name] = i
	c.upvalues = append(c.upvalues, name)
	return i
}
func (c *Compiler) compileBlock(ch *bytecode.Chunk, body []ast.Stmt) {
	ch.Emit(bytecode.OpEnterScope, 0, "", spanOf(body))
	c.runtimeDepth++
	for _, s := range body {
		c.compileStmt(ch, s)
	}
	c.runtimeDepth--
	ch.Emit(bytecode.OpExitScope, 0, "", spanOf(body))
}
func spanOf(body []ast.Stmt) source.Span {
	if len(body) > 0 {
		return body[0].GetSpan()
	}
	return source.Span{}
}
func (c *Compiler) compileIf(ch *bytecode.Chunk, x *ast.IfStmt) {
	var ends []int
	for _, b := range x.Branches {
		c.compileExpr(ch, b.Condition)
		j := ch.Emit(bytecode.OpJumpIfFalse, 0, "", b.Span)
		c.compileBlock(ch, b.Body)
		ends = append(ends, ch.Emit(bytecode.OpJump, 0, "", b.Span))
		ch.Patch(j, len(ch.Code))
	}
	if x.Else != nil {
		c.compileBlock(ch, x.Else)
	}
	for _, j := range ends {
		ch.Patch(j, len(ch.Code))
	}
}
func (c *Compiler) compileWhile(ch *bytecode.Chunk, x *ast.WhileStmt) {
	base := c.runtimeDepth
	ch.Emit(bytecode.OpEnterScope, 0, "", x.Span)
	c.runtimeDepth++
	start := len(ch.Code)
	c.compileExpr(ch, x.Condition)
	exit := ch.Emit(bytecode.OpJumpIfFalse, 0, "", x.Span)
	li := loopInfo{continueTarget: start, baseDepth: base}
	c.loopStack = append(c.loopStack, li)
	c.compileBlock(ch, x.Body)
	ch.Emit(bytecode.OpJump, start, "", x.Span)
	c.runtimeDepth--
	ch.Emit(bytecode.OpExitScope, 0, "", x.Span)
	end := len(ch.Code)
	ch.Patch(exit, end)
	li = c.loopStack[len(c.loopStack)-1]
	c.loopStack = c.loopStack[:len(c.loopStack)-1]
	for _, j := range li.breaks {
		ch.Patch(j, end)
	}
}
func (c *Compiler) compileFor(ch *bytecode.Chunk, x *ast.ForStmt) {
	base := c.runtimeDepth
	ch.Emit(bytecode.OpEnterScope, 0, "", x.Span)
	c.runtimeDepth++
	c.compileExpr(ch, x.Iterable)
	ch.Emit(bytecode.OpIterStart, 0, "", x.Span)
	ch.Emit(bytecode.OpNil, 0, "", x.Span)
	if x.ResolvedLocal {
		ch.SlotNames[x.ResolvedSlot] = x.Name
		ch.Emit(bytecode.OpDeclareLocal, x.ResolvedSlot, x.Name, x.Span)
	} else {
		ch.Emit(bytecode.OpDeclareName, ch.AddConstant(x.Name), "", x.Span)
	}
	start := len(ch.Code)
	exit := ch.Emit(bytecode.OpIterNext, 0, "", x.Span)
	if x.ResolvedLocal {
		ch.Emit(bytecode.OpStoreLocal, x.ResolvedSlot, x.Name, x.Span)
	} else {
		ch.Emit(bytecode.OpStoreName, ch.AddConstant(x.Name), "", x.Span)
	}
	ch.Emit(bytecode.OpPop, 0, "", x.Span)
	li := loopInfo{continueTarget: start, baseDepth: base}
	c.loopStack = append(c.loopStack, li)
	c.compileBlock(ch, x.Body)
	ch.Emit(bytecode.OpJump, start, "", x.Span)
	c.runtimeDepth--
	ch.Emit(bytecode.OpIterEnd, 0, "", x.Span)
	ch.Emit(bytecode.OpExitScope, 0, "", x.Span)
	end := len(ch.Code)
	ch.Patch(exit, end)
	li = c.loopStack[len(c.loopStack)-1]
	c.loopStack = c.loopStack[:len(c.loopStack)-1]
	for _, j := range li.breaks {
		ch.Patch(j, end)
	}
}
func (c *Compiler) compileTry(ch *bytecode.Chunk, x *ast.TryStmt) {
	ch.Emit(bytecode.OpEnterScope, 0, "", x.Span)
	c.runtimeDepth++
	if x.CatchName != "" {
		ch.Emit(bytecode.OpNil, 0, "", x.Span)
		if x.ResolvedLocal {
			ch.SlotNames[x.ResolvedSlot] = x.CatchName
			ch.Emit(bytecode.OpDeclareLocal, x.ResolvedSlot, x.CatchName, x.Span)
		} else {
			ch.Emit(bytecode.OpDeclareName, ch.AddConstant(x.CatchName), "", x.Span)
		}
	}
	finallyIndex := -1
	oldLimit := c.finallyLimit
	if x.HasFinally {
		finallyIndex = len(c.finallyStack)
		c.finallyStack = append(c.finallyStack, finallyContext{body: x.Finally, span: x.Span})
		c.finallyLimit = len(c.finallyStack)
	}
	tr := ch.Emit(bytecode.OpTry, 0, "", x.Span)
	c.compileBlock(ch, x.Body)
	ch.Emit(bytecode.OpEndTry, 0, "", x.Span)
	if x.HasFinally {
		c.compileFinallyAt(ch, finallyIndex)
	}
	jump := ch.Emit(bytecode.OpJump, 0, "", x.Span)
	handler := len(ch.Code)
	ch.Patch(tr, handler)
	if x.CatchName != "" {
		if x.ResolvedLocal {
			ch.Emit(bytecode.OpStoreLocal, x.ResolvedSlot, x.CatchName, x.Span)
		} else {
			ch.Emit(bytecode.OpStoreName, ch.AddConstant(x.CatchName), "", x.Span)
		}
		ch.Emit(bytecode.OpPop, 0, "", x.Span)
		if x.HasFinally {
			catchTry := ch.Emit(bytecode.OpTry, 0, "", x.Span)
			c.compileBlock(ch, x.CatchBody)
			ch.Emit(bytecode.OpEndTry, 0, "", x.Span)
			c.compileFinallyAt(ch, finallyIndex)
			catchJump := ch.Emit(bytecode.OpJump, 0, "", x.Span)
			catchError := len(ch.Code)
			ch.Patch(catchTry, catchError)
			c.compileFinallyAt(ch, finallyIndex)
			ch.Emit(bytecode.OpThrow, 0, "", x.Span)
			ch.Patch(catchJump, len(ch.Code))
		} else {
			c.compileBlock(ch, x.CatchBody)
		}
	} else if x.HasFinally {
		c.compileFinallyAt(ch, finallyIndex)
		ch.Emit(bytecode.OpThrow, 0, "", x.Span)
	} else {
		ch.Emit(bytecode.OpThrow, 0, "", x.Span)
	}
	end := len(ch.Code)
	ch.Patch(jump, end)
	c.runtimeDepth--
	ch.Emit(bytecode.OpExitScope, 0, "", x.Span)
	if x.HasFinally {
		c.finallyStack = c.finallyStack[:finallyIndex]
		c.finallyLimit = oldLimit
	}
}

func (c *Compiler) compileExpr(ch *bytecode.Chunk, e ast.Expr) {
	switch x := e.(type) {
	case *ast.Literal:
		switch x.Kind {
		case "nil":
			ch.Emit(bytecode.OpNil, 0, "", x.Span)
		case "bool":
			if x.Value.(bool) {
				ch.Emit(bytecode.OpTrue, 0, "", x.Span)
			} else {
				ch.Emit(bytecode.OpFalse, 0, "", x.Span)
			}
		default:
			ch.Emit(bytecode.OpConstant, ch.AddConstant(x.Value), "", x.Span)
		}
	case *ast.Name:
		switch x.ResolvedKind {
		case ast.NameLocal:
			ch.Emit(bytecode.OpLoadLocal, x.ResolvedSlot, x.Value, x.Span)
		case ast.NameUpvalue:
			ch.Emit(bytecode.OpLoadUpvalue, c.upvalueIndex(x.Value), x.Value, x.Span)
		default:
			ch.Emit(bytecode.OpLoadName, ch.AddConstant(x.Value), "", x.Span)
		}
	case *ast.Unary:
		c.compileExpr(ch, x.Right)
		ch.Emit(bytecode.OpUnary, 0, x.Operator, x.Span)
	case *ast.Binary:
		if x.Operator == "且" {
			c.compileExpr(ch, x.Left)
			j := ch.Emit(bytecode.OpJumpIfFalse, 0, "", x.Span)
			c.compileExpr(ch, x.Right)
			done := ch.Emit(bytecode.OpJump, 0, "", x.Span)
			falsePath := len(ch.Code)
			ch.Emit(bytecode.OpFalse, 0, "", x.Span)
			ch.Patch(j, falsePath)
			ch.Patch(done, len(ch.Code))
			return
		}
		if x.Operator == "或" {
			c.compileExpr(ch, x.Left)
			j := ch.Emit(bytecode.OpJumpIfTrue, 0, "", x.Span)
			c.compileExpr(ch, x.Right)
			done := ch.Emit(bytecode.OpJump, 0, "", x.Span)
			truePath := len(ch.Code)
			ch.Emit(bytecode.OpTrue, 0, "", x.Span)
			ch.Patch(j, truePath)
			ch.Patch(done, len(ch.Code))
			return
		}
		c.compileExpr(ch, x.Left)
		c.compileExpr(ch, x.Right)
		ch.Emit(bytecode.OpBinary, 0, x.Operator, x.Span)
	case *ast.Assign:
		switch t := x.Target.(type) {
		case *ast.Name:
			c.compileExpr(ch, x.Value)
			switch t.ResolvedKind {
			case ast.NameLocal:
				ch.Emit(bytecode.OpStoreLocal, t.ResolvedSlot, t.Value, x.Span)
			case ast.NameUpvalue:
				ch.Emit(bytecode.OpStoreUpvalue, c.upvalueIndex(t.Value), t.Value, x.Span)
			default:
				ch.Emit(bytecode.OpStoreName, ch.AddConstant(t.Value), "", x.Span)
			}
		case *ast.Field:
			c.compileExpr(ch, t.Object)
			c.compileExpr(ch, x.Value)
			ch.Emit(bytecode.OpFieldSet, ch.AddConstant(t.Name), "", x.Span)
		case *ast.Index:
			c.compileExpr(ch, t.Object)
			c.compileExpr(ch, t.Key)
			c.compileExpr(ch, x.Value)
			ch.Emit(bytecode.OpIndexSet, 0, "", x.Span)
		}
	case *ast.Call:
		c.compileExpr(ch, x.Callee)
		for _, a := range x.Args {
			c.compileExpr(ch, a)
		}
		ch.Emit(bytecode.OpCall, len(x.Args), "", x.Span)
	case *ast.Index:
		c.compileExpr(ch, x.Object)
		c.compileExpr(ch, x.Key)
		ch.Emit(bytecode.OpIndexGet, 0, "", x.Span)
	case *ast.Field:
		c.compileExpr(ch, x.Object)
		ch.Emit(bytecode.OpFieldGet, ch.AddConstant(x.Name), "", x.Span)
	case *ast.List:
		for _, i := range x.Items {
			c.compileExpr(ch, i)
		}
		ch.Emit(bytecode.OpMakeList, len(x.Items), "", x.Span)
	case *ast.Dict:
		for _, pair := range x.Pairs {
			c.compileExpr(ch, pair.Key)
			c.compileExpr(ch, pair.Value)
		}
		op := bytecode.OpMakeDict
		if x.Record {
			op = bytecode.OpMakeRecord
		}
		ch.Emit(op, len(x.Pairs), "", x.Span)
	}
}
