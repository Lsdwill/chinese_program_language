package token

import (
	"fmt"
	"huayan/internal/source"
)

type Kind int

const (
	EOF Kind = iota
	Newline
	Identifier
	Integer
	Float
	String
	LeftParen
	RightParen
	LeftBracket
	RightBracket
	LeftBrace
	RightBrace
	Comma
	Colon
	Dot
	Semicolon
	Plus
	Minus
	Star
	Slash
	Percent
	Assign
	Equal
	NotEqual
	Less
	LessEqual
	Greater
	GreaterEqual
	Module
	Import
	From
	As
	Export
	Let
	Const
	Function
	Return
	If
	Else
	ElseIf
	End
	While
	For
	In
	Break
	Continue
	True
	False
	Nil
	And
	Or
	Not
	Try
	Catch
	Finally
	Throw
	Record
	Select
	Where
	OrderBy
	Ascending
	Descending
	Limit
	Equals
)

var names = map[Kind]string{
	EOF: "文件结束", Newline: "换行", Identifier: "标识符", Integer: "整数", Float: "小数", String: "文字",
	LeftParen: "(", RightParen: ")", LeftBracket: "[", RightBracket: "]", LeftBrace: "{", RightBrace: "}", Comma: ",", Colon: ":", Dot: ".", Semicolon: ";",
	Plus: "+", Minus: "-", Star: "*", Slash: "/", Percent: "%", Assign: "=", Equal: "==", NotEqual: "!=", Less: "<", LessEqual: "<=", Greater: ">", GreaterEqual: ">=",
	Module: "模块", Import: "导入", From: "从", As: "为", Export: "公开", Let: "让", Const: "常量", Function: "函数", Return: "返回", If: "如果", Else: "否则", ElseIf: "否则如果", End: "结束", While: "当", For: "遍历", In: "于", Break: "跳出", Continue: "继续", True: "真", False: "假", Nil: "空", And: "且", Or: "或", Not: "非", Try: "尝试", Catch: "捕获", Finally: "最后", Throw: "抛出", Record: "记录", Select: "选择", Where: "其中", OrderBy: "排序", Ascending: "升序", Descending: "降序", Limit: "限制", Equals: "等于",
}

func (k Kind) String() string {
	if s, ok := names[k]; ok {
		return s
	}
	return fmt.Sprintf("Token(%d)", k)
}

type Token struct {
	Kind       Kind
	Literal    string // 原始拼写或文字内容
	Normalized string // 标识符的 NFC 规范化拼写
	Span       source.Span
}

func (t Token) String() string { return fmt.Sprintf("%s(%q)", t.Kind, t.Literal) }
func (t Token) Name() string {
	if t.Normalized != "" {
		return t.Normalized
	}
	return t.Literal
}

var Keywords = map[string]Kind{
	"模块": Module, "导入": Import, "从": From, "为": As, "公开": Export, "让": Let, "常量": Const, "函数": Function, "返回": Return, "如果": If, "否则": Else, "否则如果": ElseIf, "结束": End, "当": While, "遍历": For, "于": In, "跳出": Break, "继续": Continue, "真": True, "假": False, "空": Nil, "且": And, "或": Or, "非": Not, "尝试": Try, "捕获": Catch, "最后": Finally, "抛出": Throw, "记录": Record,
}
