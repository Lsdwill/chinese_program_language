package bytecode

import "fmt"

// Validate checks the structural invariants required by the VM. It is kept
// independent from execution so tools and embedders can reject malformed
// chunks before handing them to an interpreter.
func (c *Chunk) Validate() error {
	if c == nil {
		return validateChunk(nil, "字节码")
	}
	return validateChunk(c, c.Name)
}

func validateChunk(c *Chunk, name string) error {
	if c == nil {
		return fmt.Errorf("%s：空字节码块", name)
	}
	if c.Slots < 0 {
		return fmt.Errorf("%s：局部槽位数量无效：%d", name, c.Slots)
	}
	for ip, ins := range c.Code {
		if ins.Op < OpConstant || ins.Op > OpStoreUpvalue {
			return fmt.Errorf("%s：指令 %d 的操作码无效：%d", name, ip, ins.Op)
		}
		if needsConstant(ins.Op) && (ins.Arg < 0 || ins.Arg >= len(c.Constants)) {
			return fmt.Errorf("%s：指令 %d 的常量索引越界：%d", name, ip, ins.Arg)
		}
		if needsStringConstant(ins.Op) {
			if _, ok := c.Constants[ins.Arg].(string); !ok {
				return fmt.Errorf("%s：指令 %d 需要文字常量", name, ip)
			}
		}
		if isJump(ins.Op) && (ins.Arg < 0 || ins.Arg > len(c.Code)) {
			return fmt.Errorf("%s：指令 %d 的跳转目标越界：%d", name, ip, ins.Arg)
		}
		if ins.Op == OpCall && ins.Arg < 0 {
			return fmt.Errorf("%s：指令 %d 的参数数量无效：%d", name, ip, ins.Arg)
		}
		if (ins.Op == OpLoadLocal || ins.Op == OpDeclareLocal || ins.Op == OpStoreLocal) && (ins.Arg < 0 || ins.Arg >= c.Slots) {
			return fmt.Errorf("%s：指令 %d 的局部槽位无效：%d", name, ip, ins.Arg)
		}
		if (ins.Op == OpLoadUpvalue || ins.Op == OpStoreUpvalue) && (ins.Arg < 0 || ins.Arg >= len(c.Upvalues)) {
			return fmt.Errorf("%s：指令 %d 的上值索引无效：%d", name, ip, ins.Arg)
		}
		if ins.Op == OpMakeClosure {
			proto, ok := c.Constants[ins.Arg].(*FunctionProto)
			if !ok || proto == nil || proto.Chunk == nil {
				return fmt.Errorf("%s：指令 %d 的闭包原型无效", name, ip)
			}
			if err := validateChunk(proto.Chunk, proto.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

func needsStringConstant(op OpCode) bool {
	switch op {
	case OpLoadName, OpDeclareName, OpDeclareEmpty, OpStoreName, OpExportName,
		OpImport, OpGetExport, OpFieldGet, OpFieldSet:
		return true
	default:
		return false
	}
}

func needsConstant(op OpCode) bool {
	switch op {
	case OpConstant, OpLoadName, OpDeclareName, OpDeclareEmpty, OpStoreName,
		OpExportName, OpMakeClosure, OpImport, OpGetExport, OpFieldGet, OpFieldSet:
		return true
	default:
		return false
	}
}

func isJump(op OpCode) bool {
	return op == OpJump || op == OpJumpIfFalse || op == OpJumpIfTrue
}
