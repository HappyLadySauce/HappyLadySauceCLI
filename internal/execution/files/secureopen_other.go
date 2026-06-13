//go:build !windows && !unix

package files

import (
	"fmt"
	"os"
)

func openPathNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}

func openDirectoryNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}

func rejectLinkOrReparse(path string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("path is a symlink or reparse point: %s", path)
	}
	return nil
}

func pathHasReparsePoint(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}
