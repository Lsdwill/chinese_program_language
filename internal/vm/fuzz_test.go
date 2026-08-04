package vm

import (
	"bytes"
	"huayan/internal/bytecode"
	"testing"
)

// FuzzVMExecuteNeverPanics ensures malformed but user-reachable bytecode is
// rejected as a RuntimeError rather than escaping the interpreter boundary.
func FuzzVMExecuteNeverPanics(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7})
	f.Add([]byte{byte(bytecode.OpMakeClosure), 0xff, byte(bytecode.OpThrow)})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 128 {
			data = data[:128]
		}
		code := make([]bytecode.Instruction, len(data))
		for i, b := range data {
			op := bytecode.OpCode(int(b) % int(bytecode.OpStoreUpvalue+1))
			arg := 0
			if i+1 < len(data) {
				arg = int(int8(data[i+1]))
			}
			code[i] = bytecode.Instruction{Op: op, Arg: arg, Text: "测试"}
		}
		chunk := &bytecode.Chunk{
			Name:      "损坏输入",
			Code:      code,
			Constants: []any{"名称"},
			Slots:     2,
			Upvalues:  []string{"上值"},
		}
		v := New(&bytes.Buffer{}, bytes.NewReader(nil), nil)
		v.Execute(chunk, nil)
	})
}
