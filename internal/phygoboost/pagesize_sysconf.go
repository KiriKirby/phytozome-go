//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris

package phygoboost

import (
	"os"

	"github.com/tklauser/go-sysconf"
)

func pageSize() int64 {
	if size, err := sysconf.Sysconf(sysconf.SC_PAGE_SIZE); err == nil && size > 0 {
		return size
	}
	return int64(os.Getpagesize())
}

