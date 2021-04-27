package csif

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/golang/glog"
	"google.golang.org/grpc"
)

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

func (s *nbServer) Start(endpoint string, prep func(*grpc.Server), li grpc.UnaryServerInterceptor) {
	s.wg.Add(1)
	go s.serve(endpoint, prep, li)
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

func (s *nbServer) serve(endpoint string, prep func(*grpc.Server), li grpc.UnaryServerInterceptor) {
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
		grpc.UnaryInterceptor(li),
	}
	s.server = grpc.NewServer(server_opts...)

	if prep != nil {
		prep(s.server)
	}

	glog.Infof("Listen, Serve: %#v", listener.Addr())
	s.server.Serve(listener)
}
