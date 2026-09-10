package store

import (
	"io/fs"
	"path/filepath"
)

// DirSize sums the bytes a directory occupies.
//
// Unlike activity scanning, this counts everything — node_modules included.
// The number exists to answer "how much space do I get back?", and that space
// is real regardless of how uninteresting the files are.
func DirSize(dir string) int64 {
	var total int64
	// A walk error means one subtree could not be measured; an approximate
	// total is more useful here than no total at all.
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}
