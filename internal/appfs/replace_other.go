//go:build !windows

package appfs

import "os"

func replaceFile(oldPath string, newPath string) error {
	return os.Rename(oldPath, newPath)
}
