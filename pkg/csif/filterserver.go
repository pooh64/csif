package csif

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/pooh64/csif-driver/pkg/filter"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	CsifFilterIQNPrefix = "iqn.com.pooh64.csi.csif.filter"
	FakeBstorePath      = "/csif-fake-bstore.img"
)

type csifFilterServer struct {
	endpoint string
	tgtd     *csifTGTD
	target   *iscsiTarget

	filter.UnimplementedFilterServer
}

func getTargetInfoStr(t *filter.TargetInfo) string {
	return t.GetPortal() + "-" + fmt.Sprint(t.GetPort()) + "-" + t.GetIqn()
}

func NewCsifFilterServer(endpoint string, tgtd *csifTGTD) (*csifFilterServer, error) {
	return &csifFilterServer{
		endpoint: endpoint,
		tgtd:     tgtd,
		target:   nil,
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

func (cf *csifFilterServer) CreateTarget(ctx context.Context, req *filter.CreateTargetRequest) (*filter.CreateTargetResponse, error) {
	if cf.target != nil {
		return nil, status.Errorf(codes.AlreadyExists, "target already exists")
	}

	// fake bstore
	if err := createImg(FakeBstorePath, 16*mib); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create fake bstore: %v", err)
	}

	out, err := cf.tgtd.CreateDisk(FakeBstorePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create target: %v", err)
	}
	cf.target = out

	return &filter.CreateTargetResponse{
		Target: &filter.TargetInfo{
			Portal: cf.tgtd.portal,
			Port:   cf.tgtd.port,
			Iqn:    cf.target.iqn,
		},
	}, nil
}

func (cf *csifFilterServer) DeleteTarget(ctx context.Context, req *filter.DeleteTargetRequest) (*filter.DeleteTargetResponse, error) {
	if cf.target == nil {
		return nil, status.Errorf(codes.NotFound, "target doesn't exist")
	}

	if err := cf.tgtd.DeleteDisk(cf.target.id); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete tgtd target: %v", err)
	}
	cf.target = nil

	if err := destroyImg(FakeBstorePath); err != nil {
		glog.Errorf("failed to delete fake bstore: %v", err)
	}

	return &filter.DeleteTargetResponse{}, nil
}
