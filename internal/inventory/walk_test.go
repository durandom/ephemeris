package inventory

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestWalkCountsAllocatedBlocksNotApparentSize(t *testing.T) {
	root := t.TempDir()
	sparse := filepath.Join(root, "sparse.raw")
	f, err := os.Create(sparse)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(4 * 1024 * 1024); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	w := NewWalker(nil)
	area := w.Walk(root)
	if area.Files != 1 {
		t.Fatalf("files = %d", area.Files)
	}
	got, err := allocatedBytes(sparse)
	if err != nil {
		t.Fatal(err)
	}
	if area.Bytes != got {
		t.Fatalf("bytes = %d, allocated = %d", area.Bytes, got)
	}
}

func TestWalkHonoursSkipPaths(t *testing.T) {
	root := t.TempDir()
	keep := filepath.Join(root, "keep")
	skip := filepath.Join(root, "skipme")
	if err := os.Mkdir(keep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(skip, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keep, "a"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skip, "b"), []byte("bbbbbbbb"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := NewWalker([]string{skip + "/"})
	area := w.Walk(root)
	if area.Files != 1 {
		t.Fatalf("files = %d want 1 (skipped subtree)", area.Files)
	}
}

func TestWalkDoesNotFollowSymlinkDir(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "x"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	area := NewWalker(nil).Walk(root)
	if area.Files != 1 {
		t.Fatalf("files = %d (followed symlink?)", area.Files)
	}
}

func TestWalkCountsEPERM(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can read mode 000 directories")
	}
	root := t.TempDir()
	denied := filepath.Join(root, "secret")
	if err := os.Mkdir(denied, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(denied, "hidden"), []byte("n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(denied, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(denied, 0o755) })
	area := NewWalker(nil).Walk(root)
	if area.EPERM == 0 {
		t.Fatalf("expected eperm, got %+v", area)
	}
}

func TestWalkHonoursExcludeXattr(t *testing.T) {
	root := t.TempDir()
	keep := filepath.Join(root, "keep")
	skip := filepath.Join(root, "xattr-skip")
	if err := os.Mkdir(keep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(skip, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keep, "a"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skip, "b"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := unix.Lsetxattr(skip, ExcludeXattr, []byte("1"), 0); err != nil {
		t.Skipf("cannot set xattr: %v", err)
	}
	area := NewWalker(nil).Walk(root)
	if area.Files != 1 {
		t.Fatalf("files = %d want 1 after xattr exclude, has_xattr=%v", area.Files, HasExcludeXattr(skip))
	}
}

func TestWalkAreasSkipsMissing(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f"), []byte("f"), 0o644); err != nil {
		t.Fatal(err)
	}
	areas := WalkAreas([]string{root, filepath.Join(root, "missing")}, nil)
	if len(areas) != 1 {
		t.Fatalf("areas = %d", len(areas))
	}
}
