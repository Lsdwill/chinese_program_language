package native

import (
	"strings"
	"testing"
)

func TestRegistryNormalizesAndSortsDescriptors(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(Descriptor{
		Name: " 标准.HTTP ",
		Capabilities: []Capability{
			CapabilityNetworkIn,
			CapabilityNetworkOut,
			CapabilityNetworkIn,
		},
	}); err != nil {
		t.Fatal(err)
	}
	got, ok := r.Lookup("标准.HTTP")
	if !ok {
		t.Fatal("module was not registered")
	}
	if got.Name != "标准.HTTP" || len(got.Capabilities) != 2 {
		t.Fatalf("unexpected descriptor: %#v", got)
	}
	if got.Capabilities[0] >= got.Capabilities[1] {
		t.Fatalf("capabilities are not sorted: %#v", got.Capabilities)
	}
	got.Capabilities[0] = "被修改"
	again, _ := r.Lookup("标准.HTTP")
	if strings.Contains(string(again.Capabilities[0]), "被修改") {
		t.Fatal("lookup leaked the registry's capability slice")
	}
}

func TestRegistryRejectsDuplicateAndInvalidNames(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(Descriptor{}); err == nil {
		t.Fatal("empty module name was accepted")
	}
	if err := r.Register(Descriptor{Name: "标准.文件"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(Descriptor{Name: "标准.文件"}); err == nil {
		t.Fatal("duplicate module was accepted")
	}
	if !r.Requires("标准.文件", CapabilityFileRead) {
		// The descriptor did not declare this capability.
	} else {
		t.Fatal("undeclared capability was reported")
	}
	if r.Has("标准.不存在") {
		t.Fatal("unknown module was found")
	}
}

func TestRegistryNamesAreDeterministic(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"标准.时间", "标准.JSON", "标准.文件"} {
		if err := r.Register(Descriptor{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	got := r.Names()
	want := []string{"标准.JSON", "标准.文件", "标准.时间"}
	if len(got) != len(want) {
		t.Fatalf("names=%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("names=%v want=%v", got, want)
		}
	}
}
