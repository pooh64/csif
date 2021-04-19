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

type volAccessType int

const (
	volAccessMount volAccessType = iota
	volAccessBlock
)

// ControllerServer related info
type csifVolume struct {
	Name       string
	ID         string
	Size       int64
	AccessType volAccessType
	Disk       csifDisk
}

const (
	TopologyKeyNode = "topology.csif.csi/node"
)

type csifVolumeAttachment struct {
	Disk        csifDisk
	StagingPath string
}

type csifNodeServer struct {
	cd      *csifDriver
	mounter mount.SafeFormatAndMount
	volumes map[string]*csifVolumeAttachment
}

func NewCsifNodeServer(driver *csifDriver) *csifNodeServer {
	mounter := mount.SafeFormatAndMount{
		Interface: mount.New(""),
		Exec:      exec.New(),
	}

	return &csifNodeServer{
		cd:      driver,
		mounter: mounter,
		volumes: map[string]*csifVolumeAttachment{},
	}
}

type csifControllerServer struct {
	cd      *csifDriver
	volumes map[string]*csifVolume
}

func NewCsifControllerServer(driver *csifDriver) *csifControllerServer {
	return &csifControllerServer{
		cd:      driver,
		volumes: map[string]*csifVolume{},
	}
}

type csifDriver struct {
	name              string
	version           string
	endpoint          string
	nodeID            string
	maxVolumesPerNode int64

	ns *csifNodeServer

	diskTypes map[string]csifDiskNewFn
}

func NewCsifDriver(name, nodeID, endpoint string, version string, maxVolumesPerNode int64) (*csifDriver, error) {
	if name == "" || endpoint == "" || nodeID == "" {
		return nil, fmt.Errorf("wrong args")
	}
	if version == "" {
		version = "notset"
	}

	cf := &csifDriver{
		name:              name,
		version:           version,
		endpoint:          endpoint,
		nodeID:            nodeID,
		maxVolumesPerNode: maxVolumesPerNode,

		diskTypes: map[string]csifDiskNewFn{},
	}

	dtype, fn, err := RegisterHostImg()
	if err != nil {
		return nil, fmt.Errorf("failed to register %s driver: %v", csifHostImgName, err)
	}
	cf.diskTypes[dtype] = fn

	glog.Infof("New Driver: name=%v version=%v", name, version)

	return cf, nil
}

func (cd *csifDriver) Run() error {
	cd.ns = NewCsifNodeServer(cd)
	cs := NewCsifControllerServer(cd)

	server := NewNbServer()
	server.Start(cd.endpoint, cd, cs, cd.ns)
	server.Wait()
	return nil
}

func (cs *csifControllerServer) getVolumeByID(volID string) (*csifVolume, error) {
	if vol, ok := cs.volumes[volID]; ok {
		return vol, nil
	}
	return nil, fmt.Errorf("no volID=%s in volumes", volID)
}

func (ns *csifNodeServer) getVolumeAttachment(volID string) (*csifVolumeAttachment, error) {
	if vol, ok := ns.volumes[volID]; ok {
		return vol, nil
	}
	return nil, fmt.Errorf("no volID=%s in volumes", volID)
}

func (cs *csifControllerServer) getVolumeByName(volName string) (*csifVolume, error) {
	for _, vol := range cs.volumes {
		if vol.Name == volName {
			return vol, nil
		}
	}
	return nil, fmt.Errorf("no volName=%s in volumes", volName)
}

func (ns *csifNodeServer) createVolumeAttachment(req *csi.NodeStageVolumeRequest) (*csifVolumeAttachment, error) {
	disk, err := ns.cd.csifDiskAttach(req)
	if err != nil {
		return nil, fmt.Errorf("failed to attach disk: %v", err)
	}

	vol := &csifVolumeAttachment{
		Disk: disk,
	}
	ns.volumes[req.VolumeId] = vol
	return vol, nil
}

func (ns *csifNodeServer) deleteVolumeAttachment(volumeID string) error {
	vol := ns.volumes[volumeID]
	if err := vol.Disk.Detach(); err != nil {
		glog.Errorf("failed to detach disk while detaching pvc")
		return err
	}
	delete(ns.volumes, volumeID)
	return nil
}

func (cs *csifControllerServer) createVolume(req *csi.CreateVolumeRequest, accessType volAccessType) (*csifVolume, error) {
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

	disk, err := cs.cd.csifDiskCreate(req, volID)
	if err != nil {
		return nil, fmt.Errorf("failed to create disk: %v", err)
	}

	vol := &csifVolume{
		Name:       name,
		ID:         volID,
		Size:       req.CapacityRange.GetRequiredBytes(),
		AccessType: accessType,
		Disk:       disk,
	}
	cs.volumes[volID] = vol
	return vol, nil
}

func (cs *csifControllerServer) deleteVolume(volID string) error {
	glog.V(4).Infof("deleting csif volume: %s", volID)

	vol, err := cs.getVolumeByID(volID)
	if err != nil {
		glog.V(5).Infof("deleting nonexistent volume")
		return nil
	}

	if err := vol.Disk.Destroy(); err != nil {
		return fmt.Errorf("failed to disconnect disk: %v", err)
	}

	delete(cs.volumes, volID)
	return nil
}
