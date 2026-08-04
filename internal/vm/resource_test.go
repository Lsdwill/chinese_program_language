package vm

import (
	"testing"

	"huayan/internal/native"
)

type vmResourceCloser struct {
	count int
}

func (c *vmResourceCloser) Close() error {
	c.count++
	return nil
}

func TestVMRegistersNativeModulesAndOwnsResources(t *testing.T) {
	v := New(nil, nil, nil)
	if !v.NativeModules.Has("标准.JSON") {
		t.Fatal("JSON native module is not registered")
	}
	if !v.NativeModules.Requires("标准.文件", native.CapabilityFileRead) {
		t.Fatal("file read capability is missing")
	}
	if v.NativeModules.Requires("标准.JSON", native.CapabilityDatabase) {
		t.Fatal("JSON unexpectedly requires database capability")
	}

	closer := &vmResourceCloser{}
	value, err := v.NewResource("测试资源", closer)
	if err != nil {
		t.Fatal(err)
	}
	if value.Kind != ResourceKind || value.String() != "<资源 测试资源>" {
		t.Fatalf("resource value=%v", value)
	}
	if v.Resources.Count() != 1 {
		t.Fatal("resource was not registered")
	}
	if err := v.CloseResource(value); err != nil {
		t.Fatal(err)
	}
	if closer.count != 1 || v.Resources.Count() != 0 {
		t.Fatalf("closer count=%d resources=%d", closer.count, v.Resources.Count())
	}
	if err := v.CloseResource(value); err != nil {
		t.Fatal("resource close should be idempotent:", err)
	}
}

func TestVMClosesResourcesInShutdown(t *testing.T) {
	v := New(nil, nil, nil)
	first := &vmResourceCloser{}
	second := &vmResourceCloser{}
	if _, err := v.NewResource("第一个", first); err != nil {
		t.Fatal(err)
	}
	if _, err := v.NewResource("第二个", second); err != nil {
		t.Fatal(err)
	}
	if errs := v.CloseResources(); len(errs) != 0 {
		t.Fatalf("close errors=%v", errs)
	}
	if first.count != 1 || second.count != 1 {
		t.Fatalf("close counts=%d,%d", first.count, second.count)
	}
	if _, err := v.NewResource("关闭后", &vmResourceCloser{}); err == nil {
		t.Fatal("resource registration succeeded after VM shutdown")
	}
}
