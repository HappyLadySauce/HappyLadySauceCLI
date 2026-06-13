//go:build unix

package files

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openPathNoFollow(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("wrap file descriptor")
	}
	return file, nil
}

func openDirectoryNoFollow(path string) (*os.File, error) {
	return openPathNoFollow(path)
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
