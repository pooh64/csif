package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/golang/glog"
	"github.com/pooh64/csif-driver/pkg/csif"
	utilexec "k8s.io/utils/exec"
)

var (
	endpoint   = flag.String("endpoint", "", "endpoint")
	tgtport    = flag.Uint("tgtport", 0, "tgtd iscsi port")
	tgtcontrol = flag.Uint("tgtcontrol", 0, "tgtd control port")
)

func init() {
	flag.Set("logtostderr", "true")
}

func logDevDir() {
	cmd := utilexec.New().Command("ls", "/dev")
	out, _ := cmd.CombinedOutput()
	str := string(out)
	str = strings.Replace(str, "\n", " ", -1)
	glog.V(4).Infof("/dev dir: %v", str)
}

func main() {
	flag.Parse()

	portal, err := csif.GetPortal()
	if err != nil {
		fmt.Printf("Failed to obtain portal: %v", err)
		os.Exit(1)
	}

	tgtd, err := csif.NewCsifTGTD(uint32(*tgtcontrol),
		csif.CsifFilterIQNPrefix, portal, uint32(*tgtport))
	if err != nil {
		fmt.Printf("Can't create new tgtd: %v", err.Error())
		os.Exit(1)
	}

	logDevDir()

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
