//go:build linux

// Private operator: inspect/evict only offline trial data files, never host caches.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

type residency struct {
	Files    int   `json:"files"`
	Bytes    int64 `json:"bytes"`
	Pages    int64 `json:"pages"`
	Resident int64 `json:"resident_pages"`
}

func resident(path string) (int64, int64, error) {
	f, e := os.Open(path)
	if e != nil {
		return 0, 0, e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return 0, 0, e
	}
	// WalkDir rejects symlinks; Linux stat adds a hard-link guard.
	var raw unix.Stat_t
	if e = unix.Fstat(int(f.Fd()), &raw); e != nil || raw.Nlink != 1 {
		return 0, 0, errors.New("unexpected linked file")
	}
	if raw.Uid != uint32(os.Geteuid()) {
		return 0, 0, errors.New("cache residency requires file ownership")
	}
	var pages, n int64
	// Bound virtual mappings and scratch space even for large extension files.
	for offset := int64(0); offset < info.Size(); offset += 64 << 20 {
		length := min(int64(64<<20), info.Size()-offset)
		b, e := unix.Mmap(int(f.Fd()), offset, int(length), unix.PROT_READ, unix.MAP_SHARED)
		if e != nil {
			return 0, 0, e
		}
		count := (length + int64(os.Getpagesize()) - 1) / int64(os.Getpagesize())
		v := make([]byte, int(count))
		_, _, errno := unix.Syscall(unix.SYS_MINCORE, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), uintptr(unsafe.Pointer(&v[0])))
		runtime.KeepAlive(b)
		runtime.KeepAlive(v)
		unmapErr := unix.Munmap(b)
		if errno != 0 {
			return 0, 0, errno
		}
		if unmapErr != nil {
			return 0, 0, unmapErr
		}
		pages += count
		for _, x := range v {
			if x&1 != 0 {
				n++
			}
		}
	}
	return pages, n, nil
}
func scan(root string, evict bool) (residency, error) {
	var r residency
	e := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("symlink refused")
		}
		if d.IsDir() {
			return nil
		}
		i, e := d.Info()
		if e != nil || !i.Mode().IsRegular() {
			return errors.New("nonregular file refused")
		}
		// Server-created lock files prove the cluster is not offline.
		if d.Name() == "postmaster.pid" {
			return errors.New("database must be stopped")
		}
		if evict {
			f, e := os.Open(path)
			if e != nil {
				return e
			}
			// Flush this owned, offline file only; never sync an entire filesystem.
			e = f.Sync()
			if e == nil {
				e = unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_DONTNEED)
			}
			f.Close()
			if e != nil {
				return e
			}
			return nil
		}
		p, n, e := resident(path)
		if e != nil {
			return e
		}
		r.Files++
		r.Bytes += i.Size()
		r.Pages += p
		r.Resident += n
		return nil
	})
	return r, e
}
func run() error {
	root := flag.String("root", "", "offline trial PGDATA mount")
	output := flag.String("output", "", "new aggregate report")
	evict := flag.Bool("evict", false, "request eviction of clean pages from this mount only")
	flag.Parse()
	if *root != "/cache/source/18/docker" && *root != "/cache/parade/18/docker" {
		return errors.New("only fixed trial mount paths are allowed")
	}
	var filesystem unix.Statfs_t
	if e := unix.Statfs(*root, &filesystem); e != nil || filesystem.Type == unix.TMPFS_MAGIC || filesystem.Type == unix.RAMFS_MAGIC {
		return errors.New("require disk-backed trial data")
	}
	if !strings.HasPrefix(*output, "/work/") || filepath.Clean(*output) != *output {
		return errors.New("require aggregate work output")
	}
	v, e := os.ReadFile(filepath.Join(*root, "PG_VERSION"))
	if e != nil || string(v) != "18\n" {
		return errors.New("require PostgreSQL 18 data")
	}
	if _, e = os.Lstat(filepath.Join(*root, "postmaster.pid")); !errors.Is(e, os.ErrNotExist) {
		return errors.New("database must be cleanly stopped")
	}
	before, e := scan(*root, false)
	if e != nil {
		return errors.New("residency scan failed")
	}
	if *evict {
		if _, e = scan(*root, true); e != nil {
			return errors.New("file-scoped eviction failed")
		}
	}
	after, e := scan(*root, false)
	if e != nil {
		return errors.New("final residency scan failed")
	}
	remaining := after.Resident * int64(os.Getpagesize())
	cold := remaining <= 1<<20
	report := map[string]any{"eviction_requested": *evict, "before": before, "after": after, "page_size": os.Getpagesize(), "resident_bytes_after": remaining, "at_most_one_mib_resident": cold, "host_cache_flush": false}
	f, e := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return errors.New("new report unavailable")
	}
	e = json.NewEncoder(f).Encode(report)
	f.Close()
	if e != nil {
		return errors.New("report write failed")
	}
	if *evict && !cold {
		return errors.New("residency gate failed")
	}
	fmt.Println("offline trial cache residency recorded")
	return nil
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
