package engine

import (
	"io"
	"strings"
	"testing"
)

func BenchmarkVMRecursiveFunction(b *testing.B) {
	_, ch, ds := Compile("<基准>", "函数 阶乘(n)\n如果 n <= 1\n返回 1\n结束\n返回 n * 阶乘(n - 1)\n结束\n阶乘(12)")
	if len(ds) != 0 {
		b.Fatal(FormatDiagnostics(ds))
	}
	e := New("", io.Discard, strings.NewReader(""), nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, re := e.VM.Execute(ch, e.VM.Globals()); re != nil {
			b.Fatal(FormatRuntime(re))
		}
	}
}
