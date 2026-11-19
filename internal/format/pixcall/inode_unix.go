//go:build unix

package pixcall

import (
	"os"
	"syscall"
)

func fileInode(info os.FileInfo) *int64 {
	if info == nil {
		return nil
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st == nil || st.Ino == 0 {
		return nil
	}
	ino := int64(st.Ino)
	return &ino
}
