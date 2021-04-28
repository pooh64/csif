package csif

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	lib_iscsi "github.com/pooh64/csi-lib-iscsi/iscsi"
	"github.com/pooh64/csif-driver/pkg/filter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	CsifFServerIQNPrefix = "iqn.com.pooh64.csi.csif.filter"
	CsifFClientIQNPrefix = "iqn.com.pooh64.csi.csif.client"
)

type csifFilterEntry struct {
	conn *lib_iscsi.Connector
	dev  string
	out  *iscsiTarget
}

type csifFilterServer struct {
	endpoint string
	tgtd     *csifTGTD
	disks    map[string]*csifFilterEntry

	filter.UnimplementedFilterServer
}

func NewCsifFilterServer(endpoint string, tgtd *csifTGTD) (*csifFilterServer, error) {
	return &csifFilterServer{
		endpoint: endpoint,
		tgtd:     tgtd,
		disks:    map[string]*csifFilterEntry{},
	}, nil
}

func csifFilterLogInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	glog.V(3).Infof("call: %s", info.FullMethod)
	glog.V(4).Infof("request: %+v", req)

	resp, err := handler(ctx, req)
	if err != nil {
		glog.Errorf("error: %v", err)
	} else {
		glog.V(4).Infof("response: %+v", resp)
	}
	return resp, err
}

func (cf *csifFilterServer) Run() error {
	server := NewNbServer()
	register := func(s *grpc.Server) {
		filter.RegisterFilterServer(s, cf)
	}
	server.Start(cf.endpoint, register, csifFilterLogInterceptor)
	server.Wait()
	return nil
}

func filterGetIdFromSrc(src *filter.FilterDeviceInfo) string {
	return src.GetPortal() + "-" + fmt.Sprint(src.GetPort()) + "-" + src.GetIqn()
}

func (cf *csifFilterServer) CreateFilter(ctx context.Context, req *filter.CreateFilterRequest) (*filter.CreateFilterResponse, error) {
	src := req.GetClientDev()
	id := filterGetIdFromSrc(src)

	if _, ok := cf.disks[id]; ok {
		return nil, status.Errorf(codes.AlreadyExists, "device already filtered")
	}

	conn := &lib_iscsi.Connector{
		VolumeName: id,
		Targets: []lib_iscsi.TargetInfo{{
			Iqn:    src.GetIqn(),
			Portal: src.GetPortal(),
			Port:   fmt.Sprint(src.GetPort())}},
		Lun:         csifTGTDdefaultLUN,
		Multipath:   false,
		DoDiscovery: true,
	}

	dev, err := lib_iscsi.Connect(*conn)
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
			Portal: cf.tgtd.portal,
			Port:   cf.tgtd.port,
			Iqn:    out.iqn},
	}, nil
}

func (cf *csifFilterServer) DeleteFilter(ctx context.Context, req *filter.DeleteFilterRequest) (*filter.DeleteFilterResponse, error) {
	src := req.GetClientDev()
	id := filterGetIdFromSrc(src)

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
