package csif

import (
	"encoding/json"
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"k8s.io/mount-utils"
	"k8s.io/utils/exec"
)

const (
	mib = 1024 * 1024
)

type csifDiskNewFn = func() csifDisk

type csifDisk interface {
	Connect(req *csi.CreateVolumeRequest, volID string) error
	Disconnect() error
	Attach() (string, error)
	Detach() error
	GetPath() (string, error)
	GetType() string
	// VerifyParam() TODO: idempotent CS
}

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

	fn, err := RegisterHostImg()
	if err != nil {
		return nil, fmt.Errorf("failed to register %s driver: %v", csifHostImgName, err)
	}
	cf.diskTypes[csifHostImgName] = fn

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

func (cd *csifDriver) newDisk(dtype string) (csifDisk, error) {
	diskFn, ok := cd.diskTypes[dtype]
	if !ok {
		return nil, fmt.Errorf("diskType %s is not supported", dtype)
	}
	return diskFn(), nil
}

func (cd *csifDriver) loadDiskJson(dtype string, jsonData string) (csifDisk, error) {
	disk, err := cd.newDisk(dtype)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(jsonData), disk); err != nil {
		glog.Fatalf("failed to unmarshal json disk data: %v", err)
		return nil, err
	}
	return disk, nil
}

func (ns *csifNodeServer) createVolumeAttachment(req *csi.NodeStageVolumeRequest) (*csifVolumeAttachment, error) {
	dtype, ok := req.GetVolumeContext()["diskType"]
	if !ok {
		return nil, fmt.Errorf("no diskType in volumeContext")
	}
	data, ok := req.GetVolumeContext()["diskInfo"]
	if !ok {
		return nil, fmt.Errorf("no diskInfo in volumeContext")
	}

	disk, err := ns.cd.loadDiskJson(dtype, data)
	if err != nil {
		return nil, err
	}
	vol := &csifVolumeAttachment{
		Disk: disk,
	}
	ns.volumes[req.VolumeId] = vol
	return vol, nil
}

func (ns *csifNodeServer) deleteVolumeAttachment(volumeID string) error {
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

	dtype, ok := req.GetParameters()["diskType"]
	if !ok {
		return nil, fmt.Errorf("missing diskType volume parameter")
	}
	disk, err := cs.cd.newDisk(dtype)
	if err != nil {
		return nil, err
	}

	if err := disk.Connect(req, volID); err != nil {
		return nil, fmt.Errorf("failed to connect disk: %v", err)
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

	if err := vol.Disk.Disconnect(); err != nil {
		return fmt.Errorf("failed to disconnect disk: %v", err)
	}

	delete(cs.volumes, volID)
	return nil
}
