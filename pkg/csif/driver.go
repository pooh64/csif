package csif

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"k8s.io/mount-utils"
	"k8s.io/utils/exec"
)

const (
	mib = 1024 * 1024
)

const (
	csifImagesPath  = "/csi-csif-images"
	TopologyKeyNode = "topology.csif.csi/node"
)

type csifDriver struct {
	name              string
	version           string
	endpoint          string
	nodeID            string
	volumes           map[string]csifVolume
	maxVolumesPerNode int64

	mounter *mount.SafeFormatAndMount
}

type volAccessType int

const (
	volAccessMount volAccessType = iota
	volAccessBlock
)

type csifVolume struct {
	Name        string
	ID          string
	ImgPath     string
	Size        int64
	AccessType  volAccessType
	NodeID      string
	StagingPath string
}

func NewCsifDriver(name, nodeID, endpoint string, version string, maxVolumesPerNode int64) (*csifDriver, error) {
	if name == "" || endpoint == "" || nodeID == "" {
		return nil, fmt.Errorf("wrong args")
	}
	if version == "" {
		version = "notset"
	}

	if err := os.MkdirAll(csifImagesPath, 0750); err != nil {
		return nil, fmt.Errorf("mkdir: %s: %v", csifImagesPath, err)
	}
	glog.Infof("Mkdir: %s", csifImagesPath)

	mounter := mount.SafeFormatAndMount{
		Interface: mount.New(""),
		Exec:      exec.New(),
	}

	cf := &csifDriver{
		name:              name,
		version:           version,
		endpoint:          endpoint,
		nodeID:            nodeID,
		mounter:           &mounter,
		maxVolumesPerNode: maxVolumesPerNode,

		volumes: map[string]csifVolume{},
	}
	glog.Infof("New Driver: name=%v version=%v", name, version)

	return cf, nil
}

func (cd *csifDriver) Run() error {
	server := NewNbServer()
	server.Start(cd.endpoint, cd, cd, cd)
	server.Wait()
	return nil
}

func (cd *csifDriver) getVolumeByID(volID string) (*csifVolume, error) {
	if vol, ok := cd.volumes[volID]; ok {
		return &vol, nil
	}
	return nil, fmt.Errorf("no volID=%s in volumes", volID)
}

func (cd *csifDriver) getVolumeByName(volName string) (*csifVolume, error) {
	for _, vol := range cd.volumes {
		if vol.Name == volName {
			return &vol, nil
		}
	}
	return nil, fmt.Errorf("no volName=%s in volumes", volName)
}

func (cd *csifDriver) createVolume(req *csi.CreateVolumeRequest, cap int64, accessType volAccessType) (*csifVolume, error) {
	name := req.GetName()
	glog.V(4).Infof("creating csif volume: %s", name)

	volID, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate uuid: %w", err)
	}

	imgPath := filepath.Join(csifImagesPath, volID)
	/*
		imgPath, ok := req.GetParameters()["imgPath"]
		if !ok {
			imgPath := filepath.Join(volImgPath, volID)
		}
	*/

	switch accessType {
	case volAccessMount, volAccessBlock: // TODO: start iscsi target
		if err := createDiskImg(imgPath, cap); err != nil {
			return nil, fmt.Errorf("create disk img failed: %v", err)
		}
	default:
		return nil, fmt.Errorf("wrong access type %v", accessType)
	}

	vol := csifVolume{
		Name:       name,
		ID:         volID,
		ImgPath:    imgPath,
		Size:       cap,
		AccessType: accessType,
	}
	cd.volumes[volID] = vol
	return &vol, nil
}

func (cd *csifDriver) deleteVolume(volID string) error {
	glog.V(4).Infof("deleting csif volume: %s", volID)

	vol, err := cd.getVolumeByID(volID)
	if err != nil {
		glog.V(5).Infof("deleting nonexistent volume")
		return nil
	}

	switch vol.AccessType {
	case volAccessMount, volAccessBlock: // TODO: shutdown iscsi target
		path := vol.ImgPath
		if err := destroyDiskImg(path); err != nil {
			return fmt.Errorf("destroy disk img failed: %v", err)
		}
	}

	delete(cd.volumes, volID)
	return nil
}
