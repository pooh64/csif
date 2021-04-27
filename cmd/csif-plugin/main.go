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
	filterAddr        = flag.String("filteraddr", "", "filter server addr")
	tgtport           = flag.Uint("tgtport", 0, "tgtd iscsi port")
	tgtcontrol        = flag.Uint("tgtcontrol", 0, "tgtd control port")
)

func init() {
	flag.Set("logtostderr", "true")
}

func main() {
	flag.Parse()

	portal, err := csif.GetPortal()
	if err != nil {
		fmt.Printf("Failed to obtain portal: %v", err)
		os.Exit(1)
	}

	tgtd, err := csif.NewCsifTGTD(uint32(*tgtcontrol),
		csif.CsifFServerIQNPrefix, portal, uint32(*tgtport))
	if err != nil {
		fmt.Printf("Can't create new tgtd: %v", err.Error())
		os.Exit(1)
	}

	driver, err := csif.NewCsifDriver(*driverName, *nodeID, *endpoint, version,
		*maxVolumesPerNode, *filterAddr, tgtd)
	if err != nil {
		fmt.Printf("Can't create new driver: %s", err.Error())
		os.Exit(1)
	}

	if err := driver.Run(); err != nil {
		fmt.Printf("Failed to run: %s", err.Error())
		os.Exit(1)
	}
}
