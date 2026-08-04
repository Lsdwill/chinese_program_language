package vm

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"huayan/internal/bytecode"
	"huayan/internal/native"
	"huayan/internal/resource"
	"huayan/internal/source"
)

type Kind string

const (
	NilKind      Kind = "空"
	BoolKind     Kind = "布尔"
	IntKind      Kind = "整数"
	FloatKind    Kind = "小数"
	StringKind   Kind = "文字"
	BytesKind    Kind = "字节串"
	ListKind     Kind = "列表"
	DictKind     Kind = "字典"
	RecordKind   Kind = "记录"
	FunctionKind Kind = "函数"
	NativeKind   Kind = "原生函数"
	ModuleKind   Kind = "模块"
	ErrorKind    Kind = "错误"
	IteratorKind Kind = "迭代器"
	ResourceKind Kind = "资源"
)

type Value struct {
	Kind Kind
	Data any
}

func Nil() Value            { return Value{Kind: NilKind} }
func Bool(v bool) Value     { return Value{Kind: BoolKind, Data: v} }
func Int(v int64) Value     { return Value{Kind: IntKind, Data: v} }
func Float(v float64) Value { return Value{Kind: FloatKind, Data: v} }
func Text(v string) Value   { return Value{Kind: StringKind, Data: v} }
func Bytes(v []byte) Value {
	return Value{Kind: BytesKind, Data: &BytesObject{Data: append([]byte(nil), v...)}}
}
func List(v []Value) Value { return Value{Kind: ListKind, Data: &ListObject{Items: v}} }
func Dict() Value {
	return Value{Kind: DictKind, Data: &DictObject{Values: map[string]Value{}, Keys: map[string]Value{}}}
}
func Record() Value { return Value{Kind: RecordKind, Data: &RecordObject{Fields: map[string]Value{}}} }
func recordValues(fields map[string]Value) Value {
	r := Record()
	o := r.Data.(*RecordObject)
	for _, name := range []string{"键", "值"} {
		if value, ok := fields[name]; ok {
			o.Order = append(o.Order, name)
			o.Fields[name] = value
		}
	}
	return r
}
func (v Value) String() string { return formatValue(v, map[any]bool{}) }
func formatValue(v Value, seen map[any]bool) string {
	switch v.Kind {
	case NilKind:
		return "空"
	case BoolKind:
		if v.Data.(bool) {
			return "真"
		}
		return "假"
	case IntKind:
		return strconv.FormatInt(v.Data.(int64), 10)
	case FloatKind:
		return strconv.FormatFloat(v.Data.(float64), 'g', -1, 64)
	case StringKind:
		return v.Data.(string)
	case BytesKind:
		o := v.Data.(*BytesObject)
		return fmt.Sprintf("<字节串 %d 字节>", len(o.Data))
	case ListKind:
		o := v.Data.(*ListObject)
		if seen[o] {
			return "[循环引用]"
		}
		seen[o] = true
		defer delete(seen, o)
		var b strings.Builder
		b.WriteByte('[')
		for i, x := range o.Items {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(formatValue(x, seen))
		}
		b.WriteByte(']')
		return b.String()
	case DictKind:
		o := v.Data.(*DictObject)
		if seen[o] {
			return "{循环引用}"
		}
		seen[o] = true
		defer delete(seen, o)
		var b strings.Builder
		b.WriteByte('{')
		for i, k := range o.Order {
			if i > 0 {
				b.WriteString(", ")
			}
			key := k
			if original, ok := o.Keys[k]; ok {
				if original.Kind == StringKind {
					key = original.Data.(string)
				} else {
					key = original.String()
				}
			}
			b.WriteString(strconv.Quote(key))
			b.WriteString(": ")
			b.WriteString(formatValue(o.Values[k], seen))
		}
		b.WriteByte('}')
		return b.String()
	case RecordKind:
		o := v.Data.(*RecordObject)
		if seen[o] {
			return "记录 {循环引用}"
		}
		seen[o] = true
		defer delete(seen, o)
		var b strings.Builder
		b.WriteString("记录 {")
		for i, k := range o.Order {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(formatValue(o.Fields[k], seen))
		}
		b.WriteByte('}')
		return b.String()
	case FunctionKind:
		f := v.Data.(*FunctionObject)
		return "<函数 " + f.Proto.Name + ">"
	case NativeKind:
		n := v.Data.(*NativeObject)
		return "<函数 " + n.Name + ">"
	case ModuleKind:
		m := v.Data.(*ModuleObject)
		return "<模块 " + m.Path + ">"
	case ErrorKind:
		e := v.Data.(*ErrorObject)
		return e.Message
	case ResourceKind:
		r := v.Data.(*ResourceObject)
		return "<资源 " + r.Name + ">"
	default:
		return "<" + string(v.Kind) + ">"
	}
}

type ListObject struct{ Items []Value }
type BytesObject struct{ Data []byte }
type DictObject struct {
	Values map[string]Value
	Keys   map[string]Value
	Order  []string
}
type RecordObject struct {
	Fields map[string]Value
	Order  []string
}
type ModuleObject struct {
	Path    string
	Exports map[string]Value
}
type ResourceObject struct {
	Handle resource.Handle
	Name   string
}
type ErrorObject struct {
	Category, Message string
	Span              source.Span
	Stack             []CallFrame
	ExitCode          *int64
}
type FunctionObject struct {
	Proto    *bytecode.FunctionProto
	Env      *Env
	Upvalues []*Upvalue
}
type Upvalue struct {
	env      *Env
	name     string
	closed   Value
	constant bool
}

func (u *Upvalue) Get() (Value, bool) {
	if u == nil {
		return Nil(), false
	}
	if u.env == nil {
		return u.closed, true
	}
	b, ok := u.env.Values[u.name]
	return b.Value, ok
}
func (u *Upvalue) Set(v Value) error {
	if u == nil {
		return errors.New("上值不存在")
	}
	if u.env == nil {
		if u.constant {
			return errors.New("常量不能重新赋值")
		}
		u.closed = v
		return nil
	}
	b, ok := u.env.Values[u.name]
	if !ok {
		return errors.New("上值“" + u.name + "”尚未声明")
	}
	if b.Constant {
		return errors.New("常量“" + u.name + "”不能重新赋值")
	}
	u.env.Values[u.name] = Binding{Value: v, Constant: b.Constant}
	return nil
}

func (v *VM) closeEnv(env *Env) {
	if env == nil {
		return
	}
	for _, u := range v.openUpvalues[env] {
		if b, ok := env.Values[u.name]; ok {
			u.closed, u.constant = b.Value, b.Constant
		} else {
			u.closed = Nil()
		}
		u.env = nil
	}
	delete(v.openUpvalues, env)
}

type NativeFunc func(*VM, []Value) (Value, *RuntimeError)
type NativeObject struct {
	Name string
	Fn   NativeFunc
}
type IteratorObject struct {
	Values      []Value
	Index       int
	List        *ListObject
	InitialSize int
}
type Binding struct {
	Value    Value
	Constant bool
}
type Env struct {
	Values map[string]Binding
	Parent *Env
	Module *ModuleObject
}

func NewEnv(parent *Env, module *ModuleObject) *Env {
	return &Env{Values: map[string]Binding{}, Parent: parent, Module: module}
}
func (e *Env) Define(name string, v Value, constant bool) {
	e.Values[name] = Binding{Value: v, Constant: constant}
}
func (e *Env) Get(name string) (Value, bool) {
	for cur := e; cur != nil; cur = cur.Parent {
		if b, ok := cur.Values[name]; ok {
			return b.Value, true
		}
	}
	return Nil(), false
}
func (e *Env) Find(name string) (*Env, bool) {
	for cur := e; cur != nil; cur = cur.Parent {
		if _, ok := cur.Values[name]; ok {
			return cur, true
		}
	}
	return nil, false
}
func (e *Env) Names() map[string]bool {
	names := make(map[string]bool, len(e.Values))
	for name := range e.Values {
		names[name] = true
	}
	return names
}
func (e *Env) Set(name string, v Value) error {
	for cur := e; cur != nil; cur = cur.Parent {
		if b, ok := cur.Values[name]; ok {
			if b.Constant {
				return errors.New("常量“" + name + "”不能重新赋值")
			}
			cur.Values[name] = Binding{Value: v}
			return nil
		}
	}
	return errors.New("变量“" + name + "”尚未声明")
}

type CallFrame struct {
	Name string
	Span source.Span
}
type RuntimeError struct {
	Value Value
	Stack []CallFrame
}

func (e *RuntimeError) Error() string {
	if e == nil {
		return ""
	}
	if e.Value.Kind == ErrorKind {
		return e.Value.Data.(*ErrorObject).Message
	}
	return e.Value.String()
}

type ModuleLoader func(path, from string) (Value, error)
type Handler struct {
	Target, StackDepth int
	Env                *Env
}
type RegisteredTest struct {
	Name string
	Fn   Value
}

type VM struct {
	Out           io.Writer
	In            io.Reader
	Args          []string
	Loader        ModuleLoader
	NativeModules *native.Registry
	Resources     *resource.Manager
	modules       map[string]Value
	frames        []CallFrame
	reader        *bufio.Reader
	globals       *Env
	tests         []RegisteredTest
	MaxCallDepth  int
	MaxStackDepth int
	WorkingDir    string
	openUpvalues  map[*Env][]*Upvalue
}

func New(out io.Writer, in io.Reader, args []string) *VM {
	if out == nil {
		out = os.Stdout
	}
	if in == nil {
		in = os.Stdin
	}
	v := &VM{
		Out: out, In: in, Args: args,
		NativeModules: defaultNativeModules(),
		Resources:     resource.NewManager(),
		modules:       map[string]Value{},
		MaxCallDepth:  1024, MaxStackDepth: 1 << 20,
		WorkingDir: ".", openUpvalues: map[*Env][]*Upvalue{},
	}
	v.reader = bufio.NewReader(in)
	v.globals = NewEnv(nil, nil)
	v.installBuiltins()
	return v
}

func defaultNativeModules() *native.Registry {
	r := native.NewRegistry()
	descriptors := []native.Descriptor{
		{Name: "标准.控制台"},
		{Name: "标准.文字"},
		{Name: "标准.编码"},
		{Name: "标准.列表"},
		{Name: "标准.字典"},
		{Name: "标准.文件", Capabilities: []native.Capability{native.CapabilityFileRead, native.CapabilityFileWrite}},
		{Name: "标准.JSON"},
		{Name: "标准.时间"},
		{Name: "标准.数学"},
		{Name: "标准.程序", Capabilities: []native.Capability{native.CapabilityEnvRead}},
		{Name: "标准.测试"},
	}
	for _, descriptor := range descriptors {
		if err := r.Register(descriptor); err != nil {
			panic(err)
		}
	}
	return r
}

func (v *VM) NewResource(name string, closer io.Closer) (Value, error) {
	if v == nil || v.Resources == nil {
		return Nil(), resource.ErrInvalidHandle
	}
	handle, err := v.Resources.Register(name, closer)
	if err != nil {
		return Nil(), err
	}
	return Value{Kind: ResourceKind, Data: &ResourceObject{Handle: handle, Name: name}}, nil
}

func (v *VM) CloseResource(value Value) error {
	if value.Kind != ResourceKind {
		return resource.ErrInvalidHandle
	}
	object, ok := value.Data.(*ResourceObject)
	if !ok {
		return resource.ErrInvalidHandle
	}
	return v.Resources.Close(object.Handle)
}

func (v *VM) CloseResources() []error {
	if v == nil || v.Resources == nil {
		return nil
	}
	return v.Resources.CloseAll()
}
func (v *VM) Globals() *Env { return v.globals }
func (v *VM) Execute(ch *bytecode.Chunk, env *Env) (result Value, runErr *RuntimeError) {
	return v.execute(ch, env, nil, nil)
}

func (v *VM) execute(ch *bytecode.Chunk, env *Env, args []Value, upvalues []*Upvalue) (result Value, runErr *RuntimeError) {
	if ch == nil {
		return Nil(), v.fault("字节码错误", "不能执行空字节码块", source.Span{})
	}
	if err := ch.Validate(); err != nil {
		return Nil(), v.fault("字节码错误", err.Error(), source.Span{File: ch.File})
	}
	defer func() {
		if r := recover(); r != nil {
			result = Nil()
			runErr = v.fault("内部错误", fmt.Sprintf("解释器内部故障：%v", r), source.Span{File: ch.File})
		}
	}()
	if env == nil {
		env = v.globals
	}
	oldWorkingDir := v.WorkingDir
	if ch.File != nil && filepath.IsAbs(ch.File.Name) {
		v.WorkingDir = filepath.Dir(ch.File.Name)
	}
	defer func() { v.WorkingDir = oldWorkingDir }()
	locals := make([]Value, ch.Slots)
	localConstants := make([]bool, ch.Slots)
	for i, arg := range args {
		if i >= len(locals) {
			break
		}
		locals[i] = arg
		if name, ok := ch.SlotNames[i]; ok {
			env.Define(name, arg, false)
		}
	}
	v.frames = append(v.frames, CallFrame{Name: ch.Name, Span: source.Span{File: ch.File}})
	defer func() { v.frames = v.frames[:len(v.frames)-1] }()
	if args != nil {
		defer v.closeEnv(env)
	}
	stack := []Value{}
	ip := 0
	handlers := []Handler{}
	pop := func() (Value, bool) {
		if len(stack) == 0 {
			return Nil(), false
		}
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return x, true
	}
	for ip < len(ch.Code) {
		ins := ch.Code[ip]
		ip++
		var err *RuntimeError
		switch ins.Op {
		case bytecode.OpConstant:
			if ins.Arg < 0 || ins.Arg >= len(ch.Constants) {
				err = v.fault("字节码错误", "常量索引越界", ins.Span)
				break
			}
			stack = append(stack, fromConstant(ch.Constants[ins.Arg]))
		case bytecode.OpNil:
			stack = append(stack, Nil())
		case bytecode.OpTrue:
			stack = append(stack, Bool(true))
		case bytecode.OpFalse:
			stack = append(stack, Bool(false))
		case bytecode.OpPop:
			_, ok := pop()
			if !ok {
				err = v.fault("字节码错误", "操作数栈下溢", ins.Span)
			}
		case bytecode.OpLoadName:
			name := ch.Constants[ins.Arg].(string)
			x, ok := env.Get(name)
			if !ok {
				err = v.fault("名称错误", "变量“"+name+"”尚未声明", ins.Span)
			} else {
				stack = append(stack, x)
			}
		case bytecode.OpLoadLocal:
			if ins.Arg < 0 || ins.Arg >= len(locals) {
				err = v.fault("字节码错误", "局部槽位越界", ins.Span)
				break
			}
			if name, ok := ch.SlotNames[ins.Arg]; ok {
				if x, found := env.Get(name); found {
					locals[ins.Arg] = x
				}
			}
			stack = append(stack, locals[ins.Arg])
		case bytecode.OpDeclareLocal:
			x, ok := pop()
			if !ok {
				err = v.fault("字节码错误", "声明局部变量时栈为空", ins.Span)
				break
			}
			if ins.Arg < 0 || ins.Arg >= len(locals) {
				err = v.fault("字节码错误", "局部槽位越界", ins.Span)
				break
			}
			locals[ins.Arg] = x
			localConstants[ins.Arg] = ins.Constant
			env.Define(ins.Text, x, ins.Constant)
		case bytecode.OpStoreLocal:
			if len(stack) == 0 {
				err = v.fault("字节码错误", "写入局部变量时栈为空", ins.Span)
				break
			}
			if ins.Arg < 0 || ins.Arg >= len(locals) {
				err = v.fault("字节码错误", "局部槽位越界", ins.Span)
				break
			}
			if localConstants[ins.Arg] {
				err = v.fault("名称错误", "常量“"+ins.Text+"”不能重新赋值", ins.Span)
				break
			}
			locals[ins.Arg] = stack[len(stack)-1]
			if e := env.Set(ins.Text, locals[ins.Arg]); e != nil {
				err = v.fault("名称错误", e.Error(), ins.Span)
			}
		case bytecode.OpLoadUpvalue:
			if ins.Arg < 0 || ins.Arg >= len(upvalues) {
				err = v.fault("字节码错误", "上值索引越界", ins.Span)
				break
			}
			x, ok := upvalues[ins.Arg].Get()
			if !ok {
				err = v.fault("名称错误", "上值尚未声明", ins.Span)
			} else {
				stack = append(stack, x)
			}
		case bytecode.OpStoreUpvalue:
			if len(stack) == 0 {
				err = v.fault("字节码错误", "写入上值时栈为空", ins.Span)
				break
			}
			if ins.Arg < 0 || ins.Arg >= len(upvalues) {
				err = v.fault("字节码错误", "上值索引越界", ins.Span)
				break
			}
			if e := upvalues[ins.Arg].Set(stack[len(stack)-1]); e != nil {
				err = v.fault("名称错误", e.Error(), ins.Span)
			}
		case bytecode.OpDeclareName:
			x, ok := pop()
			if !ok {
				err = v.fault("字节码错误", "声明缺少初始值", ins.Span)
				break
			}
			name := ch.Constants[ins.Arg].(string)
			env.Define(name, x, ins.Text == "常量")
		case bytecode.OpDeclareEmpty:
			name := ch.Constants[ins.Arg].(string)
			env.Define(name, Nil(), false)
		case bytecode.OpStoreName:
			if len(stack) == 0 {
				err = v.fault("字节码错误", "写入名称时操作数栈为空", ins.Span)
				break
			}
			name := ch.Constants[ins.Arg].(string)
			if e := env.Set(name, stack[len(stack)-1]); e != nil {
				err = v.fault("名称错误", e.Error(), ins.Span)
			}
		case bytecode.OpExportName:
			name := ch.Constants[ins.Arg].(string)
			x, ok := env.Get(name)
			if !ok {
				err = v.fault("模块错误", "导出名称不存在："+name, ins.Span)
				break
			}
			if env.Module != nil {
				env.Module.Exports[name] = x
			}
		case bytecode.OpUnary:
			x, ok := pop()
			if !ok {
				err = v.fault("字节码错误", "一元运算缺少操作数", ins.Span)
				break
			}
			var y Value
			y, err = v.unary(ins.Text, x, ins.Span)
			if err == nil {
				stack = append(stack, y)
			}
		case bytecode.OpBinary:
			r, ok1 := pop()
			l, ok2 := pop()
			if !ok1 || !ok2 {
				err = v.fault("字节码错误", "二元运算缺少操作数", ins.Span)
				break
			}
			var y Value
			y, err = v.binary(ins.Text, l, r, ins.Span)
			if err == nil {
				stack = append(stack, y)
			}
		case bytecode.OpJump:
			if ins.Arg < 0 || ins.Arg > len(ch.Code) {
				err = v.fault("字节码错误", "跳转地址越界", ins.Span)
			} else {
				ip = ins.Arg
			}
		case bytecode.OpJumpIfFalse, bytecode.OpJumpIfTrue:
			x, ok := pop()
			if !ok {
				err = v.fault("字节码错误", "条件跳转缺少条件", ins.Span)
				break
			}
			b, ok := x.Data.(bool)
			if x.Kind != BoolKind || !ok {
				err = v.fault("类型错误", "条件必须是布尔值", ins.Span)
				break
			}
			if (ins.Op == bytecode.OpJumpIfFalse && !b) || (ins.Op == bytecode.OpJumpIfTrue && b) {
				if ins.Arg < 0 || ins.Arg > len(ch.Code) {
					err = v.fault("字节码错误", "跳转地址越界", ins.Span)
				} else {
					ip = ins.Arg
				}
			}
		case bytecode.OpCall:
			if ins.Arg < 0 || ins.Arg > len(stack)-1 {
				err = v.fault("字节码错误", "调用参数数量无效", ins.Span)
				break
			}
			args := make([]Value, ins.Arg)
			for i := ins.Arg - 1; i >= 0; i-- {
				args[i], _ = pop()
			}
			callee, ok := pop()
			if !ok {
				err = v.fault("字节码错误", "调用缺少函数", ins.Span)
				break
			}
			var y Value
			y, err = v.call(callee, args, ins.Span)
			if err == nil {
				stack = append(stack, y)
			}
		case bytecode.OpReturn:
			x, ok := pop()
			if !ok {
				x = Nil()
			}
			return x, nil
		case bytecode.OpMakeClosure:
			proto, ok := ch.Constants[ins.Arg].(*bytecode.FunctionProto)
			if !ok {
				err = v.fault("字节码错误", "闭包原型无效", ins.Span)
			} else {
				captures := make([]*Upvalue, len(proto.Upvalues))
				for i, name := range proto.Upvalues {
					capturedEnv, found := env.Find(name)
					if !found {
						err = v.fault("闭包错误", "无法捕获上值“"+name+"”", ins.Span)
						break
					}
					captures[i] = &Upvalue{env: capturedEnv, name: name}
					v.openUpvalues[capturedEnv] = append(v.openUpvalues[capturedEnv], captures[i])
				}
				if err == nil {
					stack = append(stack, Value{Kind: FunctionKind, Data: &FunctionObject{Proto: proto, Env: env, Upvalues: captures}})
				}
			}
		case bytecode.OpMakeList:
			if ins.Arg < 0 || ins.Arg > len(stack) {
				err = v.fault("字节码错误", "列表元素数量无效", ins.Span)
				break
			}
			items := append([]Value(nil), stack[len(stack)-ins.Arg:]...)
			stack = stack[:len(stack)-ins.Arg]
			stack = append(stack, List(items))
		case bytecode.OpMakeDict, bytecode.OpMakeRecord:
			if ins.Arg < 0 || 2*ins.Arg > len(stack) {
				err = v.fault("字节码错误", "集合元素数量无效", ins.Span)
				break
			}
			start := len(stack) - 2*ins.Arg
			var obj Value
			if ins.Op == bytecode.OpMakeRecord {
				obj = Record()
			} else {
				obj = Dict()
			}
			for i := 0; i < ins.Arg; i++ {
				key := stack[start+2*i]
				val := stack[start+2*i+1]
				if ins.Op == bytecode.OpMakeRecord {
					k, ok := key.Data.(string)
					if key.Kind != StringKind || !ok {
						err = v.fault("类型错误", "记录字段名必须是文字", ins.Span)
						break
					}
					o := obj.Data.(*RecordObject)
					if _, exists := o.Fields[k]; !exists {
						o.Order = append(o.Order, k)
					}
					o.Fields[k] = val
				} else if e := setDict(obj, key, val); e != nil {
					err = v.fault("类型错误", e.Error(), ins.Span)
					break
				}
			}
			if err == nil {
				stack = append(stack[:start], obj)
			}
		case bytecode.OpIndexGet:
			k, ok1 := pop()
			o, ok2 := pop()
			if !ok1 || !ok2 {
				err = v.fault("字节码错误", "读取索引缺少操作数", ins.Span)
				break
			}
			var y Value
			y, err = v.indexGet(o, k, ins.Span)
			if err == nil {
				stack = append(stack, y)
			}
		case bytecode.OpIndexSet:
			val, ok1 := pop()
			k, ok2 := pop()
			o, ok3 := pop()
			if !ok1 || !ok2 || !ok3 {
				err = v.fault("字节码错误", "写入索引缺少操作数", ins.Span)
				break
			}
			if e := v.indexSet(o, k, val, ins.Span); e != nil {
				err = e
			} else {
				stack = append(stack, val)
			}
		case bytecode.OpFieldGet:
			o, ok := pop()
			if !ok {
				err = v.fault("字节码错误", "读取字段缺少对象", ins.Span)
				break
			}
			name := ch.Constants[ins.Arg].(string)
			var y Value
			y, err = v.fieldGet(o, name, ins.Span)
			if err == nil {
				stack = append(stack, y)
			}
		case bytecode.OpFieldSet:
			val, ok1 := pop()
			o, ok2 := pop()
			if !ok1 || !ok2 {
				err = v.fault("字节码错误", "写入字段缺少对象或值", ins.Span)
				break
			}
			name := ch.Constants[ins.Arg].(string)
			if e := v.fieldSet(o, name, val, ins.Span); e != nil {
				err = e
			} else {
				stack = append(stack, val)
			}
		case bytecode.OpEnterScope:
			env = NewEnv(env, env.Module)
		case bytecode.OpExitScope:
			if env.Parent != nil {
				v.closeEnv(env)
				env = env.Parent
			}
		case bytecode.OpIterStart:
			x, ok := pop()
			if !ok {
				err = v.fault("字节码错误", "迭代缺少对象", ins.Span)
				break
			}
			it, e := v.makeIterator(x, ins.Span)
			if e != nil {
				err = e
			} else {
				stack = append(stack, it)
			}
		case bytecode.OpIterNext:
			if len(stack) == 0 || stack[len(stack)-1].Kind != IteratorKind {
				err = v.fault("类型错误", "迭代器状态无效", ins.Span)
				break
			}
			it := stack[len(stack)-1].Data.(*IteratorObject)
			if it.List != nil && len(it.List.Items) != it.InitialSize {
				err = v.fault("迭代错误", "遍历期间不能修改列表长度", ins.Span)
				break
			}
			if it.Index >= len(it.Values) {
				if ins.Arg < 0 || ins.Arg > len(ch.Code) {
					err = v.fault("字节码错误", "迭代结束地址无效", ins.Span)
				} else {
					ip = ins.Arg
				}
			} else {
				x := it.Values[it.Index]
				it.Index++
				stack = append(stack, x)
			}
		case bytecode.OpIterEnd:
			if _, ok := pop(); !ok {
				err = v.fault("字节码错误", "结束迭代时栈为空", ins.Span)
			}
		case bytecode.OpTry:
			handlers = append(handlers, Handler{Target: ins.Arg, StackDepth: len(stack), Env: env})
		case bytecode.OpEndTry:
			if len(handlers) > 0 {
				handlers = handlers[:len(handlers)-1]
			}
		case bytecode.OpThrow:
			x, ok := pop()
			if !ok {
				err = v.fault("错误错误", "抛出缺少错误对象", ins.Span)
				break
			}
			if x.Kind != ErrorKind {
				err = v.fault("类型错误", "只能抛出错误对象", ins.Span)
			} else {
				eo := x.Data.(*ErrorObject)
				if !eo.Span.Valid() {
					eo.Span = ins.Span
				}
				eo.Stack = append([]CallFrame(nil), v.frames...)
				err = &RuntimeError{Value: x, Stack: eo.Stack}
			}
		case bytecode.OpImport:
			path := ch.Constants[ins.Arg].(string)
			x, e := v.importModule(path, ch.Name, ins.Span)
			if e != nil {
				err = e
			} else {
				stack = append(stack, x)
			}
		case bytecode.OpGetExport:
			mod, ok := pop()
			if !ok || mod.Kind != ModuleKind {
				err = v.fault("模块错误", "读取导出需要模块对象", ins.Span)
				break
			}
			name := ch.Constants[ins.Arg].(string)
			x, ok := mod.Data.(*ModuleObject).Exports[name]
			if !ok {
				err = v.fault("模块错误", "模块没有公开名称“"+name+"”", ins.Span)
			} else {
				stack = append(stack, x)
			}
		}
		if v.MaxStackDepth > 0 && len(stack) > v.MaxStackDepth {
			err = v.fault("操作数栈错误", "操作数栈超过限制", ins.Span)
		}
		if err != nil {
			if len(handlers) > 0 {
				h := handlers[len(handlers)-1]
				handlers = handlers[:len(handlers)-1]
				if h.StackDepth < len(stack) {
					stack = stack[:h.StackDepth]
				}
				env = h.Env
				stack = append(stack, err.Value)
				ip = h.Target
				continue
			}
			return Nil(), err
		}
	}
	return Nil(), nil
}

func fromConstant(x any) Value {
	switch v := x.(type) {
	case nil:
		return Nil()
	case int64:
		return Int(v)
	case float64:
		return Float(v)
	case string:
		return Text(v)
	case Value:
		return v
	default:
		return Text(fmt.Sprint(v))
	}
}
func (v *VM) call(c Value, args []Value, sp source.Span) (Value, *RuntimeError) {
	switch c.Kind {
	case FunctionKind:
		if v.MaxCallDepth > 0 && len(v.frames) >= v.MaxCallDepth {
			return Nil(), v.fault("调用栈错误", "调用栈溢出", sp)
		}
		f := c.Data.(*FunctionObject)
		if len(args) != len(f.Proto.Params) {
			return Nil(), v.fault("调用错误", fmt.Sprintf("函数“%s”需要 %d 个参数，实际得到 %d 个", f.Proto.Name, len(f.Proto.Params), len(args)), sp)
		}
		env := NewEnv(f.Env, f.Env.Module)
		for i, n := range f.Proto.Params {
			env.Define(n, args[i], false)
		}
		return v.execute(f.Proto.Chunk, env, args, f.Upvalues)
	case NativeKind:
		return c.Data.(*NativeObject).Fn(v, args)
	default:
		return Nil(), v.fault("类型错误", "只有函数才能被调用", sp)
	}
}
func (v *VM) fault(cat, msg string, sp source.Span) *RuntimeError {
	e := &ErrorObject{Category: cat, Message: msg, Span: sp, Stack: append([]CallFrame(nil), v.frames...)}
	return &RuntimeError{Value: Value{Kind: ErrorKind, Data: e}, Stack: e.Stack}
}
func (v *VM) unary(op string, x Value, sp source.Span) (Value, *RuntimeError) {
	switch op {
	case "非":
		if x.Kind != BoolKind {
			return Nil(), v.fault("类型错误", "‘非’需要布尔值", sp)
		}
		return Bool(!x.Data.(bool)), nil
	case "-":
		if x.Kind == IntKind {
			n := x.Data.(int64)
			if n == minInt64 {
				return Nil(), v.fault("运行时错误", "整数溢出", sp)
			}
			return Int(-n), nil
		}
		if x.Kind == FloatKind {
			return Float(-x.Data.(float64)), nil
		}
		return Nil(), v.fault("类型错误", "负号需要数字", sp)
	}
	return Nil(), v.fault("运算错误", "未知一元运算："+op, sp)
}
func (v *VM) binary(op string, l, r Value, sp source.Span) (Value, *RuntimeError) {
	if op == "且" || op == "或" {
		if l.Kind != BoolKind || r.Kind != BoolKind {
			return Nil(), v.fault("类型错误", "逻辑运算需要布尔值", sp)
		}
		a, b := l.Data.(bool), r.Data.(bool)
		if op == "且" {
			return Bool(a && b), nil
		}
		return Bool(a || b), nil
	}
	if op == "==" || op == "!=" {
		eq := equal(l, r)
		if op == "!=" {
			eq = !eq
		}
		return Bool(eq), nil
	}
	if isNum(l) && isNum(r) {
		if l.Kind == IntKind && r.Kind == IntKind {
			return v.intBinary(op, l.Data.(int64), r.Data.(int64), sp)
		}
		a, b := number(l), number(r)
		switch op {
		case "+":
			return numeric(a+b, l, r), nil
		case "-":
			return numeric(a-b, l, r), nil
		case "*":
			return numeric(a*b, l, r), nil
		case "/":
			if b == 0 {
				return Nil(), v.fault("运行时错误", "除数不能为零", sp)
			}
			return Float(a / b), nil
		case "%":
			if b == 0 {
				return Nil(), v.fault("运行时错误", "除数不能为零", sp)
			}
			return Float(math.Mod(a, b)), nil
		case "<":
			return Bool(a < b), nil
		case "<=":
			return Bool(a <= b), nil
		case ">":
			return Bool(a > b), nil
		case ">=":
			return Bool(a >= b), nil
		}
	}
	if l.Kind == StringKind && r.Kind == StringKind {
		a, b := l.Data.(string), r.Data.(string)
		switch op {
		case "+":
			return Text(a + b), nil
		case "<":
			return Bool(a < b), nil
		case "<=":
			return Bool(a <= b), nil
		case ">":
			return Bool(a > b), nil
		case ">=":
			return Bool(a >= b), nil
		}
	}
	return Nil(), v.fault("类型错误", "运算“"+op+"”不支持“"+string(l.Kind)+"”和“"+string(r.Kind)+"”", sp)
}

const maxInt64 = int64(^uint64(0) >> 1)
const minInt64 = -maxInt64 - 1

func (v *VM) intBinary(op string, a, b int64, sp source.Span) (Value, *RuntimeError) {
	switch op {
	case "+":
		if (b > 0 && a > maxInt64-b) || (b < 0 && a < minInt64-b) {
			return Nil(), v.fault("运行时错误", "整数溢出", sp)
		}
		return Int(a + b), nil
	case "-":
		if (b < 0 && a > maxInt64+b) || (b > 0 && a < minInt64+b) {
			return Nil(), v.fault("运行时错误", "整数溢出", sp)
		}
		return Int(a - b), nil
	case "*":
		if a == 0 || b == 0 {
			return Int(0), nil
		}
		product := a * b
		if (a == minInt64 && b == -1) || (b == minInt64 && a == -1) || product/b != a {
			return Nil(), v.fault("运行时错误", "整数溢出", sp)
		}
		return Int(product), nil
	case "%":
		if b == 0 {
			return Nil(), v.fault("运行时错误", "除数不能为零", sp)
		}
		return Int(a % b), nil
	case "<":
		return Bool(a < b), nil
	case "<=":
		return Bool(a <= b), nil
	case ">":
		return Bool(a > b), nil
	case ">=":
		return Bool(a >= b), nil
	case "/":
		if b == 0 {
			return Nil(), v.fault("运行时错误", "除数不能为零", sp)
		}
		return Float(float64(a) / float64(b)), nil
	}
	return Nil(), v.fault("运算错误", "未知数字运算："+op, sp)
}
func isNum(x Value) bool { return x.Kind == IntKind || x.Kind == FloatKind }
func number(x Value) float64 {
	if x.Kind == IntKind {
		return float64(x.Data.(int64))
	}
	return x.Data.(float64)
}
func numeric(n float64, l, r Value) Value {
	if l.Kind == IntKind && r.Kind == IntKind {
		return Int(int64(n))
	}
	return Float(n)
}
func equal(a, b Value) bool {
	if isNum(a) && isNum(b) {
		return number(a) == number(b)
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case NilKind:
		return true
	case BoolKind:
		return a.Data == b.Data
	case StringKind:
		return a.Data == b.Data
	case BytesKind:
		return bytes.Equal(a.Data.(*BytesObject).Data, b.Data.(*BytesObject).Data)
	default:
		return a.Data == b.Data
	}
}

func validKey(k Value) (string, error) {
	switch k.Kind {
	case BoolKind:
		return fmt.Sprintf("b:%t", k.Data.(bool)), nil
	case IntKind:
		return fmt.Sprintf("i:%d", k.Data.(int64)), nil
	case StringKind:
		return "s:" + k.Data.(string), nil
	default:
		return "", errors.New("字典键只能是布尔、整数或文字")
	}
}
func setDict(d Value, k, val Value) error {
	key, e := validKey(k)
	if e != nil {
		return e
	}
	o := d.Data.(*DictObject)
	if _, ok := o.Values[key]; !ok {
		o.Order = append(o.Order, key)
		o.Keys[key] = k
	}
	o.Values[key] = val
	return nil
}
func (v *VM) indexGet(o, k Value, sp source.Span) (Value, *RuntimeError) {
	switch o.Kind {
	case ListKind:
		if k.Kind != IntKind {
			return Nil(), v.fault("类型错误", "列表下标必须是整数", sp)
		}
		i := k.Data.(int64)
		a := o.Data.(*ListObject).Items
		if i < 0 || i >= int64(len(a)) {
			return Nil(), v.fault("索引错误", "列表下标越界", sp)
		}
		return a[i], nil
	case StringKind:
		if k.Kind != IntKind {
			return Nil(), v.fault("类型错误", "文字下标必须是整数", sp)
		}
		i := k.Data.(int64)
		r := []rune(o.Data.(string))
		if i < 0 || i >= int64(len(r)) {
			return Nil(), v.fault("索引错误", "文字下标越界", sp)
		}
		return Text(string(r[i])), nil
	case BytesKind:
		if k.Kind != IntKind {
			return Nil(), v.fault("类型错误", "字节串下标必须是整数", sp)
		}
		i := k.Data.(int64)
		a := o.Data.(*BytesObject).Data
		if i < 0 || i >= int64(len(a)) {
			return Nil(), v.fault("索引错误", "字节串下标越界", sp)
		}
		return Int(int64(a[i])), nil
	case DictKind:
		key, e := validKey(k)
		if e != nil {
			return Nil(), v.fault("类型错误", e.Error(), sp)
		}
		x, ok := o.Data.(*DictObject).Values[key]
		if !ok {
			return Nil(), v.fault("键错误", "字典中不存在这个键", sp)
		}
		return x, nil
	case RecordKind:
		if k.Kind != StringKind {
			return Nil(), v.fault("类型错误", "记录下标必须是文字", sp)
		}
		x, ok := o.Data.(*RecordObject).Fields[k.Data.(string)]
		if !ok {
			return Nil(), v.fault("字段错误", "记录中不存在字段“"+k.Data.(string)+"”", sp)
		}
		return x, nil
	}
	return Nil(), v.fault("类型错误", "此值不能读取索引", sp)
}
func (v *VM) indexSet(o, k, val Value, sp source.Span) *RuntimeError {
	switch o.Kind {
	case ListKind:
		if k.Kind != IntKind {
			return v.fault("类型错误", "列表下标必须是整数", sp)
		}
		i := k.Data.(int64)
		a := o.Data.(*ListObject).Items
		if i < 0 || i >= int64(len(a)) {
			return v.fault("索引错误", "列表下标越界", sp)
		}
		a[i] = val
		return nil
	case DictKind:
		if e := setDict(o, k, val); e != nil {
			return v.fault("类型错误", e.Error(), sp)
		}
		return nil
	case RecordKind:
		if k.Kind != StringKind {
			return v.fault("类型错误", "记录下标必须是文字", sp)
		}
		return v.fieldSet(o, k.Data.(string), val, sp)
	}
	return v.fault("类型错误", "此值不能写入索引", sp)
}

func (v *VM) fieldGet(o Value, name string, sp source.Span) (Value, *RuntimeError) {
	switch o.Kind {
	case RecordKind:
		x, ok := o.Data.(*RecordObject).Fields[name]
		if !ok {
			return Nil(), v.fault("字段错误", "记录中不存在字段“"+name+"”", sp)
		}
		return x, nil
	case ModuleKind:
		x, ok := o.Data.(*ModuleObject).Exports[name]
		if !ok {
			return Nil(), v.fault("模块错误", "模块没有公开名称“"+name+"”", sp)
		}
		return x, nil
	case ErrorKind:
		e := o.Data.(*ErrorObject)
		switch name {
		case "类别":
			return Text(e.Category), nil
		case "消息":
			return Text(e.Message), nil
		case "调用栈":
			var a []Value
			for _, f := range e.Stack {
				a = append(a, Text(f.Name))
			}
			return List(a), nil
		}
	case ListKind:
		l := o.Data.(*ListObject)
		switch name {
		case "长度":
			return v.method("列表.长度", func(_ *VM, args []Value) (Value, *RuntimeError) {
				if len(args) != 0 {
					return Nil(), v.argError("长度", 0, len(args), sp)
				}
				return Int(int64(len(l.Items))), nil
			}), nil
		case "追加":
			return v.method("列表.追加", func(_ *VM, args []Value) (Value, *RuntimeError) {
				if len(args) != 1 {
					return Nil(), v.argError("追加", 1, len(args), sp)
				}
				l.Items = append(l.Items, args[0])
				return Nil(), nil
			}), nil
		case "移除首项":
			return v.method("列表.移除首项", func(_ *VM, args []Value) (Value, *RuntimeError) {
				if len(args) != 0 {
					return Nil(), v.argError("移除首项", 0, len(args), sp)
				}
				if len(l.Items) == 0 {
					return Nil(), v.fault("列表错误", "不能从空列表移除首项", sp)
				}
				x := l.Items[0]
				l.Items = l.Items[1:]
				return x, nil
			}), nil
		case "移除":
			return v.method("列表.移除", func(_ *VM, args []Value) (Value, *RuntimeError) {
				if len(args) != 1 || args[0].Kind != IntKind {
					return Nil(), v.fault("类型错误", "移除需要一个整数索引", sp)
				}
				i := args[0].Data.(int64)
				if i < 0 || i >= int64(len(l.Items)) {
					return Nil(), v.fault("索引错误", "列表索引越界", sp)
				}
				x := l.Items[i]
				l.Items = append(l.Items[:i], l.Items[i+1:]...)
				return x, nil
			}), nil
		case "清空":
			return v.method("列表.清空", func(_ *VM, args []Value) (Value, *RuntimeError) {
				if len(args) != 0 {
					return Nil(), v.argError("清空", 0, len(args), sp)
				}
				l.Items = nil
				return Nil(), nil
			}), nil
		case "包含":
			return v.method("列表.包含", func(_ *VM, args []Value) (Value, *RuntimeError) {
				if len(args) != 1 {
					return Nil(), v.argError("包含", 1, len(args), sp)
				}
				for _, x := range l.Items {
					if equal(x, args[0]) {
						return Bool(true), nil
					}
				}
				return Bool(false), nil
			}), nil
		}
	case StringKind:
		s := o.Data.(string)
		switch name {
		case "长度":
			return v.method("文字.长度", func(_ *VM, args []Value) (Value, *RuntimeError) {
				if len(args) != 0 {
					return Nil(), v.argError("长度", 0, len(args), sp)
				}
				return Int(int64(utf8.RuneCountInString(s))), nil
			}), nil
		}
	case BytesKind:
		a := o.Data.(*BytesObject)
		if name == "长度" {
			return v.method("字节串.长度", func(_ *VM, args []Value) (Value, *RuntimeError) {
				if len(args) != 0 {
					return Nil(), v.argError("长度", 0, len(args), sp)
				}
				return Int(int64(len(a.Data))), nil
			}), nil
		}
	case DictKind:
		d := o.Data.(*DictObject)
		switch name {
		case "长度":
			return v.method("字典.长度", func(_ *VM, args []Value) (Value, *RuntimeError) { return Int(int64(len(d.Order))), nil }), nil
		case "键":
			return v.method("字典.键", func(_ *VM, args []Value) (Value, *RuntimeError) {
				a := make([]Value, 0, len(d.Order))
				for _, k := range d.Order {
					a = append(a, d.Keys[k])
				}
				return List(a), nil
			}), nil
		case "值":
			return v.method("字典.值", func(_ *VM, args []Value) (Value, *RuntimeError) {
				a := make([]Value, 0, len(d.Order))
				for _, k := range d.Order {
					a = append(a, d.Values[k])
				}
				return List(a), nil
			}), nil
		case "包含键":
			return v.method("字典.包含键", func(_ *VM, args []Value) (Value, *RuntimeError) {
				if len(args) != 1 {
					return Nil(), v.argError("包含键", 1, len(args), sp)
				}
				k, e := validKey(args[0])
				if e != nil {
					return Bool(false), nil
				}
				_, ok := d.Values[k]
				return Bool(ok), nil
			}), nil
		case "条目":
			return v.method("字典.条目", func(_ *VM, args []Value) (Value, *RuntimeError) {
				if len(args) != 0 {
					return Nil(), v.argError("条目", 0, len(args), sp)
				}
				items := make([]Value, 0, len(d.Order))
				for _, key := range d.Order {
					items = append(items, recordValues(map[string]Value{"键": d.Keys[key], "值": d.Values[key]}))
				}
				return List(items), nil
			}), nil
		}
	}
	return Nil(), v.fault("字段错误", "值“"+string(o.Kind)+"”没有字段“"+name+"”", sp)
}
func (v *VM) fieldSet(o Value, name string, val Value, sp source.Span) *RuntimeError {
	if o.Kind != RecordKind {
		return v.fault("类型错误", "只有记录可以通过点号写入字段", sp)
	}
	r := o.Data.(*RecordObject)
	if _, ok := r.Fields[name]; !ok {
		r.Order = append(r.Order, name)
	}
	r.Fields[name] = val
	return nil
}
func (v *VM) method(name string, fn NativeFunc) Value {
	return Value{Kind: NativeKind, Data: &NativeObject{Name: name, Fn: fn}}
}
func (v *VM) argError(name string, want, got int, sp source.Span) *RuntimeError {
	return v.fault("调用错误", fmt.Sprintf("函数“%s”需要 %d 个参数，实际得到 %d 个", name, want, got), sp)
}
func (v *VM) makeIterator(x Value, sp source.Span) (Value, *RuntimeError) {
	switch x.Kind {
	case ListKind:
		list := x.Data.(*ListObject)
		return Value{Kind: IteratorKind, Data: &IteratorObject{Values: append([]Value(nil), list.Items...), List: list, InitialSize: len(list.Items)}}, nil
	case StringKind:
		r := []rune(x.Data.(string))
		a := make([]Value, len(r))
		for i, c := range r {
			a[i] = Text(string(c))
		}
		return Value{Kind: IteratorKind, Data: &IteratorObject{Values: a}}, nil
	case DictKind:
		d := x.Data.(*DictObject)
		a := make([]Value, 0, len(d.Order))
		for _, k := range d.Order {
			a = append(a, d.Keys[k])
		}
		return Value{Kind: IteratorKind, Data: &IteratorObject{Values: a}}, nil
	}
	return Nil(), v.fault("类型错误", "只有列表、字典和文字可以遍历", sp)
}

func (v *VM) installBuiltins() {
	v.globals.Define("打印", v.native("打印", func(vm *VM, args []Value) (Value, *RuntimeError) {
		parts := make([]string, len(args))
		for i, x := range args {
			parts[i] = x.String()
		}
		fmt.Fprintln(vm.Out, strings.Join(parts, " "))
		return Nil(), nil
	}), false)
	v.globals.Define("输入", v.native("输入", func(vm *VM, args []Value) (Value, *RuntimeError) {
		if len(args) > 1 {
			return Nil(), vm.argError("输入", 0, len(args), source.Span{})
		}
		if len(args) == 1 {
			fmt.Fprint(vm.Out, args[0].String())
		}
		line, e := vm.reader.ReadString('\n')
		if e != nil && len(line) == 0 {
			return Nil(), vm.fault("输入错误", "读取输入失败："+e.Error(), source.Span{})
		}
		return Text(strings.TrimRight(line, "\r\n")), nil
	}), false)
	v.globals.Define("长度", v.native("长度", func(vm *VM, args []Value) (Value, *RuntimeError) {
		if len(args) != 1 {
			return Nil(), vm.argError("长度", 1, len(args), source.Span{})
		}
		switch x := args[0]; {
		case x.Kind == StringKind:
			return Int(int64(utf8.RuneCountInString(x.Data.(string)))), nil
		case x.Kind == BytesKind:
			return Int(int64(len(x.Data.(*BytesObject).Data))), nil
		case x.Kind == ListKind:
			return Int(int64(len(x.Data.(*ListObject).Items))), nil
		case x.Kind == DictKind:
			return Int(int64(len(x.Data.(*DictObject).Order))), nil
		case x.Kind == RecordKind:
			return Int(int64(len(x.Data.(*RecordObject).Order))), nil
		}
		return Nil(), vm.fault("类型错误", "长度不支持此类型", source.Span{})
	}), false)
	v.globals.Define("类型", v.native("类型", func(vm *VM, args []Value) (Value, *RuntimeError) {
		if len(args) != 1 {
			return Nil(), vm.argError("类型", 1, len(args), source.Span{})
		}
		return Text(typeName(args[0])), nil
	}), false)
	v.globals.Define("转文字", v.native("转文字", func(vm *VM, args []Value) (Value, *RuntimeError) {
		if len(args) != 1 {
			return Nil(), vm.argError("转文字", 1, len(args), source.Span{})
		}
		return Text(args[0].String()), nil
	}), false)
	v.globals.Define("转整数", v.native("转整数", func(vm *VM, args []Value) (Value, *RuntimeError) {
		if len(args) != 1 {
			return Nil(), vm.argError("转整数", 1, len(args), source.Span{})
		}
		switch x := args[0]; {
		case x.Kind == IntKind:
			return x, nil
		case x.Kind == FloatKind:
			return Int(int64(x.Data.(float64))), nil
		case x.Kind == StringKind:
			n, e := strconv.ParseInt(strings.TrimSpace(x.Data.(string)), 10, 64)
			if e == nil {
				return Int(n), nil
			}
		}
		return Nil(), vm.fault("转换错误", "无法转换为整数", source.Span{})
	}), false)
	v.globals.Define("转小数", v.native("转小数", func(vm *VM, args []Value) (Value, *RuntimeError) {
		if len(args) != 1 {
			return Nil(), vm.argError("转小数", 1, len(args), source.Span{})
		}
		switch x := args[0]; {
		case x.Kind == FloatKind:
			return x, nil
		case x.Kind == IntKind:
			return Float(float64(x.Data.(int64))), nil
		case x.Kind == StringKind:
			n, e := strconv.ParseFloat(strings.TrimSpace(x.Data.(string)), 64)
			if e == nil {
				return Float(n), nil
			}
		}
		return Nil(), vm.fault("转换错误", "无法转换为小数", source.Span{})
	}), false)
	v.globals.Define("范围", v.native("范围", func(vm *VM, args []Value) (Value, *RuntimeError) {
		if len(args) < 1 || len(args) > 3 {
			return Nil(), vm.argError("范围", 1, len(args), source.Span{})
		}
		ns := make([]int64, len(args))
		for i, x := range args {
			if x.Kind != IntKind {
				return Nil(), vm.fault("类型错误", "范围参数必须是整数", source.Span{})
			}
			ns[i] = x.Data.(int64)
		}
		start, end, step := int64(0), ns[0], int64(1)
		if len(ns) >= 2 {
			start, end = ns[0], ns[1]
		}
		if len(ns) == 3 {
			step = ns[2]
		}
		if step == 0 {
			return Nil(), vm.fault("运行时错误", "范围步长不能为零", source.Span{})
		}
		var a []Value
		for i := start; (step > 0 && i < end) || (step < 0 && i > end); i += step {
			a = append(a, Int(i))
		}
		return List(a), nil
	}), false)
	v.globals.Define("错误", v.native("错误", func(vm *VM, args []Value) (Value, *RuntimeError) {
		if len(args) != 1 || args[0].Kind != StringKind {
			return Nil(), vm.fault("调用错误", "错误需要一个文字参数", source.Span{})
		}
		return Value{Kind: ErrorKind, Data: &ErrorObject{Category: "用户错误", Message: args[0].Data.(string)}}, nil
	}), false)
	v.globals.Define("断言", v.native("断言", func(vm *VM, args []Value) (Value, *RuntimeError) {
		if len(args) < 1 || len(args) > 2 || args[0].Kind != BoolKind {
			return Nil(), vm.fault("调用错误", "断言需要布尔值和可选的消息", source.Span{})
		}
		if args[0].Data.(bool) {
			return Nil(), nil
		}
		msg := "断言失败"
		if len(args) == 2 {
			msg = args[1].String()
		}
		return Nil(), vm.fault("断言错误", msg, source.Span{})
	}), false)
}
func (v *VM) native(name string, fn NativeFunc) Value {
	return Value{Kind: NativeKind, Data: &NativeObject{Name: name, Fn: fn}}
}

func (v *VM) filePath(path string) string {
	path = filepath.Clean(path)
	if filepath.IsAbs(path) || v.WorkingDir == "" {
		return path
	}
	return filepath.Join(v.WorkingDir, path)
}

type atomicFileItem struct {
	path, temp string
	old        []byte
	hadOld     bool
}

func (v *VM) atomicBatchWrite(args []Value) *RuntimeError {
	if len(args) != 1 || args[0].Kind != ListKind {
		return v.fault("类型错误", "原子写入组需要一个记录列表", source.Span{})
	}
	items := args[0].Data.(*ListObject).Items
	files := make([]atomicFileItem, 0, len(items))
	cleanup := func() {
		for _, item := range files {
			if item.temp != "" {
				_ = os.Remove(item.temp)
			}
		}
	}
	for _, item := range items {
		if item.Kind != RecordKind {
			cleanup()
			return v.fault("类型错误", "原子写入组项目必须是记录", source.Span{})
		}
		r := item.Data.(*RecordObject)
		pv, pok := r.Fields["路径"]
		cv, cok := r.Fields["内容"]
		if !pok || !cok || pv.Kind != StringKind || cv.Kind != StringKind {
			cleanup()
			return v.fault("类型错误", "原子写入组记录需要路径和内容文字", source.Span{})
		}
		path := v.filePath(pv.Data.(string))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			cleanup()
			return v.fault("文件错误", "创建目录失败："+err.Error(), source.Span{})
		}
		old, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			cleanup()
			return v.fault("文件错误", "读取旧文件失败："+err.Error(), source.Span{})
		}
		f, err := os.CreateTemp(filepath.Dir(path), ".华言-批量-*")
		if err != nil {
			cleanup()
			return v.fault("文件错误", "创建临时文件失败："+err.Error(), source.Span{})
		}
		temp := f.Name()
		if _, err = f.WriteString(cv.Data.(string)); err == nil {
			err = f.Sync()
		}
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(temp)
			cleanup()
			return v.fault("文件错误", "准备批量写入失败："+err.Error(), source.Span{})
		}
		files = append(files, atomicFileItem{path: path, temp: temp, old: old, hadOld: err == nil && len(old) > 0})
		if _, statErr := os.Stat(path); statErr == nil {
			files[len(files)-1].hadOld = true
		}
	}
	for _, item := range files {
		if item.hadOld {
			if err := os.WriteFile(item.path+".bak", item.old, 0644); err != nil {
				cleanup()
				return v.fault("文件错误", "保存备份失败："+err.Error(), source.Span{})
			}
		}
	}
	committed := 0
	for _, item := range files {
		err := os.Rename(item.temp, item.path)
		if err != nil && runtime.GOOS == "windows" {
			_ = os.Remove(item.path)
			err = os.Rename(item.temp, item.path)
		}
		if err != nil {
			for i := 0; i < committed; i++ {
				old := files[i]
				if old.hadOld {
					_ = os.WriteFile(old.path, old.old, 0644)
				} else {
					_ = os.Remove(old.path)
				}
			}
			cleanup()
			return v.fault("文件错误", "批量原子替换失败："+err.Error(), source.Span{})
		}
		committed++
	}
	cleanup()
	return nil
}

func typeName(v Value) string {
	if v.Kind == NativeKind {
		return "函数"
	}
	return string(v.Kind)
}

func (v *VM) importModule(path, from string, sp source.Span) (Value, *RuntimeError) {
	if x, ok := v.modules[path]; ok {
		return x, nil
	}
	if x, ok := StandardModule(path, v); ok {
		v.modules[path] = x
		return x, nil
	}
	if v.Loader == nil {
		return Nil(), v.fault("模块错误", "找不到模块："+path, sp)
	}
	x, e := v.Loader(path, from)
	if e != nil {
		return Nil(), v.fault("模块错误", e.Error(), sp)
	}
	if x.Kind != ModuleKind {
		return Nil(), v.fault("模块错误", "模块加载器返回了无效值", sp)
	}
	v.modules[path] = x
	return x, nil
}

func module(path string) Value {
	return Value{Kind: ModuleKind, Data: &ModuleObject{Path: path, Exports: map[string]Value{}}}
}
func export(m Value, name string, v Value) { m.Data.(*ModuleObject).Exports[name] = v }
func stringArg(args []Value, i int, name string, vm *VM) (string, *RuntimeError) {
	if i >= len(args) || args[i].Kind != StringKind {
		return "", vm.fault("类型错误", name+"的参数必须是文字", source.Span{})
	}
	return args[i].Data.(string), nil
}

func bytesArg(args []Value, i int, name string, vm *VM) ([]byte, *RuntimeError) {
	if i >= len(args) || args[i].Kind != BytesKind {
		return nil, vm.fault("类型错误", name+"的参数必须是字节串", source.Span{})
	}
	return args[i].Data.(*BytesObject).Data, nil
}

// StandardModule is kept inside the VM so native functions receive the same
// controlled value interface as user functions.
func StandardModule(path string, v *VM) (Value, bool) {
	m := module(path)
	switch path {
	case "标准.控制台":
		export(m, "输出", v.native("控制台.输出", func(vm *VM, args []Value) (Value, *RuntimeError) {
			parts := make([]string, len(args))
			for i, x := range args {
				parts[i] = x.String()
			}
			fmt.Fprintln(vm.Out, strings.Join(parts, " "))
			return Nil(), nil
		}))
		export(m, "错误输出", v.native("控制台.错误输出", func(vm *VM, args []Value) (Value, *RuntimeError) {
			parts := make([]string, len(args))
			for i, x := range args {
				parts[i] = x.String()
			}
			fmt.Fprintln(os.Stderr, strings.Join(parts, " "))
			return Nil(), nil
		}))
		export(m, "读取一行", v.native("控制台.读取一行", func(vm *VM, args []Value) (Value, *RuntimeError) {
			line, e := vm.reader.ReadString('\n')
			if e != nil && len(line) == 0 {
				return Nil(), vm.fault("输入错误", e.Error(), source.Span{})
			}
			return Text(strings.TrimRight(line, "\r\n")), nil
		}))
	case "标准.文字":
		export(m, "长度", v.native("文字.长度", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 1 || args[0].Kind != StringKind {
				return Nil(), vm.fault("类型错误", "文字.长度需要一个文字", source.Span{})
			}
			return Int(int64(len([]rune(args[0].Data.(string))))), nil
		}))
		export(m, "查找", v.native("文字.查找", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 2 || args[0].Kind != StringKind || args[1].Kind != StringKind {
				return Nil(), vm.fault("类型错误", "查找需要两个文字", source.Span{})
			}
			return Int(int64(strings.Index(args[0].Data.(string), args[1].Data.(string)))), nil
		}))
		export(m, "替换", v.native("文字.替换", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 3 {
				return Nil(), vm.argError("替换", 3, len(args), source.Span{})
			}
			for _, x := range args {
				if x.Kind != StringKind {
					return Nil(), vm.fault("类型错误", "替换参数必须是文字", source.Span{})
				}
			}
			return Text(strings.ReplaceAll(args[0].Data.(string), args[1].Data.(string), args[2].Data.(string))), nil
		}))
		export(m, "分割", v.native("文字.分割", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 2 || args[0].Kind != StringKind || args[1].Kind != StringKind {
				return Nil(), vm.fault("类型错误", "分割需要两个文字", source.Span{})
			}
			parts := strings.Split(args[0].Data.(string), args[1].Data.(string))
			a := make([]Value, len(parts))
			for i, s := range parts {
				a[i] = Text(s)
			}
			return List(a), nil
		}))
		export(m, "裁剪", v.native("文字.裁剪", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 1 || args[0].Kind != StringKind {
				return Nil(), vm.fault("类型错误", "裁剪需要一个文字", source.Span{})
			}
			return Text(strings.TrimSpace(args[0].Data.(string))), nil
		}))
		export(m, "大写", v.native("文字.大写", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 1 || args[0].Kind != StringKind {
				return Nil(), vm.fault("类型错误", "大写需要一个文字", source.Span{})
			}
			return Text(strings.ToUpper(args[0].Data.(string))), nil
		}))
		export(m, "小写", v.native("文字.小写", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 1 || args[0].Kind != StringKind {
				return Nil(), vm.fault("类型错误", "小写需要一个文字", source.Span{})
			}
			return Text(strings.ToLower(args[0].Data.(string))), nil
		}))
	case "标准.列表":
		export(m, "排序", v.native("列表.排序", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 1 || args[0].Kind != ListKind {
				return Nil(), vm.fault("类型错误", "排序需要列表", source.Span{})
			}
			a := args[0].Data.(*ListObject).Items
			for i := 1; i < len(a); i++ {
				for j := i; j > 0; j-- {
					ok, _ := less(a[j], a[j-1])
					if !ok {
						break
					}
					a[j], a[j-1] = a[j-1], a[j]
				}
			}
			return args[0], nil
		}))
		export(m, "查找", v.native("列表.查找", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 2 || args[0].Kind != ListKind {
				return Nil(), v.fault("类型错误", "查找需要列表和目标", source.Span{})
			}
			for i, x := range args[0].Data.(*ListObject).Items {
				if equal(x, args[1]) {
					return Int(int64(i)), nil
				}
			}
			return Int(-1), nil
		}))
		export(m, "映射", v.native("列表.映射", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 2 || args[0].Kind != ListKind {
				return Nil(), v.fault("类型错误", "映射需要列表和函数", source.Span{})
			}
			var out []Value
			for _, x := range args[0].Data.(*ListObject).Items {
				y, e := vm.call(args[1], []Value{x}, source.Span{})
				if e != nil {
					return Nil(), e
				}
				out = append(out, y)
			}
			return List(out), nil
		}))
		export(m, "过滤", v.native("列表.过滤", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 2 || args[0].Kind != ListKind {
				return Nil(), v.fault("类型错误", "过滤需要列表和函数", source.Span{})
			}
			var out []Value
			for _, x := range args[0].Data.(*ListObject).Items {
				y, e := vm.call(args[1], []Value{x}, source.Span{})
				if e != nil {
					return Nil(), e
				}
				if y.Kind != BoolKind {
					return Nil(), v.fault("类型错误", "过滤函数必须返回布尔值", source.Span{})
				}
				if y.Data.(bool) {
					out = append(out, x)
				}
			}
			return List(out), nil
		}))
	case "标准.字典":
		export(m, "键", v.native("字典.键", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 1 || args[0].Kind != DictKind {
				return Nil(), v.fault("类型错误", "键需要字典", source.Span{})
			}
			d := args[0].Data.(*DictObject)
			a := make([]Value, 0, len(d.Order))
			for _, k := range d.Order {
				a = append(a, d.Keys[k])
			}
			return List(a), nil
		}))
		export(m, "值", v.native("字典.值", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 1 || args[0].Kind != DictKind {
				return Nil(), v.fault("类型错误", "值需要字典", source.Span{})
			}
			d := args[0].Data.(*DictObject)
			a := make([]Value, 0, len(d.Order))
			for _, k := range d.Order {
				a = append(a, d.Values[k])
			}
			return List(a), nil
		}))
		export(m, "包含键", v.native("字典.包含键", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 2 || args[0].Kind != DictKind {
				return Nil(), v.fault("类型错误", "包含键需要字典和键", source.Span{})
			}
			k, e := validKey(args[1])
			if e != nil {
				return Bool(false), nil
			}
			_, ok := args[0].Data.(*DictObject).Values[k]
			return Bool(ok), nil
		}))
		export(m, "条目", v.native("字典.条目", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 1 || args[0].Kind != DictKind {
				return Nil(), vm.fault("类型错误", "条目需要字典", source.Span{})
			}
			d := args[0].Data.(*DictObject)
			items := make([]Value, 0, len(d.Order))
			for _, key := range d.Order {
				items = append(items, recordValues(map[string]Value{"键": d.Keys[key], "值": d.Values[key]}))
			}
			return List(items), nil
		}))
	case "标准.文件":
		export(m, "读取文字", v.native("文件.读取文字", func(vm *VM, args []Value) (Value, *RuntimeError) {
			p, e := stringArg(args, 0, "读取文字", vm)
			if e != nil {
				return Nil(), e
			}
			b, x := os.ReadFile(vm.filePath(p))
			if x != nil {
				return Nil(), vm.fault("文件错误", "读取文件失败："+x.Error(), source.Span{})
			}
			return Text(string(b)), nil
		}))
		export(m, "写入文字", v.native("文件.写入文字", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 2 {
				return Nil(), vm.argError("写入文字", 2, len(args), source.Span{})
			}
			p, e := stringArg(args, 0, "写入文字", vm)
			if e != nil {
				return Nil(), e
			}
			s, e := stringArg(args, 1, "写入文字", vm)
			if e != nil {
				return Nil(), e
			}
			if x := os.WriteFile(vm.filePath(p), []byte(s), 0644); x != nil {
				return Nil(), vm.fault("文件错误", "写入文件失败："+x.Error(), source.Span{})
			}
			return Nil(), nil
		}))
		export(m, "读取字节", v.native("文件.读取字节", func(vm *VM, args []Value) (Value, *RuntimeError) {
			p, e := stringArg(args, 0, "读取字节", vm)
			if e != nil {
				return Nil(), e
			}
			b, x := os.ReadFile(vm.filePath(p))
			if x != nil {
				return Nil(), vm.fault("文件错误", "读取文件失败："+x.Error(), source.Span{})
			}
			return Bytes(b), nil
		}))
		export(m, "写入字节", v.native("文件.写入字节", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 2 {
				return Nil(), vm.argError("写入字节", 2, len(args), source.Span{})
			}
			p, e := stringArg(args, 0, "写入字节", vm)
			if e != nil {
				return Nil(), e
			}
			b, e := bytesArg(args, 1, "写入字节", vm)
			if e != nil {
				return Nil(), e
			}
			if x := os.WriteFile(vm.filePath(p), b, 0644); x != nil {
				return Nil(), vm.fault("文件错误", "写入文件失败："+x.Error(), source.Span{})
			}
			return Nil(), nil
		}))
		export(m, "追加文字", v.native("文件.追加文字", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 2 {
				return Nil(), vm.argError("追加文字", 2, len(args), source.Span{})
			}
			p, e := stringArg(args, 0, "追加文字", vm)
			if e != nil {
				return Nil(), e
			}
			s, e := stringArg(args, 1, "追加文字", vm)
			if e != nil {
				return Nil(), e
			}
			f, x := os.OpenFile(vm.filePath(p), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if x != nil {
				return Nil(), vm.fault("文件错误", "打开文件失败："+x.Error(), source.Span{})
			}
			defer f.Close()
			if _, x = f.WriteString(s); x != nil {
				return Nil(), vm.fault("文件错误", "追加文件失败："+x.Error(), source.Span{})
			}
			return Nil(), nil
		}))
		export(m, "存在", v.native("文件.存在", func(vm *VM, args []Value) (Value, *RuntimeError) {
			p, e := stringArg(args, 0, "存在", vm)
			if e != nil {
				return Nil(), e
			}
			_, x := os.Stat(vm.filePath(p))
			return Bool(x == nil), nil
		}))
		export(m, "创建目录", v.native("文件.创建目录", func(vm *VM, args []Value) (Value, *RuntimeError) {
			p, e := stringArg(args, 0, "创建目录", vm)
			if e != nil {
				return Nil(), e
			}
			if x := os.MkdirAll(vm.filePath(p), 0755); x != nil {
				return Nil(), vm.fault("文件错误", "创建目录失败："+x.Error(), source.Span{})
			}
			return Nil(), nil
		}))
		export(m, "原子写入", v.native("文件.原子写入", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 2 {
				return Nil(), vm.argError("原子写入", 2, len(args), source.Span{})
			}
			p, e := stringArg(args, 0, "原子写入", vm)
			if e != nil {
				return Nil(), e
			}
			s, e := stringArg(args, 1, "原子写入", vm)
			if e != nil {
				return Nil(), e
			}
			p = vm.filePath(p)
			dir := filepath.Dir(p)
			f, x := os.CreateTemp(dir, ".华言-*")
			if x != nil {
				return Nil(), vm.fault("文件错误", "创建临时文件失败："+x.Error(), source.Span{})
			}
			tmp := f.Name()
			defer os.Remove(tmp)
			if _, x = f.WriteString(s); x == nil {
				x = f.Sync()
			}
			if closeErr := f.Close(); x == nil {
				x = closeErr
			}
			if x == nil {
				if old, readErr := os.ReadFile(p); readErr == nil {
					if backupErr := os.WriteFile(p+".bak", old, 0644); backupErr != nil {
						x = backupErr
					}
				}
			}
			if x == nil {
				x = os.Rename(tmp, p)
			}
			if x != nil {
				return Nil(), vm.fault("文件错误", "原子写入失败："+x.Error(), source.Span{})
			}
			return Nil(), nil
		}))
		export(m, "原子写入组", v.native("文件.原子写入组", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if err := vm.atomicBatchWrite(args); err != nil {
				return Nil(), err
			}
			return Nil(), nil
		}))
	case "标准.JSON":
		export(m, "解析", v.native("JSON.解析", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 1 || args[0].Kind != StringKind {
				return Nil(), vm.fault("类型错误", "JSON.解析需要文字", source.Span{})
			}
			dec := json.NewDecoder(strings.NewReader(args[0].Data.(string)))
			dec.UseNumber()
			value, e := decodeJSONValue(dec)
			if e != nil {
				return Nil(), vm.fault("JSON错误", "解析 JSON 失败："+e.Error(), source.Span{})
			}
			if _, e = dec.Token(); e != io.EOF {
				if e == nil {
					return Nil(), vm.fault("JSON错误", "JSON 文本后还有多余内容", source.Span{})
				}
				return Nil(), vm.fault("JSON错误", "读取 JSON 结束位置失败："+e.Error(), source.Span{})
			}
			return value, nil
		}))
		jsonEncode := func(vm *VM, args []Value, indent bool) (Value, *RuntimeError) {
			if len(args) != 1 {
				return Nil(), vm.argError("JSON.序列化", 1, len(args), source.Span{})
			}
			text, e := marshalJSON(args[0], indent)
			if e != nil {
				return Nil(), vm.fault("JSON错误", e.Error(), source.Span{})
			}
			return Text(text), nil
		}
		export(m, "序列化", v.native("JSON.序列化", func(vm *VM, args []Value) (Value, *RuntimeError) { return jsonEncode(vm, args, false) }))
		export(m, "格式化", v.native("JSON.格式化", func(vm *VM, args []Value) (Value, *RuntimeError) { return jsonEncode(vm, args, true) }))
	case "标准.编码":
		export(m, "UTF8编码", v.native("编码.UTF8编码", func(vm *VM, args []Value) (Value, *RuntimeError) {
			s, e := stringArg(args, 0, "UTF8编码", vm)
			if e != nil {
				return Nil(), e
			}
			return Bytes([]byte(s)), nil
		}))
		export(m, "UTF8解码", v.native("编码.UTF8解码", func(vm *VM, args []Value) (Value, *RuntimeError) {
			b, e := bytesArg(args, 0, "UTF8解码", vm)
			if e != nil {
				return Nil(), e
			}
			if !utf8.Valid(b) {
				return Nil(), vm.fault("编码错误", "字节串不是合法的 UTF-8", source.Span{})
			}
			return Text(string(b)), nil
		}))
		export(m, "Base64编码", v.native("编码.Base64编码", func(vm *VM, args []Value) (Value, *RuntimeError) {
			b, e := bytesArg(args, 0, "Base64编码", vm)
			if e != nil {
				return Nil(), e
			}
			return Text(base64.StdEncoding.EncodeToString(b)), nil
		}))
		export(m, "Base64解码", v.native("编码.Base64解码", func(vm *VM, args []Value) (Value, *RuntimeError) {
			s, e := stringArg(args, 0, "Base64解码", vm)
			if e != nil {
				return Nil(), e
			}
			b, x := base64.StdEncoding.DecodeString(s)
			if x != nil {
				return Nil(), vm.fault("编码错误", "Base64 解码失败："+x.Error(), source.Span{})
			}
			return Bytes(b), nil
		}))
		export(m, "十六进制编码", v.native("编码.十六进制编码", func(vm *VM, args []Value) (Value, *RuntimeError) {
			b, e := bytesArg(args, 0, "十六进制编码", vm)
			if e != nil {
				return Nil(), e
			}
			return Text(hex.EncodeToString(b)), nil
		}))
		export(m, "十六进制解码", v.native("编码.十六进制解码", func(vm *VM, args []Value) (Value, *RuntimeError) {
			s, e := stringArg(args, 0, "十六进制解码", vm)
			if e != nil {
				return Nil(), e
			}
			b, x := hex.DecodeString(s)
			if x != nil {
				return Nil(), vm.fault("编码错误", "十六进制解码失败："+x.Error(), source.Span{})
			}
			return Bytes(b), nil
		}))
	case "标准.时间":
		export(m, "现在", v.native("时间.现在", func(vm *VM, args []Value) (Value, *RuntimeError) { return Text(time.Now().Format(time.RFC3339)), nil }))
		export(m, "当前时间", m.Data.(*ModuleObject).Exports["现在"])
		export(m, "解析", v.native("时间.解析", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 2 || args[0].Kind != StringKind || args[1].Kind != StringKind {
				return Nil(), vm.fault("类型错误", "时间.解析需要时间文字和格式文字", source.Span{})
			}
			if _, e := time.Parse(args[1].Data.(string), args[0].Data.(string)); e != nil {
				return Nil(), vm.fault("时间错误", "解析时间失败："+e.Error(), source.Span{})
			}
			return args[0], nil
		}))
		export(m, "格式化", v.native("时间.格式化", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 2 || args[0].Kind != StringKind || args[1].Kind != StringKind {
				return Nil(), vm.fault("类型错误", "时间.格式化需要时间文字和格式文字", source.Span{})
			}
			t, e := time.Parse(time.RFC3339, args[0].Data.(string))
			if e != nil {
				return Nil(), vm.fault("时间错误", e.Error(), source.Span{})
			}
			return Text(t.Format(args[1].Data.(string))), nil
		}))
		export(m, "日期差", v.native("时间.日期差", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 2 || args[0].Kind != StringKind || args[1].Kind != StringKind {
				return Nil(), vm.fault("类型错误", "时间.日期差需要两个 RFC3339 时间文字", source.Span{})
			}
			start, e := time.Parse(time.RFC3339, args[0].Data.(string))
			if e != nil {
				return Nil(), vm.fault("时间错误", "解析开始时间失败："+e.Error(), source.Span{})
			}
			end, e := time.Parse(time.RFC3339, args[1].Data.(string))
			if e != nil {
				return Nil(), vm.fault("时间错误", "解析结束时间失败："+e.Error(), source.Span{})
			}
			return Int(int64(end.Sub(start) / time.Second)), nil
		}))
		export(m, "加秒", v.native("时间.加秒", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 2 || args[0].Kind != StringKind || args[1].Kind != IntKind {
				return Nil(), vm.fault("类型错误", "时间.加秒需要 RFC3339 时间文字和整数秒数", source.Span{})
			}
			t, e := time.Parse(time.RFC3339, args[0].Data.(string))
			if e != nil {
				return Nil(), vm.fault("时间错误", "解析时间失败："+e.Error(), source.Span{})
			}
			return Text(t.Add(time.Duration(args[1].Data.(int64)) * time.Second).Format(time.RFC3339)), nil
		}))
	case "标准.数学":
		export(m, "绝对值", v.native("数学.绝对值", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 1 {
				return Nil(), vm.argError("绝对值", 1, len(args), source.Span{})
			}
			if args[0].Kind == IntKind {
				x := args[0].Data.(int64)
				if x == minInt64 {
					return Nil(), vm.fault("运行时错误", "整数溢出", source.Span{})
				}
				if x < 0 {
					x = -x
				}
				return Int(x), nil
			}
			if args[0].Kind == FloatKind {
				return Float(math.Abs(args[0].Data.(float64))), nil
			}
			return Nil(), vm.fault("类型错误", "绝对值需要数字", source.Span{})
		}))
		export(m, "最小值", v.native("数学.最小值", func(vm *VM, args []Value) (Value, *RuntimeError) { return minmax(vm, args, true) }))
		export(m, "最大值", v.native("数学.最大值", func(vm *VM, args []Value) (Value, *RuntimeError) { return minmax(vm, args, false) }))
		export(m, "取整", v.native("数学.取整", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 1 || !isNum(args[0]) {
				return Nil(), vm.fault("类型错误", "取整需要数字", source.Span{})
			}
			return Int(int64(number(args[0]))), nil
		}))
	case "标准.程序":
		export(m, "参数", v.native("程序.参数", func(vm *VM, args []Value) (Value, *RuntimeError) {
			a := make([]Value, len(vm.Args))
			for i, s := range vm.Args {
				a[i] = Text(s)
			}
			return List(a), nil
		}))
		export(m, "环境", v.native("程序.环境", func(vm *VM, args []Value) (Value, *RuntimeError) {
			d := Dict()
			for _, kv := range os.Environ() {
				parts := strings.SplitN(kv, "=", 2)
				setDict(d, Text(parts[0]), Text(parts[1]))
			}
			return d, nil
		}))
		export(m, "退出", v.native("程序.退出", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) > 1 || (len(args) == 1 && args[0].Kind != IntKind) {
				return Nil(), vm.fault("类型错误", "程序.退出需要可选的整数状态码", source.Span{})
			}
			code := int64(0)
			if len(args) == 1 {
				code = args[0].Data.(int64)
			}
			exit := code
			err := vm.fault("程序退出", fmt.Sprintf("程序请求退出，状态码：%d", code), source.Span{})
			err.Value.Data.(*ErrorObject).ExitCode = &exit
			return Nil(), err
		}))
	case "标准.测试":
		export(m, "断言相等", v.native("测试.断言相等", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 2 {
				return Nil(), vm.argError("断言相等", 2, len(args), source.Span{})
			}
			if !equal(args[0], args[1]) {
				return Nil(), vm.fault("测试错误", "断言相等失败："+args[0].String()+" != "+args[1].String(), source.Span{})
			}
			return Nil(), nil
		}))
		export(m, "断言错误", v.native("测试.断言错误", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 1 || (args[0].Kind != FunctionKind && args[0].Kind != NativeKind) {
				return Nil(), vm.fault("类型错误", "断言错误需要函数", source.Span{})
			}
			_, re := vm.call(args[0], nil, source.Span{})
			if re == nil {
				return Nil(), vm.fault("测试错误", "断言错误失败：函数没有抛出错误", source.Span{})
			}
			return Nil(), nil
		}))
		export(m, "测试", v.native("测试.测试", func(vm *VM, args []Value) (Value, *RuntimeError) {
			if len(args) != 2 || args[0].Kind != StringKind || (args[1].Kind != FunctionKind && args[1].Kind != NativeKind) {
				return Nil(), vm.fault("类型错误", "测试需要名称和函数", source.Span{})
			}
			vm.tests = append(vm.tests, RegisteredTest{Name: args[0].Data.(string), Fn: args[1]})
			return Nil(), nil
		}))
		export(m, "运行", v.native("测试.运行", func(vm *VM, args []Value) (Value, *RuntimeError) {
			tests := vm.tests
			vm.tests = nil
			for _, test := range tests {
				if _, re := vm.call(test.Fn, nil, source.Span{}); re != nil {
					return Nil(), re
				}
				fmt.Fprintln(vm.Out, "通过："+test.Name)
			}
			return Nil(), nil
		}))
	default:
		return Nil(), false
	}
	return m, true
}
func less(a, b Value) (bool, error) {
	if isNum(a) && isNum(b) {
		return number(a) < number(b), nil
	}
	if a.Kind == StringKind && b.Kind == StringKind {
		return a.Data.(string) < b.Data.(string), nil
	}
	return false, errors.New("值不可排序")
}
func minmax(vm *VM, args []Value, min bool) (Value, *RuntimeError) {
	if len(args) == 0 {
		return Nil(), vm.fault("调用错误", "至少需要一个数字", source.Span{})
	}
	for _, x := range args {
		if !isNum(x) {
			return Nil(), vm.fault("类型错误", "参数必须是数字", source.Span{})
		}
	}
	best := args[0]
	for _, x := range args[1:] {
		if (min && number(x) < number(best)) || (!min && number(x) > number(best)) {
			best = x
		}
	}
	return best, nil
}

func fromJSON(x any) Value {
	switch v := x.(type) {
	case nil:
		return Nil()
	case bool:
		return Bool(v)
	case string:
		return Text(v)
	case json.Number:
		if i, e := strconv.ParseInt(string(v), 10, 64); e == nil {
			return Int(i)
		}
		f, _ := strconv.ParseFloat(string(v), 64)
		return Float(f)
	case []any:
		a := make([]Value, len(v))
		for i, y := range v {
			a[i] = fromJSON(y)
		}
		return List(a)
	case map[string]any:
		r := Record()
		o := r.Data.(*RecordObject)
		for k, y := range v {
			o.Order = append(o.Order, k)
			o.Fields[k] = fromJSON(y)
		}
		return r
	}
	return Nil()
}

func decodeJSONValue(dec *json.Decoder) (Value, error) {
	return decodeJSONValueAt(dec, 0)
}

func decodeJSONValueAt(dec *json.Decoder, depth int) (Value, error) {
	if depth > 256 {
		return Nil(), errors.New("JSON 嵌套层数超过 256")
	}
	tok, err := dec.Token()
	if err != nil {
		return Nil(), err
	}
	switch x := tok.(type) {
	case nil:
		return Nil(), nil
	case bool:
		return Bool(x), nil
	case string:
		return Text(x), nil
	case json.Number:
		if i, e := strconv.ParseInt(string(x), 10, 64); e == nil {
			return Int(i), nil
		}
		f, e := strconv.ParseFloat(string(x), 64)
		if e != nil {
			return Nil(), e
		}
		return Float(f), nil
	case json.Delim:
		switch x {
		case '[':
			var items []Value
			for dec.More() {
				item, e := decodeJSONValueAt(dec, depth+1)
				if e != nil {
					return Nil(), e
				}
				items = append(items, item)
			}
			if close, e := dec.Token(); e != nil || close != json.Delim(']') {
				return Nil(), errors.New("数组缺少结束括号")
			}
			return List(items), nil
		case '{':
			r := Record()
			obj := r.Data.(*RecordObject)
			for dec.More() {
				key, e := dec.Token()
				if e != nil {
					return Nil(), e
				}
				keyText, ok := key.(string)
				if !ok {
					return Nil(), errors.New("对象键必须是文字")
				}
				value, e := decodeJSONValueAt(dec, depth+1)
				if e != nil {
					return Nil(), e
				}
				obj.Order = append(obj.Order, keyText)
				obj.Fields[keyText] = value
			}
			if close, e := dec.Token(); e != nil || close != json.Delim('}') {
				return Nil(), errors.New("对象缺少结束大括号")
			}
			return r, nil
		}
	}
	return Nil(), errors.New("JSON 值类型无效")
}

func marshalJSON(v Value, pretty bool) (string, error) {
	return marshalJSONAt(v, pretty, 0, map[any]bool{})
}
func marshalJSONAt(v Value, pretty bool, level int, seen map[any]bool) (string, error) {
	if level > 256 {
		return "", errors.New("JSON 嵌套层数超过 256")
	}
	primitive := func(x any) (string, error) { b, e := json.Marshal(x); return string(b), e }
	split := func(items []string, open, close string) string {
		if len(items) == 0 {
			return open + close
		}
		if !pretty {
			return open + strings.Join(items, ",") + close
		}
		indent := strings.Repeat("  ", level+1)
		endIndent := strings.Repeat("  ", level)
		return open + "\n" + indent + strings.Join(items, ",\n"+indent) + "\n" + endIndent + close
	}
	switch v.Kind {
	case NilKind:
		return "null", nil
	case BoolKind, IntKind, FloatKind, StringKind:
		return primitive(v.Data)
	case ListKind:
		o := v.Data.(*ListObject)
		if seen[o] {
			return "", errors.New("JSON 不支持循环引用")
		}
		seen[o] = true
		defer delete(seen, o)
		items := make([]string, len(o.Items))
		for i, item := range o.Items {
			x, e := marshalJSONAt(item, pretty, level+1, seen)
			if e != nil {
				return "", e
			}
			items[i] = x
		}
		return split(items, "[", "]"), nil
	case DictKind:
		o := v.Data.(*DictObject)
		if seen[o] {
			return "", errors.New("JSON 不支持循环引用")
		}
		seen[o] = true
		defer delete(seen, o)
		items := make([]string, len(o.Order))
		for i, key := range o.Order {
			k := o.Keys[key].String()
			if o.Keys[key].Kind == StringKind {
				k = o.Keys[key].Data.(string)
			}
			keyText, _ := json.Marshal(k)
			x, e := marshalJSONAt(o.Values[key], pretty, level+1, seen)
			if e != nil {
				return "", e
			}
			items[i] = string(keyText) + ":" + mapJSONSeparator(pretty) + x
		}
		return split(items, "{", "}"), nil
	case RecordKind:
		o := v.Data.(*RecordObject)
		if seen[o] {
			return "", errors.New("JSON 不支持循环引用")
		}
		seen[o] = true
		defer delete(seen, o)
		items := make([]string, len(o.Order))
		for i, key := range o.Order {
			keyText, _ := json.Marshal(key)
			x, e := marshalJSONAt(o.Fields[key], pretty, level+1, seen)
			if e != nil {
				return "", e
			}
			items[i] = string(keyText) + ":" + mapJSONSeparator(pretty) + x
		}
		return split(items, "{", "}"), nil
	default:
		return "", errors.New("此类型不能序列化为 JSON")
	}
}
func mapJSONSeparator(pretty bool) string {
	if pretty {
		return " "
	}
	return ""
}
func toJSON(v Value, seen map[any]bool) (any, error) {
	switch v.Kind {
	case NilKind:
		return nil, nil
	case BoolKind, IntKind, FloatKind, StringKind:
		return v.Data, nil
	case ListKind:
		o := v.Data.(*ListObject)
		if seen[o] {
			return nil, errors.New("JSON 不支持循环引用")
		}
		seen[o] = true
		defer delete(seen, o)
		a := make([]any, len(o.Items))
		for i, x := range o.Items {
			y, e := toJSON(x, seen)
			if e != nil {
				return nil, e
			}
			a[i] = y
		}
		return a, nil
	case DictKind:
		o := v.Data.(*DictObject)
		if seen[o] {
			return nil, errors.New("JSON 不支持循环引用")
		}
		seen[o] = true
		defer delete(seen, o)
		m := map[string]any{}
		for _, k := range o.Order {
			y, e := toJSON(o.Values[k], seen)
			if e != nil {
				return nil, e
			}
			m[k] = y
		}
		return m, nil
	case RecordKind:
		o := v.Data.(*RecordObject)
		if seen[o] {
			return nil, errors.New("JSON 不支持循环引用")
		}
		seen[o] = true
		defer delete(seen, o)
		m := map[string]any{}
		for _, k := range o.Order {
			y, e := toJSON(o.Fields[k], seen)
			if e != nil {
				return nil, e
			}
			m[k] = y
		}
		return m, nil
	}
	return nil, errors.New("此类型不能序列化为 JSON")
}
