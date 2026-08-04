package parser

import (
	"strconv"

	"huayan/internal/ast"
	"huayan/internal/diagnostic"
	"huayan/internal/source"
	"huayan/internal/token"
)

type Parser struct {
	tokens  []token.Token
	current int
	errors  []diagnostic.Diagnostic
}

func Parse(tokens []token.Token) (*ast.Program, []diagnostic.Diagnostic) {
	if len(tokens) == 0 {
		tokens = []token.Token{{Kind: token.EOF}}
	} else if tokens[len(tokens)-1].Kind != token.EOF {
		tokens = append(append([]token.Token(nil), tokens...), token.Token{Kind: token.EOF, Span: tokens[len(tokens)-1].Span})
	}
	p := &Parser{tokens: tokens}
	p.skipLines()
	start := p.peek().Span
	program := &ast.Program{Span: start}
	if p.match(token.Module) {
		program.Module = p.modulePath("模块名")
		p.endLine()
	}
	for !p.check(token.EOF) {
		p.skipLines()
		if p.check(token.EOF) {
			break
		}
		if s := p.statement(false); s != nil {
			program.Statements = append(program.Statements, s)
		} else {
			p.synchronize()
		}
	}
	program.Span.End = p.peek().Span.End
	return program, p.errors
}

func (p *Parser) peek() token.Token     { return p.tokens[p.current] }
func (p *Parser) previous() token.Token { return p.tokens[p.current-1] }
func (p *Parser) advance() token.Token {
	if !p.check(token.EOF) {
		p.current++
	}
	return p.previous()
}
func (p *Parser) check(k token.Kind) bool { return p.peek().Kind == k }
func (p *Parser) match(ks ...token.Kind) bool {
	for _, k := range ks {
		if p.check(k) {
			p.advance()
			return true
		}
	}
	return false
}
func (p *Parser) expect(k token.Kind, message string) token.Token {
	if p.check(k) {
		return p.advance()
	}
	p.report(p.peek().Span, "E1101", message, "")
	return token.Token{Kind: k, Span: p.peek().Span}
}
func (p *Parser) report(s source.Span, code, msg, hint string) {
	p.errors = append(p.errors, diagnostic.Diagnostic{Severity: diagnostic.Error, Code: code, Message: msg, Hint: hint, Span: s})
}
func (p *Parser) skipLines() {
	for p.match(token.Newline) {
	}
}
func (p *Parser) endLine() {
	if p.check(token.Newline) {
		p.skipLines()
		return
	}
	if p.check(token.EOF) || p.check(token.End) || p.check(token.Else) || p.check(token.Catch) || p.check(token.Finally) {
		return
	}
	p.report(p.peek().Span, "E1102", "语句后需要换行或分号", "")
	p.synchronize()
}
func (p *Parser) synchronize() {
	for !p.check(token.EOF) && !p.check(token.Newline) && !p.check(token.End) && !p.check(token.Else) && !p.check(token.Catch) && !p.check(token.Finally) {
		p.advance()
	}
	p.skipLines()
}

func (p *Parser) statement(public bool) ast.Stmt {
	t := p.peek()
	switch t.Kind {
	case token.Export:
		p.advance()
		return p.statement(true)
	case token.Import:
		return p.importStatement()
	case token.From:
		return p.fromImportStatement()
	case token.Let, token.Const:
		p.advance()
		name := p.expect(token.Identifier, "声明需要变量名")
		p.expect(token.Assign, "声明需要初始化表达式")
		value := p.expression(0)
		p.endLine()
		end := name.Span.End
		if value != nil {
			end = value.GetSpan().End
		}
		return &ast.VarDecl{Name: name.Name(), Value: value, Constant: t.Kind == token.Const, Public: public, Span: source.Span{File: t.Span.File, Start: t.Span.Start, End: end}}
	case token.Function:
		return p.functionStatement(public)
	case token.If:
		return p.ifStatement()
	case token.While:
		return p.whileStatement()
	case token.For:
		return p.forStatement()
	case token.Try:
		return p.tryStatement()
	case token.Return:
		p.advance()
		var value ast.Expr
		if !p.check(token.Newline) && !p.check(token.End) && !p.check(token.Else) && !p.check(token.Catch) && !p.check(token.EOF) {
			value = p.expression(0)
		}
		p.endLine()
		return &ast.ReturnStmt{Value: value, Span: t.Span}
	case token.Break:
		p.advance()
		p.endLine()
		return &ast.BreakStmt{Span: t.Span}
	case token.Continue:
		p.advance()
		p.endLine()
		return &ast.ContinueStmt{Span: t.Span}
	case token.Throw:
		p.advance()
		value := p.expression(0)
		p.endLine()
		return &ast.ThrowStmt{Value: value, Span: t.Span}
	default:
		expr := p.expression(0)
		if expr == nil {
			return nil
		}
		p.endLine()
		return &ast.ExprStmt{Expression: expr, Span: expr.GetSpan()}
	}
}

func (p *Parser) importStatement() ast.Stmt {
	start := p.advance()
	path := p.modulePath("导入需要模块路径")
	alias := ""
	if p.match(token.As) {
		alias = p.expect(token.Identifier, "导入别名必须是标识符").Name()
	} else if path != "" {
		parts := splitPath(path)
		alias = parts[len(parts)-1]
	}
	p.endLine()
	return &ast.ImportStmt{Path: path, Alias: alias, Span: source.Span{File: start.Span.File, Start: start.Span.Start, End: p.previous().Span.End}}
}
func (p *Parser) fromImportStatement() ast.Stmt {
	start := p.advance()
	path := p.modulePath("从语句需要模块路径")
	p.expect(token.Import, "从模块导入需要‘导入’")
	var names []string
	for {
		n := p.expect(token.Identifier, "导入名称必须是标识符")
		names = append(names, n.Name())
		if !p.match(token.Comma) {
			break
		}
	}
	p.endLine()
	return &ast.ImportStmt{Path: path, Names: names, Span: source.Span{File: start.Span.File, Start: start.Span.Start, End: p.previous().Span.End}}
}
func splitPath(s string) []string {
	var out []string
	start := 0
	for i, r := range s {
		if r == '.' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}
func (p *Parser) modulePath(message string) string {
	if !p.check(token.Identifier) {
		p.report(p.peek().Span, "E1103", message, "")
		return ""
	}
	path := p.advance().Name()
	for p.match(token.Dot) {
		n := p.expect(token.Identifier, "模块路径的点号后需要名称")
		path += "." + n.Name()
	}
	return path
}

func (p *Parser) functionStatement(public bool) ast.Stmt {
	start := p.advance()
	name := p.expect(token.Identifier, "函数需要名称")
	p.expect(token.LeftParen, "函数名称后需要左括号")
	var params []string
	if !p.check(token.RightParen) {
		for {
			n := p.expect(token.Identifier, "参数必须是标识符")
			params = append(params, n.Name())
			if !p.match(token.Comma) {
				break
			}
		}
	}
	p.expect(token.RightParen, "函数参数缺少右括号")
	p.endLine()
	body := p.block(token.End)
	end := p.expect(token.End, "函数缺少‘结束’")
	p.endLine()
	return &ast.FunctionDecl{Name: name.Name(), Params: params, Body: body, Public: public, Span: source.Span{File: start.Span.File, Start: start.Span.Start, End: end.Span.End}}
}
func (p *Parser) block(stops ...token.Kind) []ast.Stmt {
	var body []ast.Stmt
	p.skipLines()
	for !p.check(token.EOF) && !p.hasStop(stops...) {
		if s := p.statement(false); s != nil {
			body = append(body, s)
		} else {
			p.synchronize()
		}
	}
	return body
}
func (p *Parser) hasStop(stops ...token.Kind) bool {
	for _, k := range stops {
		if p.check(k) {
			return true
		}
	}
	return false
}

func (p *Parser) ifStatement() ast.Stmt {
	start := p.advance()
	branches := []ast.IfBranch{}
	cond := p.expression(0)
	p.endLine()
	body := p.block(token.Else, token.ElseIf, token.End)
	branches = append(branches, ast.IfBranch{Condition: cond, Body: body})
	for {
		compound := p.match(token.ElseIf)
		if !compound && !p.match(token.Else) {
			break
		}
		if compound || p.match(token.If) {
			c := p.expression(0)
			p.endLine()
			b := p.block(token.Else, token.ElseIf, token.End)
			branches = append(branches, ast.IfBranch{Condition: c, Body: b})
			continue
		}
		p.endLine()
		elseBody := p.block(token.End)
		end := p.expect(token.End, "条件语句缺少‘结束’")
		p.endLine()
		return &ast.IfStmt{Branches: branches, Else: elseBody, Span: source.Span{File: start.Span.File, Start: start.Span.Start, End: end.Span.End}}
	}
	end := p.expect(token.End, "条件语句缺少‘结束’")
	p.endLine()
	return &ast.IfStmt{Branches: branches, Span: source.Span{File: start.Span.File, Start: start.Span.Start, End: end.Span.End}}
}
func (p *Parser) whileStatement() ast.Stmt {
	start := p.advance()
	c := p.expression(0)
	p.endLine()
	b := p.block(token.End)
	end := p.expect(token.End, "循环语句缺少‘结束’")
	p.endLine()
	return &ast.WhileStmt{Condition: c, Body: b, Span: source.Span{File: start.Span.File, Start: start.Span.Start, End: end.Span.End}}
}
func (p *Parser) forStatement() ast.Stmt {
	start := p.advance()
	n := p.expect(token.Identifier, "遍历需要变量名")
	p.expect(token.In, "遍历变量后需要‘于’")
	e := p.expression(0)
	p.endLine()
	b := p.block(token.End)
	end := p.expect(token.End, "遍历语句缺少‘结束’")
	p.endLine()
	return &ast.ForStmt{Name: n.Name(), Iterable: e, Body: b, Span: source.Span{File: start.Span.File, Start: start.Span.Start, End: end.Span.End}}
}
func (p *Parser) tryStatement() ast.Stmt {
	start := p.advance()
	p.endLine()
	body := p.block(token.Catch, token.Finally, token.End)
	catchName := ""
	var cb []ast.Stmt
	if p.match(token.Catch) {
		n := p.expect(token.Identifier, "捕获需要错误变量名")
		catchName = n.Name()
		p.endLine()
		cb = p.block(token.Finally, token.End)
	}
	hasFinally := p.match(token.Finally)
	var fb []ast.Stmt
	if hasFinally {
		p.endLine()
		fb = p.block(token.End)
	}
	if catchName == "" && !hasFinally {
		p.report(p.peek().Span, "E1104", "尝试语句需要‘捕获’或‘最后’", "")
	}
	end := p.expect(token.End, "尝试语句缺少‘结束’")
	p.endLine()
	return &ast.TryStmt{Body: body, CatchName: catchName, CatchBody: cb, Finally: fb, HasFinally: hasFinally, Span: source.Span{File: start.Span.File, Start: start.Span.Start, End: end.Span.End}}
}

var precedence = map[token.Kind]int{token.Assign: 1, token.Or: 2, token.And: 3, token.Equal: 4, token.NotEqual: 4, token.Less: 5, token.LessEqual: 5, token.Greater: 5, token.GreaterEqual: 5, token.Plus: 6, token.Minus: 6, token.Star: 7, token.Slash: 7, token.Percent: 7}

func (p *Parser) expression(min int) ast.Expr {
	left := p.prefix()
	if left == nil {
		return nil
	}
	for {
		// Calls, indexing and fields bind more tightly than every binary op.
		if p.match(token.LeftParen) {
			var args []ast.Expr
			if !p.check(token.RightParen) {
				for {
					args = append(args, p.expression(0))
					if !p.match(token.Comma) {
						break
					}
				}
			}
			end := p.expect(token.RightParen, "调用缺少右括号")
			left = &ast.Call{Callee: left, Args: args, Span: source.Span{File: left.GetSpan().File, Start: left.GetSpan().Start, End: end.Span.End}}
			continue
		}
		if p.match(token.LeftBracket) {
			key := p.expression(0)
			end := p.expect(token.RightBracket, "索引缺少右括号")
			left = &ast.Index{Object: left, Key: key, Span: source.Span{File: left.GetSpan().File, Start: left.GetSpan().Start, End: end.Span.End}}
			continue
		}
		if p.match(token.Dot) {
			n := p.expect(token.Identifier, "点号后需要字段名")
			left = &ast.Field{Object: left, Name: n.Name(), Span: source.Span{File: left.GetSpan().File, Start: left.GetSpan().Start, End: n.Span.End}}
			continue
		}
		op := p.peek()
		prec, ok := precedence[op.Kind]
		if !ok || prec < min {
			return left
		}
		p.advance()
		rightMin := prec + 1
		if op.Kind == token.Assign {
			rightMin = prec
		}
		right := p.expression(rightMin)
		if right == nil {
			p.report(op.Span, "E1104", "运算符后需要表达式", "")
			return left
		}
		if op.Kind == token.Assign {
			left = &ast.Assign{Target: left, Value: right, Span: source.Span{File: left.GetSpan().File, Start: left.GetSpan().Start, End: right.GetSpan().End}}
		} else {
			left = &ast.Binary{Left: left, Operator: opText(op), Right: right, Span: source.Span{File: left.GetSpan().File, Start: left.GetSpan().Start, End: right.GetSpan().End}}
		}
	}
}

func (p *Parser) prefix() ast.Expr {
	t := p.advance()
	switch t.Kind {
	case token.Select:
		return p.queryExpression(t)
	case token.Integer:
		n, e := strconv.ParseInt(t.Literal, 10, 64)
		if e != nil {
			p.report(t.Span, "E1105", "整数超出 64 位范围", "")
			return &ast.Literal{Kind: "nil", Span: t.Span}
		}
		return &ast.Literal{Kind: "int", Value: n, Span: t.Span}
	case token.Float:
		n, e := strconv.ParseFloat(t.Literal, 64)
		if e != nil {
			p.report(t.Span, "E1106", "小数格式无效", "")
			return &ast.Literal{Kind: "nil", Span: t.Span}
		}
		return &ast.Literal{Kind: "float", Value: n, Span: t.Span}
	case token.String:
		return &ast.Literal{Kind: "string", Value: t.Literal, Span: t.Span}
	case token.True:
		return &ast.Literal{Kind: "bool", Value: true, Span: t.Span}
	case token.False:
		return &ast.Literal{Kind: "bool", Value: false, Span: t.Span}
	case token.Nil:
		return &ast.Literal{Kind: "nil", Span: t.Span}
	case token.Identifier:
		if t.Name() == "选择" && p.check(token.Identifier) {
			return p.queryExpression(t)
		}
		return &ast.Name{Value: t.Name(), Span: t.Span}
	case token.Not, token.Minus:
		r := p.expression(8)
		end := t.Span.End
		if r != nil {
			end = r.GetSpan().End
		}
		return &ast.Unary{Operator: opText(t), Right: r, Span: source.Span{File: t.Span.File, Start: t.Span.Start, End: end}}
	case token.LeftParen:
		e := p.expression(0)
		p.expect(token.RightParen, "表达式缺少右括号")
		return e
	case token.LeftBracket:
		var items []ast.Expr
		if !p.check(token.RightBracket) {
			for {
				items = append(items, p.expression(0))
				if !p.match(token.Comma) {
					break
				}
			}
		}
		end := p.expect(token.RightBracket, "列表缺少右括号")
		return &ast.List{Items: items, Span: source.Span{File: t.Span.File, Start: t.Span.Start, End: end.Span.End}}
	case token.Record:
		p.expect(token.LeftBrace, "记录后需要左大括号")
		return p.dictLiteral(t, true)
	case token.LeftBrace:
		return p.dictLiteral(t, false)
	default:
		p.report(t.Span, "E1107", "这里需要表达式", "")
		return nil
	}
}

func (p *Parser) queryExpression(start token.Token) ast.Expr {
	table := p.expect(token.Identifier, "选择后需要表名")
	p.expect(token.From, "表名后需要‘从’和数据库变量")
	database := p.expression(0)
	query := &ast.Query{Table: table.Name(), Database: database, Span: source.Span{File: start.Span.File, Start: start.Span.Start, End: table.Span.End}}
	p.endLine()
	for !p.check(token.End) && !p.check(token.EOF) {
		switch {
		case p.word("其中"):
			p.advance()
			field := p.expect(token.Identifier, "其中后需要字段名")
			p.expectWord("等于", "条件字段后需要‘等于’")
			query.WhereField = field.Name()
			query.WhereValue = p.expression(0)
			p.endLine()
		case p.word("排序"):
			p.advance()
			field := p.expect(token.Identifier, "排序后需要字段名")
			query.OrderField = field.Name()
			if p.word("降序") {
				p.advance()
				query.Descending = true
			} else {
				if p.word("升序") { p.advance() }
			}
			p.endLine()
		case p.word("限制"):
			p.advance()
			query.Limit = p.expression(0)
			p.endLine()
		default:
			p.report(p.peek().Span, "E1110", "查询块只支持‘其中’、‘排序’和‘限制’", "")
			p.synchronize()
		}
	}
	end := p.expect(token.End, "查询缺少‘结束’")
	query.Span.End = end.Span.End
	return query
}

func (p *Parser) word(word string) bool {
	return p.check(token.Identifier) && p.peek().Name() == word
}

func (p *Parser) expectWord(word, message string) token.Token {
	if p.word(word) { return p.advance() }
	p.report(p.peek().Span, "E1101", message, "")
	return token.Token{Kind: token.Identifier, Span: p.peek().Span}
}
func (p *Parser) dictLiteral(start token.Token, record bool) ast.Expr {
	var pairs []ast.Pair
	if !p.check(token.RightBrace) {
		for {
			var key ast.Expr
			if record && p.check(token.Identifier) {
				n := p.advance()
				key = &ast.Literal{Kind: "string", Value: n.Name(), Span: n.Span}
			} else {
				key = p.expression(0)
			}
			p.expect(token.Colon, "字典或记录字段需要冒号")
			v := p.expression(0)
			pairs = append(pairs, ast.Pair{Key: key, Value: v})
			if !p.match(token.Comma) {
				break
			}
		}
	}
	end := p.expect(token.RightBrace, "字典或记录缺少右大括号")
	return &ast.Dict{Pairs: pairs, Record: record, Span: source.Span{File: start.Span.File, Start: start.Span.Start, End: end.Span.End}}
}

func opText(t token.Token) string {
	if t.Literal != "" {
		return t.Literal
	}
	return t.Kind.String()
}
