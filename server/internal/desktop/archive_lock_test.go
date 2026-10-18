package desktop

import (
	"context"
	"encoding/json"
	"github.com/gofrs/flock"
	"io"
	"path/filepath"
	"testing"
	"time"
)

func TestArchiveLease(t *testing.T) {
	directory := t.TempDir()
	input, hold := io.Pipe()
	ready, output := io.Pipe()
	defer input.Close()
	defer hold.Close()
	defer ready.Close()
	defer output.Close()
	result := make(chan error, 1)
	go func() { result <- HoldArchiveLock(context.Background(), directory, input, output) }()
	var info map[string]bool
	if err := json.NewDecoder(ready).Decode(&info); err != nil || !info["locked"] {
		t.Fatal("lease handshake", err)
	}
	other := flock.New(filepath.Join(directory, "owner.lock"))
	if ok, err := other.TryLock(); err != nil || ok {
		if ok {
			other.Unlock()
		}
		t.Fatal("lease did not exclude another owner", err)
	}
	hold.Close()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lease did not release on EOF")
	}
	if ok, err := other.TryLock(); err != nil || !ok {
		t.Fatal("lease not released", err)
	}
	other.Unlock()
}
