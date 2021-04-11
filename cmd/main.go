package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pooh64/csif-driver/pkg/csif"
)

var (
	version           = "0.0.1"
	endpoint          = flag.String("endpoint", "unix://tmp/csi.sock", "endpoint")
	nodeID            = flag.String("nodeid", "", "node id")
	driverName        = flag.String("drivername", "csif.csi.pooh64.io", "driver name")
	maxVolumesPerNode = flag.Int64("maxvolumespernode", 0, "limit of volumes per node")
)

func init() {
	flag.Set("logtostderr", "true")
}

func main() {
	flag.Parse()

	driver, err := csif.NewCsifDriver(*driverName, *nodeID, *endpoint, version, *maxVolumesPerNode)
	if err != nil {
		fmt.Printf("Can't create new driver: %s", err.Error())
		os.Exit(1)
	}

	if err := driver.Run(); err != nil {
		fmt.Printf("Failed to run: %s", err.Error())
		os.Exit(1)
	}
}
