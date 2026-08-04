// Package resource owns host resources held by a Huayan VM.
//
// A resource is intentionally opaque to the language. Native modules register
// a closer and receive a Handle; the VM can later close one handle or all
// handles during shutdown.
package resource

import (
	"errors"
	"io"
	"sync"
)

var (
	ErrInvalidHandle = errors.New("资源句柄无效")
	ErrManagerClosed = errors.New("资源管理器已经关闭")
	ErrNilCloser     = errors.New("资源关闭器不能为空")
	ErrEmptyName     = errors.New("资源名称不能为空")
)

type ID uint64

type Handle struct {
	manager *Manager
	id      ID
}

func (h Handle) Valid() bool {
	return h.manager != nil && h.id != 0
}

type entry struct {
	name   string
	closer io.Closer
}

type Manager struct {
	mu      sync.Mutex
	next    ID
	entries map[ID]entry
	order   []ID
	closed  bool
}

func NewManager() *Manager {
	return &Manager{entries: make(map[ID]entry)}
}

func (m *Manager) Register(name string, closer io.Closer) (Handle, error) {
	if m == nil {
		return Handle{}, ErrInvalidHandle
	}
	if name == "" {
		return Handle{}, ErrEmptyName
	}
	if closer == nil {
		return Handle{}, ErrNilCloser
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return Handle{}, ErrManagerClosed
	}
	m.next++
	if m.next == 0 {
		m.next++
	}
	handle := Handle{manager: m, id: m.next}
	m.entries[handle.id] = entry{name: name, closer: closer}
	m.order = append(m.order, handle.id)
	return handle, nil
}

func (m *Manager) IsOpen(handle Handle) bool {
	if m == nil || handle.manager != m || handle.id == 0 {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.entries[handle.id]
	return ok
}

func (m *Manager) Name(handle Handle) (string, bool) {
	if m == nil || handle.manager != m || handle.id == 0 {
		return "", false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.entries[handle.id]
	if !ok {
		return "", false
	}
	return entry.name, true
}

func (m *Manager) Close(handle Handle) error {
	if m == nil || handle.manager != m || handle.id == 0 {
		return ErrInvalidHandle
	}
	m.mu.Lock()
	entry, ok := m.entries[handle.id]
	if ok {
		delete(m.entries, handle.id)
	}
	m.mu.Unlock()
	if !ok {
		// Close is deliberately idempotent. This also makes cleanup paths
		// safe when both a finally block and VM shutdown run.
		return nil
	}
	return entry.closer.Close()
}

func (m *Manager) CloseAll() []error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	entries := make([]entry, 0, len(m.entries))
	for i := len(m.order) - 1; i >= 0; i-- {
		if current, ok := m.entries[m.order[i]]; ok {
			entries = append(entries, current)
		}
	}
	m.entries = make(map[ID]entry)
	m.order = nil
	m.mu.Unlock()

	var errs []error
	for _, entry := range entries {
		if err := entry.closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func (m *Manager) Count() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}
