package csif

import (
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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

	clientset *kubernetes.Clientset
	ns        *csifNodeServer
}

func NewCsifDriver(name, nodeID, endpoint, version string, maxVolumesPerNode int64) (*csifDriver, error) {
	if name == "" || endpoint == "" || nodeID == "" {
		return nil, fmt.Errorf("wrong args")
	}
	if version == "" {
		version = "notset"
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("load k8s config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create k8s clientset: %v", err)
	}

	cd := &csifDriver{
		name:              name,
		version:           version,
		endpoint:          endpoint,
		nodeID:            nodeID,
		maxVolumesPerNode: maxVolumesPerNode,
		clientset:         clientset,
	}

	glog.Infof("New Driver: name=%v version=%v", name, version)

	return cd, nil
}

func driverLogInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if info.FullMethod == "/csi.v1.Identity/Probe" {
		return handler(ctx, req)
	}
	glog.V(3).Infof("call: %s", info.FullMethod)
	glog.V(4).Infof("request: %+v", protosanitizer.StripSecrets(req))

	resp, err := handler(ctx, req)
	if err != nil {
		glog.Errorf("error: %v", err)
	} else {
		glog.V(4).Infof("response: %+v", protosanitizer.StripSecrets(resp))
	}
	return resp, err
}

func (cd *csifDriver) Run() error {
	cd.ns = newCsifNodeServer(cd)
	cs := newCsifControllerServer(cd)

	register := func(s *grpc.Server) {
		csi.RegisterIdentityServer(s, cd)
		if cs != nil {
			csi.RegisterControllerServer(s, cs)
		}
		if cd.ns != nil {
			csi.RegisterNodeServer(s, cd.ns)
		}
	}

	server := NewNbServer()
	server.Start(cd.endpoint, register, driverLogInterceptor)
	server.Wait()
	return nil
}
