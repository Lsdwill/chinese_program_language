package source

import (
	"os"
	"strings"
)

// File is the immutable source text used by every diagnostic and bytecode span.
type File struct {
	ID   int
	Name string
	Text string
}

type Manager struct {
	next  int
	files map[string]*File
}

func NewManager() *Manager { return &Manager{next: 1, files: map[string]*File{}} }
func (m *Manager) Add(name, text string) *File {
	if f, ok := m.files[name]; ok {
		return f
	}
	f := &File{ID: m.next, Name: name, Text: text}
	m.next++
	m.files[name] = f
	return f
}
func (m *Manager) Load(path string) (*File, error) {
	if f, ok := m.files[path]; ok {
		return f, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return m.Add(path, string(b)), nil
}
func (m *Manager) Get(name string) (*File, bool) { f, ok := m.files[name]; return f, ok }

type Span struct {
	File  *File
	Start int
	End   int
}

func (s Span) Valid() bool { return s.File != nil && s.Start >= 0 }

func (f *File) LineColumn(offset int) (line, column int) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(f.Text) {
		offset = len(f.Text)
	}
	line, last := 1, 0
	for i := 0; i < offset; {
		if f.Text[i] == '\r' {
			line++
			if i+1 < offset && f.Text[i+1] == '\n' {
				i++
			}
			last = i + 1
		} else if f.Text[i] == '\n' {
			line++
			last = i + 1
		}
		i++
	}
	// Columns are Unicode code-point columns, as specified by 华言.
	column = len([]rune(f.Text[last:offset])) + 1
	return
}

func (f *File) LineText(line int) string {
	if line < 1 {
		return ""
	}
	text := strings.ReplaceAll(strings.ReplaceAll(f.Text, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(text, "\n")
	if line > len(lines) {
		return ""
	}
	return lines[line-1]
}
