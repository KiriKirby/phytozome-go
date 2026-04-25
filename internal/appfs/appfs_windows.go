package appfs

import "syscall"

func markHiddenIfSupported(path string) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	attrs, err := syscall.GetFileAttributes(ptr)
	if err != nil {
		return
	}
	if attrs&syscall.FILE_ATTRIBUTE_HIDDEN != 0 {
		return
	}
	_ = syscall.SetFileAttributes(ptr, attrs|syscall.FILE_ATTRIBUTE_HIDDEN)
}
