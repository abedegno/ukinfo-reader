package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBoundedLog(t *testing.T) {
	directory := t.TempDir()
	file := filepath.Join(directory, "postgres.log")
	log, err := openRotatingLog(file, 16, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, err = log.Write([]byte("invented log message longer than limit")); err != nil {
			t.Fatal(err)
		}
	}
	log.Close()
	files, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatal("retention", len(files))
	}
	for _, entry := range files {
		info, e := entry.Info()
		if e != nil || info.Size() > 16 {
			t.Fatal("unbounded log", e)
		}
	}
}
