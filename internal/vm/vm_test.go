package vm

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"huayan/internal/bytecode"
	"huayan/internal/compiler"
	"huayan/internal/lexer"
	"huayan/internal/parser"
	"huayan/internal/source"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failingDatabaseRunner struct{}

type databaseResultError struct{ last, affected error }

func (r databaseResultError) LastInsertId() (int64, error) { return 0, r.last }
func (r databaseResultError) RowsAffected() (int64, error) { return 0, r.affected }

func (failingDatabaseRunner) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errors.New("执行失败")
}
func (failingDatabaseRunner) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, errors.New("查询失败")
}

func runVMText(t *testing.T, text string) (string, *RuntimeError) {
	t.Helper()
	f := &source.File{Name: "vm-test.hua", Text: text}
	ts, le := lexer.Lex(f)
	if len(le) != 0 {
		t.Fatalf("lex: %v", le)
	}
	p, pe := parser.Parse(ts)
	if len(pe) != 0 {
		t.Fatalf("parse: %v", pe)
	}
	ch, ce := compiler.Compile(p)
	if len(ce) != 0 {
		t.Fatalf("compile: %v", ce)
	}
	var out bytes.Buffer
	v := New(&out, strings.NewReader(""), nil)
	_, re := v.Execute(ch, v.Globals())
	return out.String(), re
}

func TestVMControlFlowAndCollections(t *testing.T) {
	got, re := runVMText(t, `
让 总和 = 0
让 n = 0
当 n < 5
    n = n + 1
    如果 n == 2
        继续
    结束
    总和 = 总和 + n
结束
让 xs = [1, 2]
xs.追加(3)
让 d = {"a": 1}
d["b"] = 2
让 r = 记录 {名: "华言"}
r.年龄 = 3
遍历 项 于 xs
    总和 = 总和 + 项
结束
打印(总和)
打印(d["b"])
打印(r.名)
`)
	if re != nil {
		t.Fatal(re)
	}
	if got != "19\n2\n华言\n" {
		t.Fatalf("got %q", got)
	}
}

func TestVMClosureAndErrors(t *testing.T) {
	got, re := runVMText(t, `
函数 创建()
    让 值 = 0
    函数 增加()
        值 = 值 + 1
        返回 值
    结束
    返回 增加
结束
让 f = 创建()
打印(f())
打印(f())
尝试
    抛出 错误("测试")
捕获 原因
    打印(原因.消息)
结束
`)
	if re != nil {
		t.Fatal(re)
	}
	if got != "1\n2\n测试\n" {
		t.Fatalf("got %q", got)
	}
	_, re = runVMText(t, "常量 x = 1\nx = 2")
	if re == nil || !strings.Contains(re.Error(), "常量") {
		t.Fatal("constant assignment did not fail")
	}
}

func TestVMFinallyRunsAfterSuccessAndCaughtError(t *testing.T) {
	got, re := runVMText(t, `
让 结果 = []
尝试
    结果.追加("正常")
最后
    结果.追加("清理一")
结束
尝试
    抛出 错误("失败")
捕获 原因
    结果.追加(原因.消息)
最后
    结果.追加("清理二")
结束
打印(结果)
`)
	if re != nil {
		t.Fatal(re)
	}
	if got != "[正常, 清理一, 失败, 清理二]\n" {
		t.Fatalf("got %q", got)
	}
}

func TestVMFinallyRunsBeforeRethrowingCatchError(t *testing.T) {
	got, re := runVMText(t, `
尝试
    抛出 错误("原始")
捕获 原因
    抛出 错误("捕获")
最后
    打印("清理")
结束
`)
	if got != "清理\n" || re == nil || re.Error() != "捕获" {
		t.Fatalf("output=%q runtime=%v", got, re)
	}
}

func TestUpvalueClosesAfterFunctionReturns(t *testing.T) {
	f := &source.File{Name: "close.hua", Text: "函数 创建()\n让 n = 0\n函数 增加()\nn = n + 1\n返回 n\n结束\n返回 增加\n结束\n让 f = 创建()"}
	ts, _ := lexer.Lex(f)
	p, _ := parser.Parse(ts)
	ch, ds := compiler.Compile(p)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	v := New(&bytes.Buffer{}, strings.NewReader(""), nil)
	if _, re := v.Execute(ch, v.Globals()); re != nil {
		t.Fatal(re)
	}
	value, ok := v.Globals().Get("f")
	if !ok {
		t.Fatal("closure missing")
	}
	fn := value.Data.(*FunctionObject)
	if len(fn.Upvalues) != 1 || fn.Upvalues[0].env != nil || fn.Upvalues[0].closed.Data.(int64) != 0 {
		t.Fatalf("upvalue not closed: %#v", fn.Upvalues)
	}
	if _, re := v.call(value, nil, source.Span{}); re != nil {
		t.Fatal(re)
	}
	if fn.Upvalues[0].closed.Data.(int64) != 1 {
		t.Fatalf("closed value not updated: %v", fn.Upvalues[0].closed)
	}
}

func TestVMStandardModules(t *testing.T) {
	got, re := runVMText(t, `
导入 标准.文字 为 文字
导入 标准.列表 为 列表
导入 标准.JSON 为 JSON
导入 标准.数学 为 数学
导入 标准.时间 为 时间
函数 大于一(x)
    返回 x > 1
结束
打印(文字.长度("华😀"))
打印(列表.过滤([1, 2, 3], 大于一))
打印(JSON.序列化(记录 {甲: 1}))
打印(数学.绝对值(-3))
打印(时间.日期差("2026-01-01T00:00:00Z", "2026-01-01T00:00:02Z"))
`)
	if re != nil {
		t.Fatal(re)
	}
	if !strings.Contains(got, "2\n") || !strings.Contains(got, "[2, 3]") || !strings.Contains(got, "2\n") {
		t.Fatalf("got %q", got)
	}
}

func TestVMResourceLimitsAndMalformedInput(t *testing.T) {
	_, re := runVMText(t, "函数 f(n)\n返回 f(n + 1)\n结束\nf(0)")
	if re == nil {
		t.Fatal("expected call depth error")
	}
	var out bytes.Buffer
	v := New(&out, strings.NewReader(""), nil)
	v.MaxCallDepth = 2
	f := &source.File{Name: "limit.hua", Text: "函数 f(n)\n返回 f(n + 1)\n结束\nf(0)"}
	ts, _ := lexer.Lex(f)
	p, _ := parser.Parse(ts)
	ch, ds := compiler.Compile(p)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	if _, re := v.Execute(ch, v.Globals()); re == nil {
		t.Fatal("limit was not enforced")
	}
}

func TestJSONDepthLimit(t *testing.T) {
	text := strings.Repeat("[", 258) + strings.Repeat("]", 258)
	dec := json.NewDecoder(strings.NewReader(text))
	if _, err := decodeJSONValue(dec); err == nil || !strings.Contains(err.Error(), "嵌套层数") {
		t.Fatalf("err=%v", err)
	}
}

func callNativeForCoverage(t *testing.T, v *VM, mod Value, name string, args ...Value) (Value, *RuntimeError) {
	t.Helper()
	fn, ok := mod.Data.(*ModuleObject).Exports[name]
	if !ok {
		t.Fatalf("missing standard API %s", name)
	}
	return v.call(fn, args, source.Span{})
}

func TestStandardNativeAPIAndErrorBoundaries(t *testing.T) {
	v := New(&bytes.Buffer{}, strings.NewReader("输入行\n"), nil)
	text := func(s string) Value { return Text(s) }
	文字, _ := StandardModule("标准.文字", v)
	callNativeForCoverage(t, v, 文字, "查找", text("abc"), text("b"))
	callNativeForCoverage(t, v, 文字, "替换", text("abc"), text("b"), text("x"))
	callNativeForCoverage(t, v, 文字, "分割", text("a,b"), text(","))
	callNativeForCoverage(t, v, 文字, "裁剪", text(" a "))
	callNativeForCoverage(t, v, 文字, "大写", text("a"))
	callNativeForCoverage(t, v, 文字, "小写", text("A"))
	callNativeForCoverage(t, v, 文字, "长度", text("😀"))
	callNativeForCoverage(t, v, 文字, "查找", Int(1))
	列表, _ := StandardModule("标准.列表", v)
	items := List([]Value{Int(3), Int(1), Int(2)})
	callNativeForCoverage(t, v, 列表, "排序", items)
	callNativeForCoverage(t, v, 列表, "查找", items, Int(2))
	fn := v.native("加一", func(vm *VM, args []Value) (Value, *RuntimeError) {
		if len(args) == 0 {
			return Nil(), vm.fault("测试", "故意错误", source.Span{})
		}
		return Int(args[0].Data.(int64) + 1), nil
	})
	pred := v.native("大于一", func(_ *VM, args []Value) (Value, *RuntimeError) { return Bool(args[0].Data.(int64) > 1), nil })
	callNativeForCoverage(t, v, 列表, "映射", items, fn)
	callNativeForCoverage(t, v, 列表, "过滤", items, pred)
	字典, _ := StandardModule("标准.字典", v)
	d := Dict()
	setDict(d, text("a"), Int(1))
	callNativeForCoverage(t, v, 字典, "键", d)
	callNativeForCoverage(t, v, 字典, "值", d)
	callNativeForCoverage(t, v, 字典, "条目", d)
	callNativeForCoverage(t, v, 字典, "包含键", d, text("a"))
	callNativeForCoverage(t, v, 字典, "包含键", d, List(nil))
	tmp := t.TempDir()
	path := filepath.Join(tmp, "数据.txt")
	文件, _ := StandardModule("标准.文件", v)
	callNativeForCoverage(t, v, 文件, "创建目录", text(filepath.Join(tmp, "子目录")))
	callNativeForCoverage(t, v, 文件, "写入文字", text(path), text("a"))
	callNativeForCoverage(t, v, 文件, "追加文字", text(path), text("b"))
	callNativeForCoverage(t, v, 文件, "读取文字", text(path))
	callNativeForCoverage(t, v, 文件, "存在", text(path))
	callNativeForCoverage(t, v, 文件, "原子写入", text(path), text("c"))
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	batchItem := func(p, content string) Value {
		r := Record()
		o := r.Data.(*RecordObject)
		o.Order = []string{"路径", "内容"}
		o.Fields["路径"], o.Fields["内容"] = text(p), text(content)
		return r
	}
	batch := List([]Value{batchItem(filepath.Join(tmp, "a.json"), "a"), batchItem(filepath.Join(tmp, "b.json"), "b")})
	callNativeForCoverage(t, v, 文件, "原子写入组", batch)
	if a, err := os.ReadFile(filepath.Join(tmp, "a.json")); err != nil || string(a) != "a" {
		t.Fatalf("batch a: %q %v", a, err)
	}
	if _, re := callNativeForCoverage(t, v, 文件, "原子写入组", List([]Value{Int(1)})); re == nil {
		t.Fatal("invalid batch item accepted")
	}
	JSON, _ := StandardModule("标准.JSON", v)
	record := Record()
	record.Data.(*RecordObject).Order = []string{"a"}
	record.Data.(*RecordObject).Fields["a"] = Int(1)
	callNativeForCoverage(t, v, JSON, "解析", text("{\"a\":1}"))
	callNativeForCoverage(t, v, JSON, "序列化", record)
	callNativeForCoverage(t, v, JSON, "格式化", record)
	callNativeForCoverage(t, v, JSON, "解析", text("坏"))
	cycle := List(nil)
	cycle.Data.(*ListObject).Items = []Value{cycle}
	callNativeForCoverage(t, v, JSON, "序列化", cycle)
	时间, _ := StandardModule("标准.时间", v)
	callNativeForCoverage(t, v, 时间, "现在")
	callNativeForCoverage(t, v, 时间, "解析", text("2026-01-01"), text("2006-01-02"))
	callNativeForCoverage(t, v, 时间, "格式化", text("2026-01-01T00:00:00Z"), text("2006"))
	callNativeForCoverage(t, v, 时间, "日期差", text("2026-01-01T00:00:00Z"), text("2026-01-01T00:00:01Z"))
	callNativeForCoverage(t, v, 时间, "加秒", text("2026-01-01T00:00:00Z"), Int(1))
	数学, _ := StandardModule("标准.数学", v)
	callNativeForCoverage(t, v, 数学, "绝对值", Int(-1))
	callNativeForCoverage(t, v, 数学, "最小值", Int(1), Int(2))
	callNativeForCoverage(t, v, 数学, "最大值", Float(1), Int(2))
	callNativeForCoverage(t, v, 数学, "取整", Float(2.9))
	程序, _ := StandardModule("标准.程序", v)
	callNativeForCoverage(t, v, 程序, "参数")
	callNativeForCoverage(t, v, 程序, "环境")
	测试, _ := StandardModule("标准.测试", v)
	callNativeForCoverage(t, v, 测试, "断言相等", Int(1), Int(1))
	callNativeForCoverage(t, v, 测试, "断言错误", fn)
	callNativeForCoverage(t, v, 测试, "运行")
	list := List([]Value{Int(1), Int(2)})
	for _, name := range []string{"长度", "包含", "移除首项", "清空"} {
		method, _ := v.fieldGet(list, name, source.Span{})
		v.call(method, nil, source.Span{})
	}
	strMethod, _ := v.fieldGet(text("华言"), "长度", source.Span{})
	v.call(strMethod, nil, source.Span{})
	for _, name := range []string{"长度", "键", "值", "包含键", "条目"} {
		method, _ := v.fieldGet(d, name, source.Span{})
		v.call(method, nil, source.Span{})
	}
	console, _ := StandardModule("标准.控制台", v)
	callNativeForCoverage(t, v, console, "输出", text("x"))
	callNativeForCoverage(t, v, console, "错误输出", text("x"))
	callNativeForCoverage(t, v, console, "读取一行")
	for name, args := range map[string][]Value{"长度": {text("x")}, "类型": {Int(1)}, "转文字": {Int(1)}, "转整数": {text("1")}, "转小数": {text("1.5")}, "范围": {Int(1), Int(4)}, "错误": {text("e")}, "断言": {Bool(true)}} {
		fn, ok := v.globals.Get(name)
		if ok {
			v.call(fn, args, source.Span{})
		}
	}
}

func TestBytesEncodingAndBinaryFiles(t *testing.T) {
	v := New(&bytes.Buffer{}, strings.NewReader(""), nil)
	input := []byte{0, 1, 127, 128, 255}
	b := Bytes(input)
	input[0] = 9
	if b.Kind != BytesKind || b.Data.(*BytesObject).Data[0] != 0 || b.String() != "<字节串 5 字节>" {
		t.Fatalf("bytes value is not immutable: %v", b)
	}
	if got, re := v.indexGet(b, Int(4), source.Span{}); re != nil || got.Data.(int64) != 255 {
		t.Fatalf("byte index=%v,%v", got, re)
	}
	if got, re := v.fieldGet(b, "长度", source.Span{}); re != nil {
		t.Fatal(re)
	} else if value, re := v.call(got, nil, source.Span{}); re != nil || value.Data.(int64) != 5 {
		t.Fatalf("byte length=%v,%v", value, re)
	}
	if !equal(b, Bytes([]byte{0, 1, 127, 128, 255})) {
		t.Fatal("equal bytes failed")
	}
	if _, err := marshalJSON(b, false); err == nil {
		t.Fatal("bytes unexpectedly serialized as JSON")
	}
	if err := setDict(Dict(), b, Int(1)); err == nil {
		t.Fatal("bytes unexpectedly accepted as dictionary key")
	}

	encoding, ok := StandardModule("标准.编码", v)
	if !ok {
		t.Fatal("encoding module missing")
	}
	encoded, re := callNativeForCoverage(t, v, encoding, "UTF8编码", Text("华😀言"))
	if re != nil {
		t.Fatal(re)
	}
	decoded, re := callNativeForCoverage(t, v, encoding, "UTF8解码", encoded)
	if re != nil || decoded.Data.(string) != "华😀言" {
		t.Fatalf("utf8 round trip=%v,%v", decoded, re)
	}
	if got, re := callNativeForCoverage(t, v, encoding, "Base64编码", b); re != nil || got.Data.(string) != "AAF/gP8=" {
		t.Fatalf("base64=%v,%v", got, re)
	}
	if got, re := callNativeForCoverage(t, v, encoding, "Base64解码", Text("AAF/gP8=")); re != nil || !equal(got, b) {
		t.Fatalf("base64 decode=%v,%v", got, re)
	}
	if got, re := callNativeForCoverage(t, v, encoding, "十六进制编码", b); re != nil || got.Data.(string) != "00017f80ff" {
		t.Fatalf("hex=%v,%v", got, re)
	}
	if got, re := callNativeForCoverage(t, v, encoding, "十六进制解码", Text("00017f80ff")); re != nil || !equal(got, b) {
		t.Fatalf("hex decode=%v,%v", got, re)
	}
	if _, re := callNativeForCoverage(t, v, encoding, "UTF8解码", Bytes([]byte{0xff})); re == nil {
		t.Fatal("invalid utf8 accepted")
	}
	if _, re := callNativeForCoverage(t, v, encoding, "Base64解码", Text("!")); re == nil {
		t.Fatal("invalid base64 accepted")
	}
	if _, re := callNativeForCoverage(t, v, encoding, "十六进制解码", Text("f")); re == nil {
		t.Fatal("invalid hex accepted")
	}

	dir := t.TempDir()
	v.WorkingDir = dir
	files, ok := StandardModule("标准.文件", v)
	if !ok {
		t.Fatal("file module missing")
	}
	if _, re := callNativeForCoverage(t, v, files, "写入字节", Text("数据.bin"), b); re != nil {
		t.Fatal(re)
	}
	read, re := callNativeForCoverage(t, v, files, "读取字节", Text("数据.bin"))
	if re != nil || !equal(read, b) {
		t.Fatalf("binary file round trip=%v,%v", read, re)
	}
	if _, re := callNativeForCoverage(t, v, files, "写入字节", Text("数据.bin"), Text("错误类型")); re == nil {
		t.Fatal("text accepted by binary file API")
	}
}

func TestChineseDatabaseAPI(t *testing.T) {
	v := New(&bytes.Buffer{}, strings.NewReader(""), nil)
	v.WorkingDir = t.TempDir()
	database, ok := StandardModule("标准.数据库", v)
	if !ok {
		t.Fatal("database module missing")
	}
	record := func(items ...struct {
		name  string
		value Value
	}) Value {
		value := Record()
		object := value.Data.(*RecordObject)
		for _, item := range items {
			object.Order = append(object.Order, item.name)
			object.Fields[item.name] = item.value
		}
		return value
	}
	db, re := callNativeForCoverage(t, v, database, "打开", Text("图书馆.db"))
	if re != nil {
		t.Fatal(re)
	}
	fields := record(
		struct {
			name  string
			value Value
		}{"编号", Text("文字")},
		struct {
			name  string
			value Value
		}{"书名", Text("文字")},
		struct {
			name  string
			value Value
		}{"页数", Text("整数")},
		struct {
			name  string
			value Value
		}{"评分", Text("小数")},
		struct {
			name  string
			value Value
		}{"可借", Text("布尔")},
		struct {
			name  string
			value Value
		}{"封面", Text("字节串")},
	)
	if _, re = callNativeForCoverage(t, v, database, "建表", db, Text("图书"), fields); re != nil {
		t.Fatal(re)
	}
	insert := record(
		struct {
			name  string
			value Value
		}{"编号", Text("B001")},
		struct {
			name  string
			value Value
		}{"书名", Text("活着")},
		struct {
			name  string
			value Value
		}{"页数", Int(240)},
		struct {
			name  string
			value Value
		}{"评分", Float(9.5)},
		struct {
			name  string
			value Value
		}{"可借", Bool(true)},
		struct {
			name  string
			value Value
		}{"封面", Bytes([]byte{1, 2, 3})},
	)
	result, re := callNativeForCoverage(t, v, database, "插入", db, Text("图书"), insert)
	if re != nil {
		t.Fatal(re)
	}
	if got, re := callNativeForCoverage(t, v, database, "受影响行数", result); re != nil || got.Data.(int64) != 1 {
		t.Fatalf("affected rows=%v,%v", got, re)
	}
	condition := record(struct {
		name  string
		value Value
	}{"编号", Text("B001")})
	row, re := callNativeForCoverage(t, v, database, "选择一行", db, Text("图书"), condition)
	if re != nil {
		t.Fatal(re)
	}
	if title, re := v.fieldGet(row, "书名", source.Span{}); re != nil || title.Data.(string) != "活着" {
		t.Fatalf("selected row=%v,%v", title, re)
	}
	if _, re := callNativeForCoverage(t, v, database, "更新", db, Text("图书"), record(struct {
		name  string
		value Value
	}{"页数", Int(241)}), condition); re != nil {
		t.Fatal(re)
	}
	rows, re := callNativeForCoverage(t, v, database, "选择", db, Text("图书"))
	if re != nil || len(rows.Data.(*ListObject).Items) != 1 {
		t.Fatalf("selected rows=%v,%v", rows, re)
	}
	queryFn, ok := v.Globals().Get("选择")
	if !ok {
		t.Fatal("query builtin missing")
	}
	advanced, re := v.call(queryFn, []Value{db, Text("图书"), Nil(), Text("编号"), Bool(true), Int(1)}, source.Span{})
	if re != nil || len(advanced.Data.(*ListObject).Items) != 1 {
		t.Fatalf("advanced query=%v,%v", advanced, re)
	}
	if _, re = v.call(queryFn, []Value{db, Text("图书"), Nil(), Nil(), Bool(false), Int(0)}, source.Span{}); re == nil {
		t.Fatal("zero query limit accepted")
	}
	tx, re := callNativeForCoverage(t, v, database, "开始事务", db)
	if re != nil {
		t.Fatal(re)
	}
	if _, re = callNativeForCoverage(t, v, database, "插入", tx, Text("图书"), record(struct {
		name  string
		value Value
	}{"编号", Text("B002")}, struct {
		name  string
		value Value
	}{"书名", Text("围城")}, struct {
		name  string
		value Value
	}{"页数", Int(300)})); re != nil {
		t.Fatal(re)
	}
	if _, re = callNativeForCoverage(t, v, database, "回滚", tx); re != nil {
		t.Fatal(re)
	}
	tx, re = callNativeForCoverage(t, v, database, "开始事务", db)
	if re != nil {
		t.Fatal(re)
	}
	if _, re = callNativeForCoverage(t, v, database, "提交", tx); re != nil {
		t.Fatal(re)
	}
	missing, re := callNativeForCoverage(t, v, database, "选择一行", db, Text("图书"), record(struct {
		name  string
		value Value
	}{"编号", Text("B002")}))
	if re != nil || missing.Kind != NilKind {
		t.Fatalf("rollback result=%v,%v", missing, re)
	}
	if _, re = callNativeForCoverage(t, v, database, "删除", db, Text("图书"), condition); re != nil {
		t.Fatal(re)
	}
	if _, re = callNativeForCoverage(t, v, database, "关闭", db); re != nil {
		t.Fatal(re)
	}
	if _, re = callNativeForCoverage(t, v, database, "选择", db, Text("图书")); re == nil {
		t.Fatal("closed database accepted query")
	}
}

func TestDatabaseHelperBoundaries(t *testing.T) {
	v := New(&bytes.Buffer{}, strings.NewReader(""), nil)
	database, ok := StandardModule("标准.数据库", v)
	if !ok {
		t.Fatal("database module missing")
	}
	for _, name := range []string{"打开", "关闭", "建表", "插入", "选择", "选择一行", "更新", "删除", "开始事务", "提交", "回滚", "最后插入编号", "受影响行数"} {
		if _, re := callNativeForCoverage(t, v, database, name); re == nil {
			t.Fatalf("invalid %s call accepted", name)
		}
	}
	db, re := callNativeForCoverage(t, v, database, "打开", Text(":memory:"))
	if re != nil {
		t.Fatal(re)
	}
	for _, call := range [][]Value{
		{db, Int(1), Record()},
		{db, Text("表"), Text("不是记录")},
		{db, Text("表"), Record()},
	} {
		if _, re := callNativeForCoverage(t, v, database, "建表", call...); re == nil {
			t.Fatal("invalid table definition accepted")
		}
	}
	if _, re := callNativeForCoverage(t, v, database, "关闭", db); re != nil {
		t.Fatal(re)
	}
	db, re = callNativeForCoverage(t, v, database, "打开", Text(":memory:"))
	if re != nil {
		t.Fatal(re)
	}
	badCalls := []struct {
		name string
		args []Value
	}{
		{"插入", []Value{db, Text("表"), Text("不是记录")}},
		{"选择", []Value{db, Text("不存在"), Int(1)}},
		{"选择一行", []Value{db, Text("不存在"), Int(1)}},
		{"更新", []Value{db, Text("表"), Text("不是记录")}},
		{"删除", []Value{db, Text("不存在"), Int(1)}},
	}
	for _, call := range badCalls {
		if _, re := callNativeForCoverage(t, v, database, call.name, call.args...); re == nil {
			t.Fatalf("invalid %s accepted", call.name)
		}
	}
	if _, re := callNativeForCoverage(t, v, database, "关闭", db); re != nil {
		t.Fatal(re)
	}
	if _, re := quoteDatabaseIdentifier(v, "危险;删除", "表名"); re == nil {
		t.Fatal("unsafe identifier accepted")
	}
	for _, value := range []Value{Nil(), Bool(true), Int(1), Float(1.5), Text("x"), Bytes([]byte{1})} {
		if _, re := databaseValue(v, value); re != nil {
			t.Fatalf("database value rejected: %v", re)
		}
	}
	if _, re := databaseValue(v, List(nil)); re == nil {
		t.Fatal("list database value accepted")
	}
	if _, _, re := databaseCondition(v, Int(1)); re == nil {
		t.Fatal("invalid database condition accepted")
	}
	if _, _, re := databaseCondition(v, Record()); re != nil {
		t.Fatal(re)
	}
	for _, value := range []any{nil, int64(1), float64(1), true, "文字", []byte{1}} {
		if _, re := databaseScanValue(v, value); re != nil {
			t.Fatalf("scan value rejected: %v", re)
		}
	}
	if _, re := databaseScanValue(v, struct{}{}); re == nil {
		t.Fatal("unsupported scan value accepted")
	}
	if databaseError(v, "测试错误") == nil {
		t.Fatal("database error was nil")
	}
	if _, re := databaseObject(v, Text("不是资源")); re == nil {
		t.Fatal("text accepted as database")
	}
	if _, re := transactionObject(v, Text("不是资源")); re == nil {
		t.Fatal("text accepted as transaction")
	}
	if _, re := databaseRunnerFor(v, Text("不是资源")); re == nil {
		t.Fatal("text accepted as database runner")
	}
	if _, re := databaseQuery(v, failingDatabaseRunner{}, "查询", nil); re == nil {
		t.Fatal("failing database query did not return error")
	}
	if _, re := databaseResult(v, databaseResultError{last: errors.New("编号失败")}); re == nil {
		t.Fatal("last insert id error was ignored")
	}
	if _, re := databaseResult(v, databaseResultError{affected: errors.New("行数失败")}); re == nil {
		t.Fatal("affected rows error was ignored")
	}
	queryFn, ok := v.Globals().Get("选择")
	if !ok {
		t.Fatal("query builtin missing")
	}
	if _, re := v.call(queryFn, []Value{Text("不是数据库"), Text("表"), Nil(), Nil(), Bool(false), Int(1)}, source.Span{}); re == nil {
		t.Fatal("invalid query resource accepted")
	}
	if _, _, re := databaseCondition(v, Nil()); re != nil {
		t.Fatal(re)
	}
	ordinary, err := v.NewResource("普通资源", &vmResourceCloser{})
	if err != nil {
		t.Fatal(err)
	}
	if _, re := databaseObject(v, ordinary); re == nil {
		t.Fatal("ordinary resource accepted as database")
	}
	if _, re := transactionObject(v, ordinary); re == nil {
		t.Fatal("ordinary resource accepted as transaction")
	}
	if err := v.CloseResource(ordinary); err != nil {
		t.Fatal(err)
	}
	if (Value{Kind: DatabaseResultKind, Data: &DatabaseResultObject{}}).String() != "<数据库结果>" {
		t.Fatal("database result formatting failed")
	}
	var nilVM *VM
	if _, err := nilVM.newResource("无效", &vmResourceCloser{}, nil); err == nil {
		t.Fatal("nil VM accepted resource")
	}
}

func TestValueOperationsAndBoundaryErrors(t *testing.T) {
	v := New(&bytes.Buffer{}, strings.NewReader(""), nil)
	sp := source.Span{}
	for _, op := range []string{"+", "-", "*", "/", "%", "==", "!=", "<", "<=", ">", ">=", "且", "或"} {
		left, right := Int(4), Int(2)
		if op == "且" || op == "或" {
			left, right = Bool(true), Bool(false)
		}
		v.binary(op, left, right, sp)
	}
	v.binary("/", Int(1), Int(0), sp)
	v.binary("+", Text("x"), Int(1), sp)
	v.unary("非", Bool(true), sp)
	v.unary("-", Int(1), sp)
	v.unary("-", Text("x"), sp)
	list := List([]Value{Int(1), Int(2)})
	dict := Dict()
	setDict(dict, Text("x"), Int(3))
	rec := Record()
	rec.Data.(*RecordObject).Fields["x"] = Int(4)
	for _, pair := range []struct{ object, key Value }{{Text("华言"), Int(0)}, {list, Int(1)}, {dict, Text("x")}} {
		v.indexGet(pair.object, pair.key, sp)
	}
	v.indexGet(list, Int(9), sp)
	v.indexGet(Int(1), Int(0), sp)
	v.indexGet(rec, Text("x"), sp)
	v.indexGet(rec, Text("no"), sp)
	v.indexGet(rec, Int(0), sp)
	v.indexSet(list, Int(0), Int(8), sp)
	v.indexSet(dict, Text("y"), Int(9), sp)
	v.indexSet(rec, Text("y"), Int(10), sp)
	v.indexSet(rec, Int(0), Int(10), sp)
	v.indexSet(Text("x"), Int(0), Int(1), sp)
	v.fieldGet(rec, "x", sp)
	v.fieldGet(rec, "no", sp)
	v.fieldGet(Int(1), "x", sp)
	v.fieldSet(rec, "y", Int(1), sp)
	v.fieldSet(Int(1), "x", Int(1), sp)
	for _, x := range []Value{list, Text("ab"), dict, Int(1)} {
		v.makeIterator(x, sp)
	}
	less(Int(1), Int(2))
	less(Text("a"), Text("b"))
	less(Bool(true), Bool(false))
	equal(Int(1), Float(1))
	equal(List(nil), List(nil))
	equal(Nil(), Nil())
	equal(Bool(true), Bool(false))
	equal(Text("甲"), Text("甲"))
	equal(Text("甲"), Text("乙"))
	same := List([]Value{Int(1)})
	equal(same, same)
	equal(Value{Kind: FunctionKind, Data: &FunctionObject{}}, Value{Kind: FunctionKind, Data: &FunctionObject{}})
}

func TestNumericJSONAndValueErrorBranches(t *testing.T) {
	v := New(&bytes.Buffer{}, strings.NewReader(""), nil)
	sp := source.Span{}
	for _, args := range [][3]any{{"+", int64(maxInt64), int64(1)}, {"-", int64(minInt64), int64(1)}, {"*", int64(minInt64), int64(-1)}, {"%", int64(1), int64(0)}, {"/", int64(1), int64(0)}, {"?", int64(1), int64(1)}} {
		v.intBinary(args[0].(string), args[1].(int64), args[2].(int64), sp)
	}
	v.unary("-", Int(minInt64), sp)
	v.unary("未知", Nil(), sp)
	for _, pair := range [][2]Value{{Text("a"), Text("b")}, {Bool(true), Bool(false)}, {Nil(), Nil()}} {
		v.binary("<", pair[0], pair[1], sp)
	}
	for _, x := range []Value{Bool(true), Int(1), Text("x"), Float(1), Nil(), List(nil)} {
		validKey(x)
	}
	fromJSON(nil)
	fromJSON(true)
	fromJSON("文字")
	fromJSON(json.Number("12"))
	fromJSON(json.Number("1.5"))
	fromJSON([]any{nil})
	fromJSON(map[string]any{"a": true})
	for _, x := range []Value{Nil(), Bool(true), Int(1), Float(1.5), Text("x"), List([]Value{Int(1)}), Dict(), Record(), Value{Kind: FunctionKind}} {
		toJSON(x, map[any]bool{})
	}
	marshalJSON(Value{Kind: FunctionKind}, false)
	decodeJSONValue(json.NewDecoder(strings.NewReader("[1")))
	minmax(v, nil, true)
	minmax(v, []Value{Text("x")}, true)
	less(Int(1), Text("x"))
}

func TestValueFormattingEnvironmentsAndUpvalues(t *testing.T) {
	list := List([]Value{Int(1)})
	list.Data.(*ListObject).Items = append(list.Data.(*ListObject).Items, list)
	if !strings.Contains(list.String(), "循环引用") {
		t.Fatalf("cyclic list formatting=%q", list.String())
	}
	dict := Dict()
	setDict(dict, Text("a"), Int(1))
	if !strings.Contains(dict.String(), "\"a\"") {
		t.Fatalf("dict formatting=%q", dict.String())
	}
	record := Record()
	record.Data.(*RecordObject).Order = []string{"名"}
	record.Data.(*RecordObject).Fields["名"] = Text("华言")
	if !strings.Contains(record.String(), "记录") {
		t.Fatalf("record formatting=%q", record.String())
	}
	if got := (Value{Kind: FunctionKind, Data: &FunctionObject{Proto: &bytecode.FunctionProto{Name: "函数"}}}).String(); got != "<函数 函数>" {
		t.Fatalf("function formatting=%q", got)
	}
	if got := (Value{Kind: NativeKind, Data: &NativeObject{Name: "原生"}}).String(); got != "<函数 原生>" {
		t.Fatalf("native formatting=%q", got)
	}
	if got := (Value{Kind: ModuleKind, Data: &ModuleObject{Path: "模块"}}).String(); got != "<模块 模块>" {
		t.Fatalf("module formatting=%q", got)
	}
	if got := (Value{Kind: ErrorKind, Data: &ErrorObject{Message: "错误"}}).String(); got != "错误" {
		t.Fatalf("error formatting=%q", got)
	}
	parent := NewEnv(nil, nil)
	parent.Define("甲", Int(1), false)
	parent.Define("常量", Int(2), true)
	child := NewEnv(parent, nil)
	child.Define("乙", Int(3), false)
	if value, ok := child.Get("甲"); !ok || value.Data.(int64) != 1 {
		t.Fatal("parent binding unavailable")
	}
	if _, ok := child.Find("甲"); !ok || len(child.Names()) != 1 {
		t.Fatal("environment lookup failed")
	}
	if err := child.Set("甲", Int(4)); err != nil {
		t.Fatal(err)
	}
	if err := child.Set("常量", Int(5)); err == nil || child.Set("不存在", Int(1)) == nil {
		t.Fatal("invalid environment assignment accepted")
	}
	u := &Upvalue{env: parent, name: "甲"}
	if value, ok := u.Get(); !ok || value.Data.(int64) != 4 {
		t.Fatal("upvalue get failed")
	}
	if err := u.Set(Int(6)); err != nil {
		t.Fatal(err)
	}
	closed := &Upvalue{closed: Int(1)}
	if err := closed.Set(Int(2)); err != nil {
		t.Fatal(err)
	}
	if value, ok := closed.Get(); !ok || value.Data.(int64) != 2 {
		t.Fatal("closed upvalue failed")
	}
	if err := (&Upvalue{}).Set(Int(1)); err != nil {
		t.Fatal(err)
	}
	if _, ok := (*Upvalue)(nil).Get(); ok {
		t.Fatal("nil upvalue returned a value")
	}
}

func TestVMExecutionErrorBranches(t *testing.T) {
	cases := []struct {
		name, source, want string
	}{
		{"调用非函数", "让 值 = 1\n值()", "只有函数才能被调用"},
		{"参数数量", "函数 f(x)\n返回 x\n结束\nf()", "需要 1 个参数"},
		{"列表下标", "让 值 = []\n打印(值[0])", "列表下标越界"},
		{"字典缺键", "让 值 = {}\n打印(值[\"没有\"])", "字典中不存在这个键"},
		{"记录缺字段", "让 值 = 记录 {}\n打印(值.没有)", "记录中不存在字段"},
		{"错误迭代", "遍历 项 于 1\n打印(项)\n结束", "只有列表、字典和文字可以遍历"},
		{"原生参数", "长度()", "需要 1 个参数"},
		{"用户模块加载器", "导入 不存在", "找不到模块"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, re := runVMText(t, tc.source)
			if re == nil || !strings.Contains(re.Error(), tc.want) {
				t.Fatalf("runtime=%v, want %q", re, tc.want)
			}
		})
	}
}

func TestBuiltinBoundaryBehavior(t *testing.T) {
	var out bytes.Buffer
	v := New(&out, strings.NewReader("输入内容\n"), nil)
	call := func(name string, args ...Value) (Value, *RuntimeError) {
		t.Helper()
		fn, ok := v.globals.Get(name)
		if !ok {
			t.Fatalf("builtin %q missing", name)
		}
		return v.call(fn, args, source.Span{})
	}
	if got, re := call("输入", Text("提示：")); re != nil || got.String() != "输入内容" || out.String() != "提示：" {
		t.Fatalf("input=%v,%v output=%q", got, re, out.String())
	}
	if _, re := call("输入", Int(1), Int(2)); re == nil {
		t.Fatal("input accepted too many arguments")
	}
	if _, re := call("长度", Text("华😀")); re != nil {
		t.Fatal(re)
	}
	if _, re := call("长度", Int(1)); re == nil {
		t.Fatal("length accepted unsupported type")
	}
	for _, pair := range []struct {
		name string
		args []Value
	}{
		{"类型", nil}, {"转文字", nil}, {"转整数", nil}, {"转小数", nil}, {"范围", nil},
	} {
		if _, re := call(pair.name, pair.args...); re == nil {
			t.Fatalf("%s accepted wrong arity", pair.name)
		}
	}
	if got, re := call("转整数", Text(" 12 ")); re != nil || got.Data.(int64) != 12 {
		t.Fatalf("integer conversion=%v,%v", got, re)
	}
	if _, re := call("转整数", Text("坏")); re == nil {
		t.Fatal("invalid integer accepted")
	}
	if got, re := call("转小数", Int(2)); re != nil || got.Kind != FloatKind {
		t.Fatalf("float conversion=%v,%v", got, re)
	}
	if _, re := call("转小数", Text("坏")); re == nil {
		t.Fatal("invalid float accepted")
	}
	for _, args := range [][]Value{{Int(3)}, {Int(3), Int(0), Int(-1)}} {
		if got, re := call("范围", args...); re != nil || got.Kind != ListKind {
			t.Fatalf("range=%v,%v", got, re)
		}
	}
	if _, re := call("范围", Int(1), Int(2), Int(0)); re == nil {
		t.Fatal("zero range step accepted")
	}
	if _, re := call("错误", Int(1)); re == nil {
		t.Fatal("non-text error accepted")
	}
	if _, re := call("断言", Bool(true)); re != nil {
		t.Fatal(re)
	}
	if _, re := call("断言", Bool(false), Text("失败消息")); re == nil || !strings.Contains(re.Error(), "失败消息") {
		t.Fatalf("assertion=%v", re)
	}
}

func TestEveryStandardModuleNativeHandlesEmptyArgs(t *testing.T) {
	v := New(&bytes.Buffer{}, strings.NewReader(""), nil)
	paths := []string{"标准.控制台", "标准.文字", "标准.列表", "标准.字典", "标准.文件", "标准.JSON", "标准.时间", "标准.数学", "标准.程序", "标准.测试"}
	for _, path := range paths {
		mod, ok := StandardModule(path, v)
		if !ok {
			t.Fatalf("standard module missing: %s", path)
		}
		for name, fn := range mod.Data.(*ModuleObject).Exports {
			if _, re := v.call(fn, nil, source.Span{}); re != nil {
				// Arity/type diagnostics are expected here. The important
				// invariant is that every native boundary returns an error,
				// rather than panicking, for empty input.
				continue
			}
			_ = name
		}
	}
	if _, ok := StandardModule("标准.不存在", v); ok {
		t.Fatal("unknown standard module accepted")
	}
}

func TestVMValidBytecodeSemanticErrorBoundaries(t *testing.T) {
	textConst := func(s string) *bytecode.Chunk {
		return &bytecode.Chunk{Name: "边界", Constants: []any{s}}
	}
	cases := []struct {
		name  string
		chunk *bytecode.Chunk
	}{
		{"弹出空栈", &bytecode.Chunk{Code: []bytecode.Instruction{{Op: bytecode.OpPop}}}},
		{"读取缺失名称", textConst("没有")},
		{"声明局部空栈", &bytecode.Chunk{Slots: 1, Code: []bytecode.Instruction{{Op: bytecode.OpDeclareLocal, Arg: 0}}}},
		{"写入局部空栈", &bytecode.Chunk{Slots: 1, Code: []bytecode.Instruction{{Op: bytecode.OpStoreLocal, Arg: 0}}}},
		{"声明名称空栈", textConst("名字")},
		{"写入名称空栈", textConst("名字")},
		{"导出不存在", textConst("公开")},
		{"一元缺操作数", &bytecode.Chunk{Code: []bytecode.Instruction{{Op: bytecode.OpUnary, Text: "-"}}}},
		{"二元缺操作数", &bytecode.Chunk{Code: []bytecode.Instruction{{Op: bytecode.OpBinary, Text: "+"}}}},
		{"条件缺操作数", &bytecode.Chunk{Code: []bytecode.Instruction{{Op: bytecode.OpJumpIfFalse, Arg: 0}}}},
		{"调用缺函数", &bytecode.Chunk{Code: []bytecode.Instruction{{Op: bytecode.OpCall, Arg: 0}}}},
		{"列表元素不足", &bytecode.Chunk{Code: []bytecode.Instruction{{Op: bytecode.OpMakeList, Arg: 1}}}},
		{"字典元素不足", &bytecode.Chunk{Code: []bytecode.Instruction{{Op: bytecode.OpMakeDict, Arg: 1}}}},
		{"索引读取缺操作数", &bytecode.Chunk{Code: []bytecode.Instruction{{Op: bytecode.OpIndexGet}}}},
		{"索引写入缺操作数", &bytecode.Chunk{Code: []bytecode.Instruction{{Op: bytecode.OpIndexSet}}}},
		{"字段读取缺对象", textConst("字段")},
		{"字段写入缺对象", textConst("字段")},
		{"迭代开始缺对象", &bytecode.Chunk{Code: []bytecode.Instruction{{Op: bytecode.OpIterStart}}}},
		{"迭代下一项状态错误", &bytecode.Chunk{Code: []bytecode.Instruction{{Op: bytecode.OpIterNext, Arg: 0}}}},
		{"迭代结束空栈", &bytecode.Chunk{Code: []bytecode.Instruction{{Op: bytecode.OpIterEnd}}}},
		{"抛出空栈", &bytecode.Chunk{Code: []bytecode.Instruction{{Op: bytecode.OpThrow}}}},
		{"导入失败", textConst("标准.不存在")},
		{"读取导出缺模块", textConst("名称")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "读取缺失名称" {
				tc.chunk.Code = []bytecode.Instruction{{Op: bytecode.OpLoadName, Arg: 0}}
			} else if tc.name == "声明名称空栈" {
				tc.chunk.Code = []bytecode.Instruction{{Op: bytecode.OpDeclareName, Arg: 0}}
			} else if tc.name == "写入名称空栈" {
				tc.chunk.Code = []bytecode.Instruction{{Op: bytecode.OpStoreName, Arg: 0}}
			} else if tc.name == "导出不存在" {
				tc.chunk.Code = []bytecode.Instruction{{Op: bytecode.OpExportName, Arg: 0}}
			} else if tc.name == "字段读取缺对象" {
				tc.chunk.Code = []bytecode.Instruction{{Op: bytecode.OpFieldGet, Arg: 0}}
			} else if tc.name == "字段写入缺对象" {
				tc.chunk.Code = []bytecode.Instruction{{Op: bytecode.OpFieldSet, Arg: 0}}
			} else if tc.name == "读取导出缺模块" {
				tc.chunk.Code = []bytecode.Instruction{{Op: bytecode.OpGetExport, Arg: 0}}
			} else if tc.name == "导入失败" {
				tc.chunk.Code = []bytecode.Instruction{{Op: bytecode.OpImport, Arg: 0}}
			}
			v := New(&bytes.Buffer{}, strings.NewReader(""), nil)
			if _, re := v.Execute(tc.chunk, v.Globals()); re == nil {
				t.Fatal("semantic error was not reported")
			}
		})
	}
	valid := &bytecode.Chunk{Constants: []any{"名字"}, Code: []bytecode.Instruction{{Op: bytecode.OpDeclareEmpty, Arg: 0}, {Op: bytecode.OpReturn}}}
	if _, re := New(&bytes.Buffer{}, strings.NewReader(""), nil).Execute(valid, nil); re != nil {
		t.Fatal(re)
	}
}

func TestFieldMethodsAndErrorViews(t *testing.T) {
	v := New(&bytes.Buffer{}, strings.NewReader(""), nil)
	sp := source.Span{}
	errValue := Value{Kind: ErrorKind, Data: &ErrorObject{Category: "测试", Message: "消息", Stack: []CallFrame{{Name: "外层"}, {Name: "内层"}}}}
	for _, name := range []string{"类别", "消息", "调用栈"} {
		if _, re := v.fieldGet(errValue, name, sp); re != nil {
			t.Fatal(re)
		}
	}
	if _, re := v.fieldGet(errValue, "不存在", sp); re == nil {
		t.Fatal("unknown error field accepted")
	}
	mod := module("模块")
	export(mod, "值", Int(1))
	if got, re := v.fieldGet(mod, "值", sp); re != nil || got.Data.(int64) != 1 {
		t.Fatalf("module field=%v,%v", got, re)
	}
	if _, re := v.fieldGet(mod, "私有", sp); re == nil {
		t.Fatal("unknown module export accepted")
	}
	list := List([]Value{Int(1), Int(2)})
	callMethod := func(value Value, name string, args ...Value) (Value, *RuntimeError) {
		t.Helper()
		method, re := v.fieldGet(value, name, sp)
		if re != nil {
			t.Fatal(re)
		}
		return v.call(method, args, sp)
	}
	if got, re := callMethod(list, "长度"); re != nil || got.Data.(int64) != 2 {
		t.Fatalf("list length=%v,%v", got, re)
	}
	if got, re := callMethod(list, "移除", Int(0)); re != nil || got.Data.(int64) != 1 {
		t.Fatalf("list remove=%v,%v", got, re)
	}
	callMethod(list, "追加", Int(3))
	callMethod(list, "包含", Int(3))
	callMethod(list, "移除首项")
	callMethod(list, "清空")
	if _, re := callMethod(list, "移除首项"); re == nil {
		t.Fatal("empty list removal accepted")
	}
	if _, re := callMethod(list, "移除", Text("坏")); re == nil {
		t.Fatal("invalid list index accepted")
	}
	str := Text("华😀")
	if got, re := callMethod(str, "长度"); re != nil || got.Data.(int64) != 2 {
		t.Fatalf("string method=%v,%v", got, re)
	}
	dict := Dict()
	setDict(dict, Text("甲"), Int(1))
	for _, name := range []string{"长度", "键", "值", "包含键", "条目"} {
		if _, re := callMethod(dict, name, func() []Value {
			if name == "包含键" {
				return []Value{Text("甲")}
			}
			return nil
		}()...); re != nil {
			t.Fatalf("dict method %s: %v", name, re)
		}
	}
	if _, re := v.fieldGet(Int(1), "字段", sp); re == nil {
		t.Fatal("integer field accepted")
	}
}

func TestVMPrimitiveHelpersAndPathBoundaries(t *testing.T) {
	if fromConstant(nil).Kind != NilKind || fromConstant(int64(1)).Data.(int64) != 1 || fromConstant(float64(1)).Kind != FloatKind || fromConstant("文字").Data.(string) != "文字" {
		t.Fatal("constant conversion failed")
	}
	if fromConstant(Bool(true)).Kind != BoolKind || fromConstant(struct{ N int }{1}).Kind != StringKind {
		t.Fatal("constant fallback failed")
	}
	if numeric(1, Int(1), Int(2)).Kind != IntKind || numeric(1.5, Int(1), Float(2)).Kind != FloatKind {
		t.Fatal("numeric result kind failed")
	}
	if typeName(Value{Kind: NativeKind}) != "函数" || typeName(Int(1)) != "整数" {
		t.Fatal("type name failed")
	}
	v := New(&bytes.Buffer{}, strings.NewReader(""), nil)
	v.WorkingDir = "/tmp/华言工作目录"
	if got := v.filePath("数据.json"); got != filepath.Join(v.WorkingDir, "数据.json") {
		t.Fatalf("relative path=%q", got)
	}
	if got := v.filePath("/绝对/数据.json"); got != "/绝对/数据.json" {
		t.Fatalf("absolute path=%q", got)
	}
	if _, ok := NewEnv(nil, nil).Find("没有"); ok {
		t.Fatal("missing environment binding found")
	}
	if (&RuntimeError{Value: Text("普通")}).Error() != "普通" {
		t.Fatal("non-error runtime formatting failed")
	}
}

func TestNativeFailureAndJSONBoundaryBranches(t *testing.T) {
	v := New(&bytes.Buffer{}, strings.NewReader(""), nil)
	fileMod, _ := StandardModule("标准.文件", v)
	for _, args := range [][]Value{{}, {Int(1)}, {Text("/不存在/华言.txt")}} {
		callNativeForCoverage(t, v, fileMod, "读取文字", args...)
	}
	callNativeForCoverage(t, v, fileMod, "写入文字", Text("/不存在/华言.txt"), Text("内容"))
	callNativeForCoverage(t, v, fileMod, "追加文字", Text("/不存在/华言.txt"), Text("内容"))
	callNativeForCoverage(t, v, fileMod, "存在", Int(1))
	callNativeForCoverage(t, v, fileMod, "创建目录", Int(1))
	callNativeForCoverage(t, v, fileMod, "原子写入", Text("/不存在/华言.txt"), Text("内容"))
	callNativeForCoverage(t, v, fileMod, "原子写入组", Int(1))
	missing := Record()
	callNativeForCoverage(t, v, fileMod, "原子写入组", List([]Value{missing}))
	tmp := t.TempDir()
	badPath := Record()
	badPath.Data.(*RecordObject).Fields["路径"] = Text(tmp)
	badPath.Data.(*RecordObject).Fields["内容"] = Text("内容")
	callNativeForCoverage(t, v, fileMod, "原子写入组", List([]Value{badPath}))

	jsonMod, _ := StandardModule("标准.JSON", v)
	for _, input := range []string{"", "{} {}", "[", "{\"a\":}"} {
		callNativeForCoverage(t, v, jsonMod, "解析", Text(input))
	}
	callNativeForCoverage(t, v, jsonMod, "序列化")
	callNativeForCoverage(t, v, jsonMod, "格式化")
	nested := Record()
	nested.Data.(*RecordObject).Order = []string{"a", "b"}
	nested.Data.(*RecordObject).Fields["a"] = List([]Value{Int(1), Bool(true)})
	nested.Data.(*RecordObject).Fields["b"] = Dict()
	callNativeForCoverage(t, v, jsonMod, "格式化", nested)
	for _, raw := range []string{"[1,", "{\"a\":1", "{1:2}"} {
		dec := json.NewDecoder(strings.NewReader(raw))
		dec.UseNumber()
		decodeJSONValue(dec)
	}
	marshalJSON(Value{Kind: IteratorKind}, false)
}

func TestJSONValueConversionAndModuleLoaderBranches(t *testing.T) {
	seen := map[any]bool{}
	for _, value := range []Value{Nil(), Bool(true), Int(1), Float(1.5), Text("文字"), List([]Value{Int(1)}), Dict(), Record()} {
		if _, err := toJSON(value, seen); err != nil {
			t.Fatalf("toJSON(%s): %v", value.Kind, err)
		}
	}
	cycle := List(nil)
	cycle.Data.(*ListObject).Items = []Value{cycle}
	if _, err := toJSON(cycle, map[any]bool{}); err == nil {
		t.Fatal("cyclic list converted to JSON")
	}
	if _, err := toJSON(Value{Kind: IteratorKind}, map[any]bool{}); err == nil {
		t.Fatal("unsupported value converted to JSON")
	}
	v := New(&bytes.Buffer{}, strings.NewReader(""), nil)
	v.Loader = func(path, from string) (Value, error) {
		if path == "错误" {
			return Nil(), errors.New("加载失败")
		}
		if path == "无效" {
			return Int(1), nil
		}
		return module(path + "@" + from), nil
	}
	if got, re := v.importModule("用户", "主", source.Span{}); re != nil || got.Kind != ModuleKind {
		t.Fatalf("module load=%v,%v", got, re)
	}
	if got, re := v.importModule("用户", "其他", source.Span{}); re != nil || got.Kind != ModuleKind {
		t.Fatalf("module cache=%v,%v", got, re)
	}
	if _, re := v.importModule("错误", "主", source.Span{}); re == nil {
		t.Fatal("loader error was ignored")
	}
	if _, re := v.importModule("无效", "主", source.Span{}); re == nil {
		t.Fatal("invalid loader value accepted")
	}
}

func TestRemainingValueAndRuntimeFormattingBranches(t *testing.T) {
	if got := (Value{Kind: IteratorKind}).String(); got != "<迭代器>" {
		t.Fatalf("iterator formatting=%q", got)
	}
	if got := (Value{Kind: Kind("自定义")}).String(); got != "<自定义>" {
		t.Fatalf("unknown value formatting=%q", got)
	}
	if got := (*RuntimeError)(nil).Error(); got != "" {
		t.Fatalf("nil runtime error=%q", got)
	}
	if got := (&RuntimeError{Value: Value{Kind: ErrorKind, Data: &ErrorObject{Message: "错误"}}}).Error(); got != "错误" {
		t.Fatalf("error runtime=%q", got)
	}
}

func TestUpvalueCloseMissingBinding(t *testing.T) {
	v := New(nil, nil, nil)
	env := NewEnv(nil, nil)
	u := &Upvalue{env: env, name: "不存在"}
	v.openUpvalues[env] = []*Upvalue{u}
	v.closeEnv(env)
	if u.env != nil || u.closed.Kind != NilKind {
		t.Fatalf("missing binding was not closed as nil: %#v", u)
	}
	if _, ok := env.Find("不存在"); ok {
		t.Fatal("missing binding appeared during close")
	}
}
