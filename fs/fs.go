package fs

import (
	i_fs "io/fs"
	"os"
	"path/filepath"
)

// FS is an interface abstracting the file system operations used by the scanner.
// This allows for testable code by mocking the file system.
type FS interface {
	Stat(name string) (i_fs.FileInfo, error)
	ReadDir(name string) ([]i_fs.DirEntry, error)
	ReadFile(name string) ([]byte, error)
	WalkDir(root string, fn i_fs.WalkDirFunc) error
}

// osFS implements FS using the underlying os package. This is the default
// implementation used for real file system operations.
type osFS struct{}

// NewOSFS creates a new osFS instance.
func NewOSFS() FS {
	return &osFS{}
}

func (f *osFS) Stat(name string) (i_fs.FileInfo, error) {
	return os.Stat(name)
}

func (f *osFS) ReadDir(name string) ([]i_fs.DirEntry, error) {
	return os.ReadDir(name)
}

func (f *osFS) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (f *osFS) WalkDir(root string, fn i_fs.WalkDirFunc) error {
	return filepath.WalkDir(root, fn)
}
