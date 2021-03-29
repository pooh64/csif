package csif

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type NbServer interface {
	Start(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer)
	Stop()
	ForceStop()
	Wait()
}

type nbServer struct {
	server *grpc.Server
	wg     sync.WaitGroup
}

func NewNbServer() *nbServer {
	return &nbServer{}
}

func (s *nbServer) Wait() {
	s.wg.Wait()
}

func (s *nbServer) Stop() {
	s.server.GracefulStop()
}

func (s *nbServer) ForceStop() {
	s.server.Stop()
}

func (s *nbServer) Start(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {
	s.wg.Add(1)
	go s.serve(endpoint, ids, cs, ns)
}

func parseSockEndpoint(ep string) (string, string, error) {
	if strings.HasPrefix(strings.ToLower(ep), "unix://") || strings.HasPrefix(strings.ToLower(ep), "tcp://") {
		splitted := strings.SplitN(ep, "://", 2)
		if splitted[1] != "" {
			return splitted[0], splitted[1], nil
		}
	}
	return "", "", fmt.Errorf("parseEndpoint: invalid: %v", ep)
}

func logInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
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

func (s *nbServer) serve(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {
	network, addr, err := parseSockEndpoint(endpoint)
	if err != nil {
		glog.Fatal(err.Error())
	}

	if network == "unix" {
		addr = "/" + addr
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			glog.Fatalf("Remove(%s) unix sock failed: %s", addr, err.Error())
		}
	}

	listener, err := net.Listen(network, addr)
	if err != nil {
		glog.Fatalf("Listen failed: %v", err)
	}

	server_opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(logInterceptor),
	}
	s.server = grpc.NewServer(server_opts...)

	if ids != nil {
		csi.RegisterIdentityServer(s.server, ids)
	}
	if cs != nil {
		csi.RegisterControllerServer(s.server, cs)
	}
	if ns != nil {
		csi.RegisterNodeServer(s.server, ns)
	}

	glog.Infof("Listen, Serve: %#v", listener.Addr())
	s.server.Serve(listener)
}
