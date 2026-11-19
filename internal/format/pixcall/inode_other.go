//go:build !unix

package pixcall

import "os"

func fileInode(info os.FileInfo) *int64 {
	return nil
}
