package catalog

import (
	"bufio"
	"container/heap"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// Exact external sorting checks source identities without retaining tens of
// millions of strings or building a second permanent PostgreSQL index.
// Runs contain only occurrence IDs, never names/addresses, and are still private.
type identities struct {
	directory string
	buffer    []string
	runs      []string
	capacity  int
}

func newIdentities(capacity int) (*identities, error) {
	dir, e := os.MkdirTemp("", "ukinfo-identities-")
	if e != nil {
		return nil, e
	}
	return &identities{directory: dir, capacity: capacity, buffer: make([]string, 0, capacity)}, nil
}
func (s *identities) close() { _ = os.RemoveAll(s.directory) }
func (s *identities) add(id string) error {
	s.buffer = append(s.buffer, id)
	if len(s.buffer) == s.capacity {
		return s.flush()
	}
	return nil
}
func (s *identities) flush() error {
	if len(s.buffer) == 0 {
		return nil
	}
	sort.Strings(s.buffer)
	file, e := os.CreateTemp(s.directory, "run-")
	if e != nil {
		return e
	}
	defer file.Close()
	w := bufio.NewWriterSize(file, 128<<10)
	for i, id := range s.buffer {
		if i > 0 && id == s.buffer[i-1] {
			return errors.New("duplicate source identity; import rejected")
		}
		if e = binary.Write(w, binary.LittleEndian, uint16(len(id))); e != nil {
			return e
		}
		if _, e = w.WriteString(id); e != nil {
			return e
		}
	}
	if e = w.Flush(); e != nil {
		return e
	}
	if e = file.Close(); e != nil {
		return e
	}
	s.runs = append(s.runs, filepath.Base(file.Name()))
	clear(s.buffer)
	s.buffer = s.buffer[:0]
	return nil
}

type identityRun struct {
	value  string
	file   *os.File
	reader *bufio.Reader
}

func (r *identityRun) next() error {
	var n uint16
	if e := binary.Read(r.reader, binary.LittleEndian, &n); e != nil {
		return e
	}
	if n == 0 || n > maxSourceIDBytes {
		return errors.New("invalid identity run")
	}
	b := make([]byte, n)
	if _, e := io.ReadFull(r.reader, b); e != nil {
		return e
	}
	r.value = string(b)
	return nil
}

type identityHeap []*identityRun

func (h identityHeap) Len() int           { return len(h) }
func (h identityHeap) Less(i, j int) bool { return h[i].value < h[j].value }
func (h identityHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *identityHeap) Push(v any)        { *h = append(*h, v.(*identityRun)) }
func (h *identityHeap) Pop() any          { old := *h; v := old[len(old)-1]; *h = old[:len(old)-1]; return v }
func (s *identities) verify(ctx context.Context) error {
	if e := s.flush(); e != nil {
		return e
	}
	h := identityHeap{}
	var opened []*os.File
	defer func() {
		for _, f := range opened {
			_ = f.Close()
		}
	}()
	for _, name := range s.runs {
		f, e := os.Open(filepath.Join(s.directory, name))
		if e != nil {
			return e
		}
		opened = append(opened, f)
		r := &identityRun{file: f, reader: bufio.NewReaderSize(f, 32<<10)}
		if e = r.next(); e != nil {
			return e
		}
		heap.Push(&h, r)
	}
	previous := ""
	n := 0
	for h.Len() > 0 {
		if n%4096 == 0 {
			if e := ctx.Err(); e != nil {
				return e
			}
		}
		n++
		r := heap.Pop(&h).(*identityRun)
		if r.value == previous {
			return errors.New("duplicate source identity; import rejected")
		}
		previous = r.value
		if e := r.next(); e == nil {
			heap.Push(&h, r)
		} else if e != io.EOF {
			return e
		}
	}
	return nil
}
