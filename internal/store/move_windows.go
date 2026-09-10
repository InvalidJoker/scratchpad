//go:build windows

package store

import "syscall"

// errCrossDevice is what Windows reports when a rename would cross volumes.
const errCrossDevice = syscall.Errno(17) // ERROR_NOT_SAME_DEVICE
