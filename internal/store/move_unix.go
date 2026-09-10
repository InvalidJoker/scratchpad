//go:build !windows

package store

import "syscall"

// errCrossDevice is the errno os.Rename reports when src and dst are on
// different filesystems.
const errCrossDevice = syscall.EXDEV
