//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package scan

import (
	"os"
	"syscall"
)

const HasAllocated = true

func allocated(info os.FileInfo) int64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return int64(stat.Blocks) * 512
	}
	return info.Size()
}
