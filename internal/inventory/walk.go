package inventory

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const ExcludeXattr = "com.apple.metadata:com_apple_backup_excludeItem"

type Area struct {
	Path  string
	Files int64
	Bytes int64
	EPERM int64
}

type Walker struct {
	Skip   map[string]struct{}
	Xattr  func(path string) bool
	Blocks func(path string) (int64, error)
}

func NewWalker(skipPaths []string) *Walker {
	return &Walker{
		Skip:   skipSet(skipPaths),
		Xattr:  HasExcludeXattr,
		Blocks: allocatedBytes,
	}
}

func (w *Walker) excluded(path string) bool {
	if _, ok := w.Skip[normalizePath(path)]; ok {
		return true
	}
	if w.Xattr != nil && w.Xattr(path) {
		return true
	}
	return false
}

func (w *Walker) Walk(root string) Area {
	area := Area{Path: root}
	info, err := os.Lstat(root)
	if err != nil {
		if isPerm(err) {
			area.EPERM++
		}
		return area
	}
	if !info.IsDir() || isSymlink(info) {
		return area
	}
	if w.excluded(root) {
		return area
	}

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if isPerm(err) {
				area.EPERM++
			}
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if path == root {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if w.excluded(path) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		n, berr := w.blocks(path)
		if berr != nil {
			if isPerm(berr) {
				area.EPERM++
			}
			return nil
		}
		area.Files++
		area.Bytes += n
		return nil
	})
	return area
}

func (w *Walker) blocks(path string) (int64, error) {
	if w.Blocks != nil {
		return w.Blocks(path)
	}
	return allocatedBytes(path)
}

func allocatedBytes(path string) (int64, error) {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return 0, err
	}
	return st.Blocks * 512, nil
}

func HasExcludeXattr(path string) bool {
	_, err := unix.Lgetxattr(path, ExcludeXattr, nil)
	if err == nil || errors.Is(err, unix.ERANGE) {
		return true
	}
	return false
}

func isPerm(err error) bool {
	return errors.Is(err, os.ErrPermission) || errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM)
}

func isSymlink(info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}

func WalkAreas(roots []string, skip []string) []Area {
	w := NewWalker(skip)
	var out []Area
	for _, root := range roots {
		info, err := os.Lstat(root)
		if err != nil || !info.IsDir() || isSymlink(info) || w.excluded(root) {
			continue
		}
		out = append(out, w.Walk(root))
	}
	return out
}
