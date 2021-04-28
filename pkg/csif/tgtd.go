package csif

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	utilexec "k8s.io/utils/exec"
)

const (
	csifTGTDmaxTargets = 128
	csifTGTDdefaultLUN = 1
)

type csifTGTD struct {
	control uint32
	iqnPref string
	targets map[int]*iscsiTarget

	portal string
	port   uint32
}

type iscsiTarget struct {
	control uint32
	id      int
	iqn     string
}

func startTGTD(control uint32, portal string) error {
	exec := utilexec.New()
	out, err := exec.Command("tgtd", "-C", fmt.Sprint(control),
		"--iscsi", "portal="+portal).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v", string(out))
	}
	return nil
}

func NewCsifTGTD(control uint32, iqnPref, portal string, port uint32) (*csifTGTD, error) {
	fullportal := "*:" + fmt.Sprint(port)

	if err := startTGTD(control, fullportal); err != nil {
		return nil, fmt.Errorf("failed to start tgtd: %v", err)
	}
	glog.V(4).Infof("tgtd: control=%v, portal=%v", control, fullportal)

	return &csifTGTD{
		control: control,
		iqnPref: iqnPref,
		targets: map[int]*iscsiTarget{0: {}},
		portal:  portal,
		port:    port,
	}, nil
}

func (d *csifTGTD) CreateDisk(bstore string) (*iscsiTarget, error) {
	tid, err := d.allocTID()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate tid: %v", err)
	}

	target := &iscsiTarget{
		control: d.control,
		id:      tid,
		iqn:     d.iqnPref + ":" + fmt.Sprint(tid),
	}

	if err = target.start(); err != nil {
		return nil, fmt.Errorf("failed to start target: %v", err)
	}
	defer cleanup(err, func() { target.stop() })

	if err = target.createLun(csifTGTDdefaultLUN, bstore); err != nil {
		return nil, fmt.Errorf("failed to create lun: %v", err)
	}
	defer cleanup(err, func() { target.deleteLun(csifTGTDdefaultLUN) })

	if err = target.bindAddr("ALL"); err != nil {
		return nil, fmt.Errorf("failed to bind addr: %v", err)
	}
	d.targets[tid] = target
	return target, nil
}

func (d *csifTGTD) DeleteDisk(tid int) error {
	target, ok := d.targets[tid]
	if !ok {
		return fmt.Errorf("tid doesn't exitst")
	}

	if err := target.deleteLun(csifTGTDdefaultLUN); err != nil {
		if err := target.stop(); err != nil {
			glog.Errorf("failed to stop target: %v", err)
		} else {
			delete(d.targets, tid)
			return nil
		}
		return fmt.Errorf("failed to delete lun: %v", err)
	}

	if err := target.stop(); err != nil {
		return fmt.Errorf("failed to stop target: %v", err)
	}
	delete(d.targets, tid)
	return nil
}

// TODO: rewrite
func (d *csifTGTD) allocTID() (int, error) {
	for id := 1; id < csifTGTDmaxTargets; id++ {
		if _, ok := d.targets[id]; !ok {
			return id, nil
		}
	}
	return 0, fmt.Errorf("cisfTGTD limit reached")
}

func (it *iscsiTarget) execCmd(args ...string) error {
	cmd := utilexec.New().Command("tgtadm", args...)
	out, err := cmd.CombinedOutput()
	glog.V(4).Infof("exec: %v\nout: %v", strings.Join(args, " "), string(out))
	if err != nil {
		return fmt.Errorf("%v", string(out))
	}
	return nil
}

func (it *iscsiTarget) start() error {
	//--lld <driver> --op new --mode target --tid <id> --targetname <name>
	args := []string{"-C", fmt.Sprint(it.control), "--lld", "iscsi",
		"--op", "new", "--mode", "target",
		"--tid", fmt.Sprint(it.id), "--targetname", it.iqn}
	return it.execCmd(args...)
}

func (it *iscsiTarget) stop() error {
	//--lld <driver> --op delete --mode target --tid <id>
	args := []string{"-C", fmt.Sprint(it.control), "--lld", "iscsi",
		"--op", "delete", "--mode", "target",
		"--tid", fmt.Sprint(it.id)}
	return it.execCmd(args...)
}

func (it *iscsiTarget) createLun(lun uint32, bstore string) error {
	//--lld <driver> --op new --mode logicalunit --tid <id> --lun <lun> --backing-store <path>
	args := []string{"-C", fmt.Sprint(it.control), "--lld", "iscsi",
		"--op", "new", "--mode", "logicalunit",
		"--tid", fmt.Sprint(it.id), "--lun", fmt.Sprint(lun),
		"--backing-store", bstore}
	return it.execCmd(args...)
}

func (it *iscsiTarget) deleteLun(lun uint32) error {
	//--lld <driver> --op delete --mode logicalunit --tid <id> --lun <lun>
	args := []string{"-C", fmt.Sprint(it.control), "--lld", "iscsi",
		"--op", "delete", "--mode", "logicalunit",
		"--tid", fmt.Sprint(it.id), "--lun", fmt.Sprint(lun)}
	return it.execCmd(args...)
}

func (it *iscsiTarget) bindAddr(addr string) error {
	//--lld <driver> --op bind --mode target --tid <id> --initiator-address <address>
	args := []string{"-C", fmt.Sprint(it.control), "--lld", "iscsi",
		"--op", "bind", "--mode", "target",
		"--tid", fmt.Sprint(it.id), "--initiator-address", addr}
	return it.execCmd(args...)
}
