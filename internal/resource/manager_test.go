package resource

import (
	"errors"
	"reflect"
	"testing"
)

type testCloser struct {
	name  string
	order *[]string
	err   error
}

func (c testCloser) Close() error {
	*c.order = append(*c.order, c.name)
	return c.err
}

func TestManagerClosesOneResourceIdempotently(t *testing.T) {
	order := []string{}
	m := NewManager()
	h, err := m.Register("文件", testCloser{name: "文件", order: &order})
	if err != nil {
		t.Fatal(err)
	}
	if !m.IsOpen(h) || m.Count() != 1 {
		t.Fatal("registered resource is not open")
	}
	if name, ok := m.Name(h); !ok || name != "文件" {
		t.Fatalf("name=%q ok=%v", name, ok)
	}
	if err := m.Close(h); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(h); err != nil {
		t.Fatal("close was not idempotent:", err)
	}
	if m.IsOpen(h) || m.Count() != 0 {
		t.Fatal("resource remained open")
	}
	if !reflect.DeepEqual(order, []string{"文件"}) {
		t.Fatalf("close order=%v", order)
	}
}

func TestManagerClosesAllInReverseRegistrationOrder(t *testing.T) {
	order := []string{}
	m := NewManager()
	for _, name := range []string{"数据库", "事务", "响应"} {
		if _, err := m.Register(name, testCloser{name: name, order: &order}); err != nil {
			t.Fatal(err)
		}
	}
	errs := m.CloseAll()
	if len(errs) != 0 {
		t.Fatalf("close errors=%v", errs)
	}
	if want := []string{"响应", "事务", "数据库"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("close order=%v want=%v", order, want)
	}
	if m.Count() != 0 || len(m.CloseAll()) != 0 {
		t.Fatal("manager did not remain closed")
	}
	if _, err := m.Register("新资源", testCloser{order: &order}); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("register after close error=%v", err)
	}
}

func TestManagerReturnsCloserErrors(t *testing.T) {
	order := []string{}
	expected := errors.New("关闭失败")
	m := NewManager()
	if _, err := m.Register("错误资源", testCloser{name: "错误资源", order: &order, err: expected}); err != nil {
		t.Fatal(err)
	}
	errs := m.CloseAll()
	if len(errs) != 1 || !errors.Is(errs[0], expected) {
		t.Fatalf("errors=%v", errs)
	}
}

func TestManagerRejectsInvalidRegistration(t *testing.T) {
	m := NewManager()
	if _, err := m.Register("", testCloser{}); !errors.Is(err, ErrEmptyName) {
		t.Fatalf("empty name error=%v", err)
	}
	if _, err := m.Register("资源", nil); !errors.Is(err, ErrNilCloser) {
		t.Fatalf("nil closer error=%v", err)
	}
	if err := m.Close(Handle{}); !errors.Is(err, ErrInvalidHandle) {
		t.Fatalf("invalid handle error=%v", err)
	}
}
