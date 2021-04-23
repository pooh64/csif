package csif

import (
	"fmt"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	lib_iscsi "github.com/pooh64/csi-lib-iscsi/iscsi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	csifDiskTypeISCSI = "hostimg"
)

type csifDiskISCSI struct {
	TargetPortal string `json:"targetPortal"`
	Iqn          string `json:"iqn"`
	Lun          int32  `json:"lun,string"`

	name string `json:"-"`
	conn *lib_iscsi.Connector
}

func RegisterDiskTypeISCSI() (string, csifDiskNewFn, error) {
	return csifDiskTypeISCSI, newCsifDiskISCSI, nil
}

func newCsifDiskISCSI() csifDisk {
	return &csifDiskISCSI{}
}

func (d *csifDiskISCSI) GetType() string {
	return csifDiskTypeISCSI
}

func (d *csifDiskISCSI) Create(req *csi.CreateVolumeRequest, volID string) error {
	/* no dynamic provisioning */
	return status.Errorf(codes.Unimplemented, "")
}

func (d *csifDiskISCSI) Destroy() error {
	/* no dynamic provisioning */
	return status.Errorf(codes.Unimplemented, "")
}

func iscsiSetDefaultPort(portal string) string {
	if !strings.Contains(portal, ":") {
		portal = portal + ":3260"
	}
	return portal
}

func (d *csifDiskISCSI) Attach(req *csi.NodeStageVolumeRequest) (string, error) {
	d.TargetPortal = iscsiSetDefaultPort(d.TargetPortal)
	d.name = req.GetVolumeId()

	target := lib_iscsi.TargetInfo{Iqn: d.Iqn, Portal: d.TargetPortal, Port: ""}
	d.conn = &lib_iscsi.Connector{
		VolumeName: d.name,
		Targets:    []lib_iscsi.TargetInfo{target},
		Multipath:  false,
	}

	dev, err := lib_iscsi.Connect(*d.conn)
	if err != nil {
		return "", fmt.Errorf("iscsi connect failed: %v", err)
	}
	return dev, nil
	// TODO:
	//file := path.Join(req.GetTargetPath(), d.name+".json")
	//err = iscsi_lib.PersistConnector(d.conn, file)
}

func (d *csifDiskISCSI) Detach() error {
	if err := lib_iscsi.Disconnect(d.Iqn, []string{d.TargetPortal}); err != nil {
		return fmt.Errorf("iscsi disconnect failed: %v", err)
	}
	return nil
}

func (d *csifDiskISCSI) GetPath() (string, error) {
	if d.conn.DevicePath == "" {
		return "", fmt.Errorf("device not mounted")
	}
	return d.conn.DevicePath, nil
}
