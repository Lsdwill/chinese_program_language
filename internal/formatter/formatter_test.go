package formatter

import "testing"

func TestFormatKeepsStringsAndComments(t *testing.T) {
	in := "打印(\"a;b\"); // c;d\n/* e;f */打印(2);"
	want := "打印(\"a;b\")\n// c;d\n/* e;f */打印(2)\n"
	if got := Format(in); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := Format(Format(in)); got != want {
		t.Fatalf("format is not idempotent: %q", got)
	}
}

func TestFormatStructuredIndentationGolden(t *testing.T) {
	in := "函数 计算(x)\n如果 x > 0\n返回 x\n否则\n返回 -x\n结束\n结束"
	want := "函数 计算(x)\n    如果 x > 0\n        返回 x\n    否则\n        返回 -x\n    结束\n结束"
	if got := Format(in); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	formatted := Format(in)
	if again := Format(formatted); again != formatted {
		t.Fatalf("not idempotent: %q", again)
	}
}
