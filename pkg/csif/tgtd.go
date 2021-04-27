package csif

import (
	"fmt"

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

func startTGTD(control uint32, port uint32) error {
	exec := utilexec.New()
	out, err := exec.Command("tgtd", "-C", fmt.Sprint(control),
		"--iscsi", "portal=:"+fmt.Sprint(port)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v", string(out))
	}
	return nil
}

func NewCsifTGTD(control uint32, iqnPref, portal string, port uint32) (*csifTGTD, error) {
	if err := startTGTD(control, port); err != nil {
		return nil, fmt.Errorf("failed to start tgtd: %v", err)
	}

	return &csifTGTD{
		control: control,
		iqnPref: iqnPref,
		targets: map[int]*iscsiTarget{},
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
	defer cleanup(&err, func() { target.stop() })

	if err = target.createLun(csifTGTDdefaultLUN, bstore); err != nil {
		return nil, fmt.Errorf("failed to create lun: %v", err)
	}
	defer cleanup(&err, func() { target.deleteLun(csifTGTDdefaultLUN) })

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
	for id := 0; id < csifTGTDmaxTargets; id++ {
		if _, ok := d.targets[id]; !ok {
			return id, nil
		}
	}
	return 0, fmt.Errorf("cisfTGTD limit reached")
}

func (it *iscsiTarget) start() error {
	exec := utilexec.New()
	//--lld <driver> --op new --mode target --tid <id> --targetname <name>
	out, err := exec.Command("tgtadm", "-C", fmt.Sprint(it.control), "--lld", "iscsi",
		"--op", "new", "--mode", "target",
		"--tid", fmt.Sprint(it.id), "--targetname", it.iqn).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v", string(out))
	}
	return nil
}

func (it *iscsiTarget) stop() error {
	exec := utilexec.New()
	//--lld <driver> --op delete --mode target --tid <id>
	out, err := exec.Command("tgtadm", "-C", fmt.Sprint(it.control), "--lld", "iscsi",
		"--op", "delete", "--mode", "target",
		"--tid", fmt.Sprint(it.id)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v", string(out))
	}
	return nil
}

func (it *iscsiTarget) createLun(lun uint32, bstore string) error {
	exec := utilexec.New()
	//--lld <driver> --op new --mode logicalunit --tid <id> --lun <lun> --backing-store <path>
	out, err := exec.Command("tgtadm", "-C", fmt.Sprint(it.control), "--lld", "iscsi",
		"--op", "new", "--mode", "logicalunit",
		"--tid", fmt.Sprint(it.id), "--lun", fmt.Sprint(lun),
		"--backing-store", bstore).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v", string(out))
	}
	return nil
}

func (it *iscsiTarget) deleteLun(lun uint32) error {
	exec := utilexec.New()
	//--lld <driver> --op delete --mode logicalunit --tid <id> --lun <lun>
	out, err := exec.Command("tgtadm", "-C", fmt.Sprint(it.control), "--lld", "iscsi",
		"--op", "delete", "--mode", "logicalunit",
		"--tid", fmt.Sprint(it.id), "--lun", fmt.Sprint(lun)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v", string(out))
	}
	return nil
}

func (it *iscsiTarget) bindAddr(addr string) error {
	exec := utilexec.New()
	//--lld <driver> --op bind --mode target --tid <id> --initiator-address <address>
	out, err := exec.Command("tgtadm", "-C", fmt.Sprint(it.control), "--lld", "iscsi",
		"--op", "bind", "--mode", "target",
		"--tid", fmt.Sprint(it.id), "--initiator-address", addr).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v", string(out))
	}
	return nil
}
