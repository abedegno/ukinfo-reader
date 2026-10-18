package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/gofrs/flock"
)

// HoldArchiveLock prevents another app/CLI owner from starting PostgreSQL during a
// cold copy. The Electron owner holds stdin open and releases the lease on EOF.
func HoldArchiveLock(ctx context.Context, directory string, input io.Reader, output io.Writer) error {
	lock := flock.New(filepath.Join(directory, "owner.lock"))
	ok, err := lock.TryLock()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("archive is owned by another process; files preserved")
	}
	defer lock.Unlock()
	if err := json.NewEncoder(output).Encode(map[string]bool{"locked": true}); err != nil {
		return err
	}
	closed := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, input); close(closed) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-closed:
		return nil
	}
}
