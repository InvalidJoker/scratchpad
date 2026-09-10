package store

// CopyTreeForTest exposes the cross-device copy path, which os.Rename would
// otherwise hide on a single-filesystem test machine.
func CopyTreeForTest(src, dst string) error { return copyTree(src, dst) }
