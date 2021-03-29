package csif

import (
	"errors"
	"fmt"
	"os"

	"github.com/golang/glog"
)

type accessType int

const (
	stageVolumePath = "/csif-volumes"
)

type csifDriver struct {
	name     string
	version  string
	endpoint string
	nodeID   string
	volumes  map[string]csifVolume
}

type csifVolume struct {
	VolName       string     `json:"volName"`
	VolID         string     `json:"volID"`
	NodeID        string     `json:"nodeID"`
	VolSize       int64      `json:"volSize"`
	VolAccessType accessType `json:"volAccessType"`
	ParentVolID   string     `json:"parentVolID,omitempty"`
}

func NewCsifDriver(name, nodeID, endpoint string, version string) (*csifDriver, error) {
	if name == "" || endpoint == "" || nodeID == "" {
		return nil, errors.New("NewCsiFDriver: wrong args")
	}
	if version == "" {
		version = "notset"
	}

	if err := os.MkdirAll(stageVolumePath, 0750); err != nil {
		return nil, fmt.Errorf("mkdir: %s: %v", stageVolumePath, err)
	}
	glog.Info("Mkdir: %s", stageVolumePath)

	cf := &csifDriver{
		name:     name,
		version:  version,
		endpoint: endpoint,
		nodeID:   nodeID,

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
