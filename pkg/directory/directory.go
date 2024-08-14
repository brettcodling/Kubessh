package directory

import (
	"os"
	"path/filepath"
)

var Dir string

func init() {
	path, _ := os.Executable()
	Dir = filepath.Dir(path)
}
