package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pooh64/csif-driver/pkg/csif"
)

var (
	endpoint   = flag.String("endpoint", "", "endpoint")
	tgtport    = flag.Uint("tgtport", 0, "tgtd iscsi port")
	tgtcontrol = flag.Uint("tgtcontrol", 0, "tgtd control port")
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

	filter, err := csif.NewCsifFilterServer(*endpoint, tgtd)
	if err != nil {
		fmt.Printf("Can't create new filter: %v", err.Error())
		os.Exit(1)
	}

	if err := filter.Run(); err != nil {
		fmt.Printf("Failed to run: %v", err.Error())
		os.Exit(1)
	}
}
