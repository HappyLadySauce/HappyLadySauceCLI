//go:build windows

package files

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func openPathNoFollow(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("wrap file handle")
	}
	return file, nil
}

func openDirectoryNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}

func rejectLinkOrReparse(path string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 || pathHasReparsePoint(path) {
		return fmt.Errorf("path is a symlink or reparse point: %s", path)
	}
	return nil
}

func pathHasReparsePoint(path string) bool {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attrs, err := windows.GetFileAttributes(name)
	if err != nil {
		return false
	}
	return attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
