// Package native stores metadata for host-provided Huayan modules.
//
// It deliberately does not depend on the VM. Keeping the registry independent
// lets the compiler, project checker, LSP, and runtime share the same module
// metadata without creating an import cycle.
package native

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

type Capability string

const (
	CapabilityFileRead   Capability = "文件读取"
	CapabilityFileWrite  Capability = "文件写入"
	CapabilityEnvRead    Capability = "环境读取"
	CapabilityNetworkOut Capability = "网络客户端"
	CapabilityNetworkIn  Capability = "网络监听"
	CapabilityDatabase   Capability = "数据库"
)

type Descriptor struct {
	Name         string
	Version      string
	Capabilities []Capability
}

func (d Descriptor) normalized() (Descriptor, error) {
	d.Name = strings.TrimSpace(d.Name)
	d.Version = strings.TrimSpace(d.Version)
	if d.Name == "" {
		return Descriptor{}, errors.New("原生模块名称不能为空")
	}
	seen := make(map[Capability]bool, len(d.Capabilities))
	caps := make([]Capability, 0, len(d.Capabilities))
	for _, capability := range d.Capabilities {
		capability = Capability(strings.TrimSpace(string(capability)))
		if capability == "" || seen[capability] {
			continue
		}
		seen[capability] = true
		caps = append(caps, capability)
	}
	sort.Slice(caps, func(i, j int) bool { return caps[i] < caps[j] })
	d.Capabilities = caps
	return d, nil
}

func (d Descriptor) clone() Descriptor {
	d.Capabilities = append([]Capability(nil), d.Capabilities...)
	return d
}

type Registry struct {
	mu      sync.RWMutex
	modules map[string]Descriptor
}

func NewRegistry() *Registry {
	return &Registry{modules: make(map[string]Descriptor)}
}

func (r *Registry) Register(descriptor Descriptor) error {
	if r == nil {
		return errors.New("原生模块注册表不存在")
	}
	normalized, err := descriptor.normalized()
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.modules[normalized.Name]; exists {
		return errors.New("原生模块重复注册：" + normalized.Name)
	}
	r.modules[normalized.Name] = normalized
	return nil
}

func (r *Registry) Lookup(name string) (Descriptor, bool) {
	if r == nil {
		return Descriptor{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	descriptor, ok := r.modules[name]
	if !ok {
		return Descriptor{}, false
	}
	return descriptor.clone(), true
}

func (r *Registry) Has(name string) bool {
	_, ok := r.Lookup(name)
	return ok
}

func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.modules))
	for name := range r.modules {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) Requires(name string, capability Capability) bool {
	descriptor, ok := r.Lookup(name)
	if !ok {
		return false
	}
	for _, required := range descriptor.Capabilities {
		if required == capability {
			return true
		}
	}
	return false
}
