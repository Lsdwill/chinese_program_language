package bytecode

import "testing"

func FuzzValidateNeverPanics(f *testing.F) {
	f.Add([]byte{0, 0, 1, 2, 3})
	f.Add([]byte{255, 1, 0, 9})
	f.Fuzz(func(t *testing.T, data []byte) {
		c := &Chunk{Slots: len(data) % 8}
		for i, b := range data {
			c.Code = append(c.Code, Instruction{Op: OpCode(int(b)), Arg: int(int8(b)), Text: "fuzz"})
			if i%7 == 0 {
				c.Constants = append(c.Constants, "名称")
			}
		}
		_ = c.Validate()
	})
}
