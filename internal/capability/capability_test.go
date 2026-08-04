package capability

import (
	"testing"

	"huayan/internal/native"
)

func TestSet(t *testing.T) {
	s := New(native.CapabilityFileRead, native.CapabilityFileRead)
	if !s.Allows(native.CapabilityFileRead) || s.Allows(native.CapabilityFileWrite) {
		t.Fatal("capability membership failed")
	}
	if s.Require(native.CapabilityFileWrite) != ErrDenied {
		t.Fatal("missing capability was not denied")
	}
	if got := s.Names(); len(got) != 1 || got[0] != native.CapabilityFileRead {
		t.Fatalf("names=%v", got)
	}
	if !AllowAll().Allows(native.CapabilityDatabase) || AllowAll().Names() != nil {
		t.Fatal("unrestricted capability set failed")
	}
}
