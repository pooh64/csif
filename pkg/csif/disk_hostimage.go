package csif

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/volume/util/volumepathhandler"
)

const (
	csifHostImagesPath  = "/csi-csif-hostimg"
	csifDiskTypeHostImg = "hostimg"
)

type csifDiskHostImg struct {
	Size    int64  `json:"diskSize,string"`
	ImgPath string `json:"diskImgPath"`
}

func RegisterDiskTypeHostImg() (string, csifDiskNewFn, error) {
	if err := os.MkdirAll(csifHostImagesPath, 0750); err != nil {
		return "", nil, fmt.Errorf("mkdir: %s: %v", csifHostImagesPath, err)
	}
	glog.Infof("Mkdir: %s", csifHostImagesPath)
	return csifDiskTypeHostImg, newCsifDiskHostImg, nil
}

func newCsifDiskHostImg() csifDisk {
	return &csifDiskHostImg{}
}

func (hi *csifDiskHostImg) GetType() string {
	return csifDiskTypeHostImg
}

func (hi *csifDiskHostImg) Create(req *csi.CreateVolumeRequest, volID string) error {
	hi.Size = req.CapacityRange.GetRequiredBytes()
	hi.ImgPath = filepath.Join(csifHostImagesPath, volID)
	// req.GetParameters()[...]
	return nil
}

func (hi *csifDiskHostImg) Destroy() error {
	return nil
}

func (hi *csifDiskHostImg) Attach(req *csi.NodeStageVolumeRequest) (string, error) {
	if err := createImg(hi.ImgPath, hi.Size); err != nil {
		return "", fmt.Errorf("create disk img failed: %v", err)
	}
	volPathHandler := volumepathhandler.VolumePathHandler{}
	// TODO: make sure path exists
	dev, err := volPathHandler.AttachFileDevice(hi.ImgPath)
	if err != nil {
		return "", fmt.Errorf("attachFileDevice failed: %v: %v", hi.ImgPath, err)
	}
	return dev, nil
}

func (hi *csifDiskHostImg) Detach() error {
	volPathHandler := volumepathhandler.VolumePathHandler{}
	if err := volPathHandler.DetachFileDevice(hi.ImgPath); err != nil {
		return fmt.Errorf("detachFileDevice failed: %s: %v", hi.ImgPath, err)
	}
	if err := destroyImg(hi.ImgPath); err != nil {
		return fmt.Errorf("destroy disk img failed: %v", err)
	}
	return nil
}

func (hi *csifDiskHostImg) GetPath() (string, error) {
	volPathHandler := volumepathhandler.VolumePathHandler{}
	dev, err := volPathHandler.GetLoopDevice(hi.ImgPath)
	if err != nil {
		return "", fmt.Errorf("getLoopDevice failed: %s: %v", hi.ImgPath, err)
	}
	return dev, nil
}

/*
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
*/
