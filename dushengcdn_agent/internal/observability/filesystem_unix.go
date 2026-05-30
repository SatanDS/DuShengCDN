//go:build !windows

package observability

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func statFilesystem(path string) (int64, int64) {
	if strings.TrimSpace(path) == "" {
		path = string(os.PathSeparator)
	}
	absPath := filepath.Clean(path)
	var stat syscall.Statfs_t
	if err := syscall.Statfs(absPath, &stat); err != nil {
		return 0, 0
	}
	total := int64(stat.Blocks) * int64(stat.Bsize)
	free := int64(stat.Bavail) * int64(stat.Bsize)
	used := total - free
	if used < 0 {
		used = 0
	}
	return total, used
}
