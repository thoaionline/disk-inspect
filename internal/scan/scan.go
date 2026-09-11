// Package scan measures a directory tree without following symbolic links.
package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
)

// Node owns its children. Totals include the entry itself and all descendants.
// Hard links are counted per pathname, as are shared/reflinked blocks.
type Node struct {
	Name      string  `json:"name"`
	Path      string  `json:"path"`
	Dir       bool    `json:"directory"`
	Symlink   bool    `json:"symlink"`
	Apparent  int64   `json:"apparent_bytes"`
	Allocated int64   `json:"allocated_bytes"`
	Files     int64   `json:"files"`
	Dirs      int64   `json:"directories"`
	Errors    int64   `json:"errors"`
	Error     string  `json:"error,omitempty"`
	Children  []*Node `json:"-"`
	Parent    *Node   `json:"-"`
}

func (n *Node) Size(apparent bool) int64 {
	if apparent {
		return n.Apparent
	}
	return n.Allocated
}

// Progress is safe to read while Scan runs. Published nodes are only read after
// Scan returns; the UI never observes a partially mutated tree.
type Progress struct {
	Entries atomic.Int64
	Bytes   atomic.Int64
	Errors  atomic.Int64
}

func Scan(ctx context.Context, path string, p *Progress) (*Node, error) {
	if p == nil {
		p = &Progress{}
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// Resolve the explicitly chosen root; symlinks below it are never followed.
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", path)
	}
	root, err := walk(ctx, path, info, nil, p)
	if err != nil {
		return nil, err
	}
	if root.Error != "" {
		return nil, fmt.Errorf("cannot read %s: %s", path, root.Error)
	}
	return root, nil
}

func walk(ctx context.Context, path string, info os.FileInfo, parent *Node, p *Progress) (*Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	n := &Node{Name: filepath.Base(path), Path: path, Dir: info.IsDir(), Symlink: info.Mode()&os.ModeSymlink != 0, Apparent: info.Size(), Allocated: allocated(info), Parent: parent}
	p.Entries.Add(1)
	p.Bytes.Add(n.Allocated)
	if !n.Dir {
		n.Files = 1
		return n, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		n.Error = err.Error()
		n.Errors++
		p.Errors.Add(1)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		childPath := filepath.Join(path, entry.Name())
		info, err := entry.Info()
		var child *Node
		if err != nil {
			child = &Node{Name: entry.Name(), Path: childPath, Dir: entry.IsDir(), Parent: n, Error: err.Error(), Errors: 1}
			p.Errors.Add(1)
			p.Entries.Add(1)
		} else {
			child, err = walk(ctx, childPath, info, n, p)
			if err != nil {
				return nil, err
			}
		}
		n.Children = append(n.Children, child)
		n.Apparent += child.Apparent
		n.Allocated += child.Allocated
		n.Files += child.Files
		n.Dirs += child.Dirs
		if child.Dir {
			n.Dirs++
		}
		n.Errors += child.Errors
	}
	return n, nil
}
