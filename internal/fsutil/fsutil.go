package fsutil

import (
	"fmt"
	"os"

	gollyfsutil "github.com/tav/golly/fsutil"
)

const (
	dirPerms = 0700
)

func DefaultRootDir() string {
	return os.ExpandEnv("$HOME/.blockmania")
}

func EnsureDir(path string) error {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.Mkdir(path, dirPerms)
		}
		return err
	}
	return nil
}

func CreateUnlessExists(path string) error {
	if exists, _ := gollyfsutil.Exists(path); exists {
		return fmt.Errorf("Directory `%v` already exists", path)
	}
	if err := os.Mkdir(path, dirPerms); err != nil {
		return fmt.Errorf("Could not create directory `%v` (%v)", path, err)
	}
	return nil
}
