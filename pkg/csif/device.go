package csif

import (
	"fmt"
	"os"

	"k8s.io/kubernetes/pkg/volume/util/volumepathhandler"
	"k8s.io/utils/exec"
)

func createDiskImg(path string, size int64) error {
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

func destroyDiskImg(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove failed: %s: %v", path, err)
	}
	return nil
}

func (cd *csifDriver) createBDev(vol *csifVolume) (string, error) {
	volPathHandler := volumepathhandler.VolumePathHandler{}
	dev, err := volPathHandler.AttachFileDevice(vol.ImgPath)
	if err != nil {
		return "", fmt.Errorf("attachFileDevice failed: %v: %v", vol.ImgPath, err)
	}
	return dev, nil
}

func (cd *csifDriver) destroyBDev(vol *csifVolume) error {
	volPathHandler := volumepathhandler.VolumePathHandler{}
	if err := volPathHandler.DetachFileDevice(vol.ImgPath); err != nil {
		return fmt.Errorf("detachFileDevice failed: %s: %v", vol.ImgPath, err)
	}
	return nil
}

func (cd *csifDriver) getBDev(vol *csifVolume) (string, error) {
	volPathHandler := volumepathhandler.VolumePathHandler{}
	dev, err := volPathHandler.GetLoopDevice(vol.ImgPath)
	if err != nil {
		return "", fmt.Errorf("getLoopDevice failed: %s: %v", vol.ImgPath, err)
	}
	return dev, nil
}
