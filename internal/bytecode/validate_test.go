package bytecode

import (
	"huayan/internal/source"
	"strings"
	"testing"
)

func TestValidateRejectsMalformedChunk(t *testing.T) {
	cases := []*Chunk{
		nil,
		{Slots: -1},
		{Code: []Instruction{{Op: OpConstant, Arg: 3}}},
		{Constants: []any{1}, Code: []Instruction{{Op: OpLoadName, Arg: 0}}},
		{Code: []Instruction{{Op: OpJump, Arg: 2}}},
		{Code: []Instruction{{Op: OpCall, Arg: -1}}},
		{Slots: 1, Code: []Instruction{{Op: OpLoadLocal, Arg: 1}}},
		{Code: []Instruction{{Op: OpMakeClosure, Arg: 0}}, Constants: []any{"不是函数"}},
		{Code: []Instruction{{Op: OpCode(999)}}},
		{Code: []Instruction{{Op: OpLoadUpvalue, Arg: 0}}},
	}
	for i, c := range cases {
		if err := c.Validate(); err == nil {
			t.Errorf("case %d was accepted", i)
		}
	}
}

func TestDisassembleAndPatchAreSafe(t *testing.T) {
	inner := &Chunk{Name: "内层", Slots: 1, Upvalues: []string{"值"}, Code: []Instruction{{Op: OpLoadUpvalue, Arg: 0}}}
	c := &Chunk{Name: "主", Slots: 1, Upvalues: []string{"值"}, Constants: []any{"名称", &FunctionProto{Name: "函数", Chunk: inner, Slots: 1, Upvalues: []string{"值"}}}}
	at := c.Emit(OpJump, 0, "", source.Span{})
	c.Emit(OpLoadUpvalue, 0, "", source.Span{})
	c.Emit(OpMakeClosure, 1, "", source.Span{})
	c.Patch(at, 2)
	c.Patch(-1, 0)
	c.Patch(99, 0)
	out := c.Disassemble()
	for _, want := range []string{"局部槽位: 1", "上值: [值]", "函数原型 函数", "(\"值\")"} {
		if !strings.Contains(out, want) {
			t.Fatalf("disassembly missing %q:\n%s", want, out)
		}
	}
}

func TestValidateAcceptsNestedFunction(t *testing.T) {
	inner := &Chunk{Upvalues: []string{"n"}, Code: []Instruction{{Op: OpLoadUpvalue, Arg: 0}, {Op: OpReturn}}}
	c := &Chunk{Constants: []any{&FunctionProto{Name: "内层", Chunk: inner, Upvalues: []string{"n"}}}, Code: []Instruction{{Op: OpMakeClosure, Arg: 0}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
