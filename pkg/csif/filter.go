package csif

import (
	"context"
	"fmt"

	lib_iscsi "github.com/pooh64/csi-lib-iscsi/iscsi"
	"github.com/pooh64/csif-driver/pkg/filter"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	csifFilterIQNPrefix = "iqn.com.pooh64.csi.csif.filter"
)

type csifFilterEntry struct {
	conn lib_iscsi.Connector
	dev  string
	out  *iscsiTarget
}

type csifFilterServer struct {
	portal string
	tgtd   *csifTGTD
	disks  map[string]*csifFilterEntry
}

func newCsifFilterServer(portal string, port uint32) *csifFilterServer {
	return &csifFilterServer{
		portal: portal,
		tgtd:   NewCsifTGTD(port, csifFilterIQNPrefix),
		disks:  map[string]*csifFilterEntry{},
	}
}

func (cf *csifFilterServer) getIdFromSrc(src *filter.FilterDeviceInfo) string {
	return src.GetTp() + "-" + fmt.Sprint(src.GetPort()) + "-" + src.GetIqn()
}

func (cf *csifFilterServer) CreateFilter(ctx context.Context, req *filter.CreateFilterRequest) (*filter.CreateFilterResponse, error) {
	src := req.GetClientDev()
	id := cf.getIdFromSrc(src)

	if _, ok := cf.disks[id]; ok {
		return nil, status.Errorf(codes.AlreadyExists, "device already filtered")
	}

	conn := lib_iscsi.Connector{
		VolumeName: id,
		Targets: []lib_iscsi.TargetInfo{{
			Iqn:    src.GetIqn(),
			Portal: src.GetTp(),
			Port:   fmt.Sprint(src.GetPort())}},
		Lun:         csifTGTDdefaultLUN,
		Multipath:   false,
		DoDiscovery: true,
	}

	dev, err := lib_iscsi.Connect(conn)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "iscsi connect failed: %v", err)
	}

	out, err := cf.tgtd.CreateDisk(dev)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create target: %v", err)
	}

	cf.disks[id] = &csifFilterEntry{
		conn: conn,
		dev:  dev,
		out:  out,
	}
	return &filter.CreateFilterResponse{
		ServerDev: &filter.FilterDeviceInfo{
			Tp:   cf.portal,
			Port: out.port,
			Iqn:  out.iqn},
	}, nil
}

func (cf *csifFilterServer) DeleteFilter(ctx context.Context, req *filter.DeleteFilterRequest) (*filter.DeleteFilterResponse, error) {
	src := req.GetClientDev()
	id := cf.getIdFromSrc(src)

	entry, ok := cf.disks[id]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "filter doesn't exist")
	}

	if entry.out != nil {
		if err := cf.tgtd.DeleteDisk(entry.out.id); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to delete tgtd target: %v", err)
		}
		entry.out = nil
	}

	portal := entry.conn.Targets[0].Portal + ":" + entry.conn.Targets[0].Port
	iqn := entry.conn.Targets[0].Iqn
	if err := lib_iscsi.Disconnect(iqn, []string{portal}); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to disconnect iscsi: %v", err)
	}

	return &filter.DeleteFilterResponse{}, nil
}
