package desktop

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
)

// rotatingLog bounds PostgreSQL stderr without asking users to maintain log files.
type rotatingLog struct {
	mu   sync.Mutex
	path string
	max  int64
	keep int
	file *os.File
	size int64
}

func openRotatingLog(path string, max int64, keep int) (*rotatingLog, error) {
	l := &rotatingLog{path: path, max: max, keep: keep}
	if err := l.open(); err != nil {
		return nil, err
	}
	return l, nil
}
func (l *rotatingLog) open() error {
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	size := stat.Size()
	if size > l.max {
		tail := make([]byte, int(l.max))
		if _, err = file.ReadAt(tail, size-l.max); err != nil {
			file.Close()
			return err
		}
		if err = file.Truncate(0); err != nil {
			file.Close()
			return err
		}
		if _, err = file.Write(tail); err != nil {
			file.Close()
			return err
		}
		size = l.max
	}
	l.file = file
	l.size = size
	return nil
}
func (l *rotatingLog) rotate() error {
	if err := l.file.Close(); err != nil {
		return err
	}
	l.file = nil
	for i := l.keep; i >= 1; i-- {
		target := l.path + "." + strconv.Itoa(i)
		source := l.path
		if i > 1 {
			source = l.path + "." + strconv.Itoa(i-1)
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(source, target); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return l.open()
}
func (l *rotatingLog) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	original := len(data)
	if int64(len(data)) > l.max {
		data = data[len(data)-int(l.max):]
	}
	if l.file == nil {
		return 0, fmt.Errorf("log is closed")
	}
	if l.size+int64(len(data)) > l.max {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := l.file.Write(data)
	l.size += int64(n)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return 0, err
	}
	return original, nil
}
func (l *rotatingLog) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}
