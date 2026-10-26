//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestOfflineFileCacheGuardAndEviction(t *testing.T) {
	root := t.TempDir()
	var filesystem unix.Statfs_t
	if err := unix.Statfs(root, &filesystem); err != nil {
		t.Fatal(err)
	}
	if filesystem.Type == unix.TMPFS_MAGIC || filesystem.Type == unix.RAMFS_MAGIC {
		t.Skip("requires a disk-backed cache fixture")
	}
	path := filepath.Join(root, "fixture")
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.Write(make([]byte, 1<<20)); e != nil {
		t.Fatal(e)
	}
	if e = f.Sync(); e != nil {
		t.Fatal(e)
	}
	f.Close()
	before, e := scan(root, false)
	if e != nil || before.Files != 1 || before.Pages != (1<<20)/int64(os.Getpagesize()) {
		t.Fatal("file residency", before, e)
	}
	if _, e = scan(root, true); e != nil {
		t.Fatal(e)
	}
	after, e := scan(root, false)
	if e != nil || after.Resident != 0 {
		t.Fatal("clean fixture not evicted", after, e)
	}
	large := filepath.Join(root, "sparse-window-boundary")
	window, e := os.OpenFile(large, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		t.Fatal(e)
	}
	if e = window.Truncate((64 << 20) + 4097); e != nil {
		t.Fatal(e)
	}
	window.Close()
	pages, _, e := resident(large)
	expectedPages := ((64 << 20) + 4097 + int64(os.Getpagesize()) - 1) / int64(os.Getpagesize())
	if e != nil || pages != expectedPages {
		t.Fatal("bounded mapping lost a final partial page", pages, e)
	}
	os.Remove(large)
	link := filepath.Join(root, "symlink")
	if e = os.Symlink(path, link); e != nil {
		t.Fatal(e)
	}
	if _, e = scan(root, false); e == nil {
		t.Fatal("symlink accepted")
	}
	os.Remove(link)
	hard := filepath.Join(root, "hardlink")
	if e = os.Link(path, hard); e != nil {
		t.Fatal(e)
	}
	if _, e = scan(root, false); e == nil {
		t.Fatal("hardlink accepted")
	}
	os.Remove(hard)
	if e = os.WriteFile(filepath.Join(root, "postmaster.pid"), []byte("1\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = scan(root, false); e == nil {
		t.Fatal("running database accepted")
	}
}
