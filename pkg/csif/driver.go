package csif

import (
	"fmt"

	"github.com/golang/glog"
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

	dtype, fn, err := RegisterDiskTypeHostImg()
	if err != nil {
		return nil, fmt.Errorf("failed to register DiskHostImg driver: %v", err)
	}
	cf.diskTypes[dtype] = fn

	dtype, fn, err = RegisterDiskTypeISCSI()
	if err != nil {
		return nil, fmt.Errorf("failed to register DiskISCSI driver: %v", err)
	}
	cf.diskTypes[dtype] = fn

	glog.Infof("New Driver: name=%v version=%v", name, version)

	return cf, nil
}

func (cd *csifDriver) Run() error {
	cd.ns = newCsifNodeServer(cd)
	cs := NewCsifControllerServer(cd)

	server := NewNbServer()
	server.Start(cd.endpoint, cd, cs, cd.ns)
	server.Wait()
	return nil
}
