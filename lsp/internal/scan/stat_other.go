//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd

package scan

import "os"

func inodeOf(_ os.FileInfo) int64 {
	return 0
}
