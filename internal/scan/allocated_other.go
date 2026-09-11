//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package scan

import "os"

const HasAllocated = false

func allocated(info os.FileInfo) int64 { return info.Size() }
