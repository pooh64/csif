package csif

import (
	"fmt"
	"os"

	"github.com/google/uuid"
	"k8s.io/utils/exec"
)

func cleanup(err *error, f func()) {
	if err != nil {
		f()
	}
}

func newUUID() (string, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func createImg(path string, size int64) error {
	executor := exec.New()

	// Check path and allocate if needed
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			out, err := executor.Command("fallocate", "-l", fmt.Sprintf("%dM", size/mib), path).CombinedOutput()
			if err != nil {
				return fmt.Errorf("fallocate failed: %v: %v", err, string(out))
			}
		} else {
			return fmt.Errorf("can't stat file: %v: %v", path, err)
		}
	}
	return nil
}

func destroyImg(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove failed: %s: %v", path, err)
	}
	return nil
}

func makeFile(pathname string) error {
	f, err := os.OpenFile(pathname, os.O_CREATE, os.FileMode(0644))
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	defer f.Close()
	return nil
}
