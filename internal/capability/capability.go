// Package capability defines the permission set attached to a Huayan run.
// It has no dependency on the VM so embedders and project tooling can share it.
package capability

import (
	"errors"
	"sort"

	"huayan/internal/native"
)

var ErrDenied = errors.New("权限不足")

// Set is immutable from the caller's perspective after construction. The
// unrestricted form preserves the v0.3 behavior for direct script execution.
type Set struct {
	allowed map[native.Capability]struct{}
	all     bool
}

func New(capabilities ...native.Capability) Set {
	allowed := make(map[native.Capability]struct{}, len(capabilities))
	for _, c := range capabilities {
		allowed[c] = struct{}{}
	}
	return Set{allowed: allowed}
}

func AllowAll() Set { return Set{all: true} }

func (s Set) Allows(c native.Capability) bool {
	if s.all {
		return true
	}
	_, ok := s.allowed[c]
	return ok
}

func (s Set) Require(c native.Capability) error {
	if s.Allows(c) {
		return nil
	}
	return ErrDenied
}

func (s Set) Names() []native.Capability {
	if s.all {
		return nil
	}
	result := make([]native.Capability, 0, len(s.allowed))
	for c := range s.allowed {
		result = append(result, c)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
