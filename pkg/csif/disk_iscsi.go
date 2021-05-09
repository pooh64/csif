package csif

import (
	"fmt"
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	lib_iscsi "github.com/pooh64/csi-lib-iscsi/iscsi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	csifDiskTypeISCSI = "iscsi"
)

type csifDiskISCSI struct {
	TargetPortal string `json:"targetPortal"`
	Port         string `json:"port"`
	Iqn          string `json:"iqn"`
	Lun          int32  `json:"lun,string"`

	dev  string               `json:"-"`
	conn *lib_iscsi.Connector `json:"-"`
}

func RegisterDiskTypeISCSI() (string, csifDiskNewFn, error) {
	lib_iscsi.EnableDebugLogging(os.Stdout)
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

func (d *csifDiskISCSI) Attach() (string, error) {
	//d.TargetPortal = iscsiSetDefaultPort(d.TargetPortal)

	target := lib_iscsi.TargetInfo{Iqn: d.Iqn, Portal: d.TargetPortal, Port: d.Port}
	d.conn = &lib_iscsi.Connector{
		VolumeName:  d.Iqn, // unique, probably
		Targets:     []lib_iscsi.TargetInfo{target},
		Lun:         d.Lun,
		Multipath:   false,
		DoDiscovery: true,
	}

	dev, err := lib_iscsi.Connect(*d.conn)
	if err != nil {
		return "", fmt.Errorf("iscsi connect failed: %v", err)
	}
	d.dev = dev
	return dev, nil
	// TODO:
	//file := path.Join(req.GetTargetPath(), d.name+".json")
	//err = iscsi_lib.PersistConnector(d.conn, file)
}

func (d *csifDiskISCSI) Detach() error {
	portal := d.TargetPortal + ":" + d.Port
	if err := lib_iscsi.Disconnect(d.Iqn, []string{portal}); err != nil {
		return fmt.Errorf("iscsi disconnect failed: %v", err)
	}
	return nil
}

func (d *csifDiskISCSI) GetPath() (string, error) {
	if d.dev == "" {
		return "", fmt.Errorf("device not mounted")
	}
	return d.dev, nil
}
