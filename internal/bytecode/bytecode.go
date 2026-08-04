package bytecode

import (
	"fmt"
	"strings"

	"huayan/internal/source"
)

type OpCode int

const (
	OpConstant OpCode = iota
	OpNil
	OpTrue
	OpFalse
	OpPop
	OpLoadName
	OpDeclareName
	OpDeclareEmpty
	OpStoreName
	OpExportName
	OpUnary
	OpBinary
	OpJump
	OpJumpIfFalse
	OpJumpIfTrue
	OpCall
	OpReturn
	OpMakeClosure
	OpMakeList
	OpMakeDict
	OpMakeRecord
	OpIndexGet
	OpIndexSet
	OpFieldGet
	OpFieldSet
	OpEnterScope
	OpExitScope
	OpIterStart
	OpIterNext
	OpIterEnd
	OpTry
	OpEndTry
	OpThrow
	OpImport
	OpGetExport
	OpLoadLocal
	OpDeclareLocal
	OpStoreLocal
	OpLoadUpvalue
	OpStoreUpvalue
)

var opNames = map[OpCode]string{
	OpConstant: "加载常量", OpNil: "加载空", OpTrue: "加载真", OpFalse: "加载假", OpPop: "弹出",
	OpLoadName: "读取名称", OpDeclareName: "声明名称", OpDeclareEmpty: "声明空名称", OpStoreName: "写入名称", OpExportName: "导出名称",
	OpLoadLocal: "读取局部槽", OpDeclareLocal: "声明局部槽", OpStoreLocal: "写入局部槽", OpLoadUpvalue: "读取上值", OpStoreUpvalue: "写入上值",
	OpUnary: "一元运算", OpBinary: "二元运算", OpJump: "跳转", OpJumpIfFalse: "条件假跳转", OpJumpIfTrue: "条件真跳转", OpCall: "调用", OpReturn: "返回", OpMakeClosure: "创建闭包", OpMakeList: "创建列表", OpMakeDict: "创建字典", OpMakeRecord: "创建记录", OpIndexGet: "读取索引", OpIndexSet: "写入索引", OpFieldGet: "读取字段", OpFieldSet: "写入字段", OpEnterScope: "进入作用域", OpExitScope: "退出作用域", OpIterStart: "开始迭代", OpIterNext: "迭代下一项", OpIterEnd: "结束迭代", OpTry: "建立捕获区", OpEndTry: "退出捕获区", OpThrow: "抛出", OpImport: "导入模块", OpGetExport: "读取导出",
}

func (o OpCode) String() string {
	if s, ok := opNames[o]; ok {
		return s
	}
	return fmt.Sprintf("指令(%d)", o)
}

type Instruction struct {
	Op       OpCode
	Arg      int
	Text     string
	Span     source.Span
	Constant bool
}
type FunctionProto struct {
	Name     string
	Params   []string
	Chunk    *Chunk
	Span     source.Span
	Slots    int
	Upvalues []string
}
type Chunk struct {
	Name      string
	File      *source.File
	Code      []Instruction
	Constants []any
	Slots     int
	SlotNames map[int]string
	Upvalues  []string
}

func (c *Chunk) AddConstant(v any) int {
	c.Constants = append(c.Constants, v)
	return len(c.Constants) - 1
}
func (c *Chunk) Emit(op OpCode, arg int, text string, span source.Span) int {
	c.Code = append(c.Code, Instruction{Op: op, Arg: arg, Text: text, Span: span})
	return len(c.Code) - 1
}
func (c *Chunk) Patch(at, target int) {
	if at >= 0 && at < len(c.Code) {
		c.Code[at].Arg = target
	}
}

func (c *Chunk) Disassemble() string {
	var b strings.Builder
	c.disassembleInto(&b, "")
	return b.String()
}

func (c *Chunk) disassembleInto(b *strings.Builder, indent string) {
	fmt.Fprintf(b, "%s== %s ==\n", indent, c.Name)
	fmt.Fprintf(b, "%s局部槽位: %d\n", indent, c.Slots)
	fmt.Fprintf(b, "%s上值: %v\n", indent, c.Upvalues)
	fmt.Fprintf(b, "%s常量池: %d\n", indent, len(c.Constants))
	for i, ins := range c.Code {
		fmt.Fprintf(b, "%s%04d  %-8s", indent, i, ins.Op)
		if ins.Text != "" {
			fmt.Fprintf(b, " %-12s", ins.Text)
		} else if hasOperand(ins.Op) {
			operand := fmt.Sprintf("%d", ins.Arg)
			if (ins.Op == OpLoadUpvalue || ins.Op == OpStoreUpvalue) && ins.Arg >= 0 && ins.Arg < len(c.Upvalues) {
				operand += fmt.Sprintf(" (%q)", c.Upvalues[ins.Arg])
			} else if ins.Arg >= 0 && ins.Arg < len(c.Constants) {
				switch ins.Op {
				case OpConstant, OpLoadName, OpDeclareName, OpDeclareEmpty, OpStoreName, OpExportName, OpImport, OpGetExport, OpFieldGet, OpFieldSet:
					operand += fmt.Sprintf(" (%q)", c.Constants[ins.Arg])
				}
			}
			fmt.Fprintf(b, " %s", operand)
		}
		b.WriteByte('\n')
	}
	for _, constant := range c.Constants {
		if proto, ok := constant.(*FunctionProto); ok && proto.Chunk != nil {
			fmt.Fprintf(b, "%s函数原型 %s 槽位=%d 上值=%v\n", indent, proto.Name, proto.Slots, proto.Upvalues)
			proto.Chunk.disassembleInto(b, indent+"  ")
		}
	}
}

func hasOperand(op OpCode) bool {
	switch op {
	case OpConstant, OpLoadName, OpDeclareName, OpDeclareEmpty, OpStoreName, OpExportName, OpJump, OpJumpIfFalse, OpJumpIfTrue, OpCall, OpMakeClosure, OpMakeList, OpMakeDict, OpMakeRecord, OpIterNext, OpTry, OpImport, OpGetExport, OpFieldGet, OpFieldSet, OpLoadLocal, OpDeclareLocal, OpStoreLocal, OpLoadUpvalue, OpStoreUpvalue:
		return true
	default:
		return false
	}
}
