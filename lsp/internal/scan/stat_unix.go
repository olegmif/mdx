//go:build linux || darwin || freebsd || netbsd || openbsd

package scan

import (
	"os"
	"syscall"
)

func inodeOf(fi os.FileInfo) int64 {
	sys, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return int64(sys.Ino)
}
