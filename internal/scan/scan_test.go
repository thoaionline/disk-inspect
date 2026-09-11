package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestScanTotalsAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{".hidden": "abc", "nested/data": "1234567"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(dir, filepath.Join(dir, "loop")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(dir, "broken")); err != nil {
		t.Fatal(err)
	}
	p := &Progress{}
	root, err := Scan(context.Background(), dir, p)
	if err != nil {
		t.Fatal(err)
	}
	if root.Files != 4 || root.Dirs != 1 || root.Errors != 0 {
		t.Fatalf("unexpected totals: %+v", root)
	}
	var apparent, disk int64
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		apparent += info.Size()
		disk += allocated(info)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if root.Apparent != apparent || root.Allocated != disk {
		t.Fatalf("got %d/%d, want %d/%d", root.Apparent, root.Allocated, apparent, disk)
	}
	if p.Entries.Load() != 6 || p.Bytes.Load() != disk {
		t.Fatalf("incorrect progress: %d entries, %d bytes", p.Entries.Load(), p.Bytes.Load())
	}
	for _, child := range root.Children {
		if child.Parent != root {
			t.Fatal("missing parent")
		}
		if child.Name == "loop" && (!child.Symlink || child.Dir || len(child.Children) != 0) {
			t.Fatal("followed symlink")
		}
	}
}

func TestSparseFile(t *testing.T) {
	if !HasAllocated {
		t.Skip("allocated sizes unavailable")
	}
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "sparse"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(64 << 20); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	root, err := Scan(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := root.Children[0]
	if n.Apparent != 64<<20 {
		t.Fatalf("apparent = %d", n.Apparent)
	}
	if n.Allocated >= n.Apparent {
		t.Skip("filesystem does not expose sparse allocation")
	}
	if n.Size(false) != n.Allocated || n.Size(true) != n.Apparent {
		t.Fatal("size mode is incorrect")
	}
}

func TestCancellationAndInvalidRoots(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, dir, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if _, err := Scan(context.Background(), filepath.Join(dir, "missing"), nil); err == nil {
		t.Fatal("missing root accepted")
	}
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Scan(context.Background(), path, nil); err == nil {
		t.Fatal("file root accepted")
	}
}

func TestUnreadableDirectory(t *testing.T) {
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0700) })
	if _, err := os.ReadDir(locked); err == nil {
		t.Skip("user can read mode-000 directories")
	}
	root, err := Scan(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if root.Errors != 1 || root.Children[0].Error == "" {
		t.Fatalf("read error not reported: %+v", root)
	}
	if _, err := Scan(context.Background(), locked, nil); err == nil {
		t.Fatal("unreadable root should fail")
	}
}

func TestHardLinksCountPerPath(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	if err := os.WriteFile(a, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(a, filepath.Join(dir, "b")); err != nil {
		t.Fatal(err)
	}
	root, err := Scan(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if root.Files != 2 || root.Children[0].Apparent != 5 || root.Children[1].Apparent != 5 {
		t.Fatalf("unexpected hard-link accounting: %+v", root)
	}
}
