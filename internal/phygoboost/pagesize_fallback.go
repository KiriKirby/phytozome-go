//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris)

package phygoboost

import "os"

func pageSize() int64 {
	return int64(os.Getpagesize())
}

