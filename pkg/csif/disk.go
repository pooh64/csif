package csif

import (
	"encoding/json"
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
)

type csifDiskNewFn = func() csifDisk

const (
	csifDiskTypeParam = "diskType"
)

// Usage: High level:
// CS: csifDiskCreate() -> csifDiskGetAttributes() - write in PV context -> Destroy()
// NS: csifDiskAttach() -> GetPath() -> Detach()
type csifDisk interface {
	GetType() string
	// TODO:
	// GetConfigIn() interface{}
	// GetConfigOut() interface{}

	// CS routine: process sclass volumeAttributes: check, provision
	Create(req *csi.CreateVolumeRequest, volID string) error
	// CS routine: delete created disk
	Destroy() error
	// TODO: idempotent CS
	// VerifyParam(req *csi.CreateVolumeRequest) error

	// NS routine: attach disk as block device
	Attach(req *csi.NodeStageVolumeRequest) (string, error)
	// NS routine: detack disk block device, close connection
	Detach() error
	// NS routine: get block device path
	GetPath() (string, error)
}

// CS routine: create disk from storageclass volumeAttributes
// Needs Destroy()
func (cd *csifDriver) csifDiskCreate(req *csi.CreateVolumeRequest, volID string) (csifDisk, error) {
	disk, err := cd.newCsifDisk(req.GetParameters())
	if err != nil {
		return nil, err
	}

	if err := disk.Create(req, volID); err != nil {
		return nil, fmt.Errorf("disk.Create failed: %v", err)
	}
	return disk, nil
}

// CS routine: save disk claim info to PV volumeAttributes
func (cd *csifDriver) csifDiskGetAttributes(disk csifDisk) map[string]string {
	jbyt, err := json.Marshal(interface{}(disk))
	if err != nil {
		panic(err)
	}
	attr := make(map[string]string)
	if err := json.Unmarshal([]byte(jbyt), &attr); err != nil {
		panic(err)
	}
	attr[csifDiskTypeParam] = disk.GetType()
	return attr
}

// NS routine: load PV context and attach disk claim
// Needs Detach()
func (cd *csifDriver) csifDiskAttach(req *csi.NodeStageVolumeRequest) (csifDisk, error) {
	attr := req.GetVolumeContext()
	disk, err := cd.newCsifDisk(attr)
	if err != nil {
		return nil, err
	}

	jbyt, _ := json.Marshal(attr)
	if err := json.Unmarshal([]byte(jbyt), disk); err != nil {
		glog.Errorf("failed to unmarshal disk data")
		return nil, err
	}

	if _, err := disk.Attach(req); err != nil {
		return nil, err
	}
	return disk, nil
}

// Internal, do not use
func (cd *csifDriver) newCsifDisk(attr map[string]string) (csifDisk, error) {
	dtype, ok := attr[csifDiskTypeParam]
	if !ok {
		return nil, fmt.Errorf("%s field is not presented", csifDiskTypeParam)
	}
	diskFn, ok := cd.diskTypes[dtype]
	if !ok {
		return nil, fmt.Errorf("%s %s is not supported", csifDiskTypeParam, dtype)
	}
	return diskFn(), nil
}
