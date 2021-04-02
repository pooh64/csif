package csif

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang/glog"
)

const (
	stageVolumePath = "/csif-volumes"
	TopologyKeyNode = "topology.csif.csi/node"
)

type csifDriver struct {
	name     string
	version  string
	endpoint string
	nodeID   string
	volumes  map[string]csifVolume
}

type volAccessType int

const (
	volAccessMount volAccessType = iota
	volAccessBlock
)

type csifVolume struct {
	Name        string        `json:"Name"`
	ID          string        `json:"ID"`
	Path        string        `json:"Path"`
	Size        int64         `json:"Size"`
	AccessType  volAccessType `json:"AccessType"`
	NodeID      string        `json:"NodeID"`
	ParentVolID string        `json:"ParentVolID,omitempty"`
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

func getVolumePath(volID string) string {
	return filepath.Join(stageVolumePath, volID)
}

func (cd *csifDriver) getVolumeByID(volID string) (csifVolume, error) {
	if vol, ok := cd.volumes[volID]; ok {
		return vol, nil
	}
	return csifVolume{}, fmt.Errorf("no volID=%s in volumes", volID)
}

func (cd *csifDriver) getVolumeByName(volName string) (csifVolume, error) {
	for _, vol := range cd.volumes {
		if vol.Name == volName {
			return vol, nil
		}
	}
	return csifVolume{}, fmt.Errorf("no volName=%s in volumes", volName)
}

func (cd *csifDriver) createVolume(volID, name string, cap int64, accessType volAccessType) (*csifVolume, error) {
	glog.V(4).Infof("creating csif volume: %s", volID)
	path := getVolumePath(volID)

	switch accessType {
	case volAccessMount: // TODO: loop iscsi device
		err := os.MkdirAll(path, 0777)
		if err != nil {
			return nil, err
		}
	case volAccessBlock:
		panic("not implemented")
	default:
		return nil, fmt.Errorf("wrong access type %v", accessType)
	}

	vol := csifVolume{
		Name:       name,
		ID:         volID,
		Path:       path,
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
	case volAccessMount: // TODO: loop iscsi device
		path := getVolumePath(volID)
		if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	case volAccessBlock:
		panic("not implemented")
	}

	delete(cd.volumes, volID)
	return nil
}
