package csif

import (
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"k8s.io/mount-utils"
	"k8s.io/utils/exec"
)

const (
	mib = 1024 * 1024
)

const (
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

type csifDisk interface {
	Create(req *csi.CreateVolumeRequest, volID string) error
	Destroy() error
	Attach() (string, error)
	Detach() error
	GetPath() (string, error)
}
type csifVolume struct {
	Name       string
	ID         string
	Capacity   int64
	AccessType volAccessType

	StagingPath string

	Disk csifDisk
}

func NewCsifDriver(name, nodeID, endpoint string, version string, maxVolumesPerNode int64) (*csifDriver, error) {
	if name == "" || endpoint == "" || nodeID == "" {
		return nil, fmt.Errorf("wrong args")
	}
	if version == "" {
		version = "notset"
	}

	mounter := mount.SafeFormatAndMount{
		Interface: mount.New(""),
		Exec:      exec.New(),
	}

	if err := InitHostImages(); err != nil {
		return nil, fmt.Errorf("failed to init hostimages: %v", err)
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

func (cd *csifDriver) createVolume(req *csi.CreateVolumeRequest, accessType volAccessType) (*csifVolume, error) {
	name := req.GetName()
	glog.V(4).Infof("creating csif volume: %s", name)

	volID, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate uuid: %w", err)
	}

	switch accessType {
	case volAccessMount, volAccessBlock:
	default:
		return nil, fmt.Errorf("wrong access type %v", accessType)
	}

	disk := newCsifHostImg()
	if err := disk.Create(req, volID); err != nil {
		return nil, fmt.Errorf("failed to create disk: %v", err)
	}

	vol := csifVolume{
		Name:       name,
		ID:         volID,
		Capacity:   req.CapacityRange.GetRequiredBytes(),
		AccessType: accessType,
		Disk:       disk,
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

	if err := vol.Disk.Destroy(); err != nil {
		return fmt.Errorf("failed to destroy disk: %v", err)
	}

	delete(cd.volumes, volID)
	return nil
}
