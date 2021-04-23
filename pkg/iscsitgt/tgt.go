package iscsitgt

import (
	"fmt"

	utilexec "k8s.io/utils/exec"
)

type iscsiTarget struct {
	id      uint32
	name    string
	freeLun uint32 // dummy, TODO:
}

func newISCSITarget() *iscsiTarget {
	return &iscsiTarget{}
}

func (it *iscsiTarget) Start(id uint32, name string) error {
	exec := utilexec.New()
	//--lld <driver> --op new --mode target --tid <id> --targetname <name>
	out, err := exec.Command("tgtadm", "--lld", "iscsi",
		"--op", "new", "--mode", "target",
		"--tid", fmt.Sprint(id), "--targetname", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v", string(out))
	}
	it.id = id
	it.name = name
	it.freeLun = 1
	return nil
}

func (it *iscsiTarget) CreateLun(bstore string) (uint32, error) {
	exec := utilexec.New()
	//--lld <driver> --op new --mode logicalunit --tid <id> --lun <lun> --backing-store <path>
	out, err := exec.Command("tgtadm", "--lld", "iscsi",
		"--op", "new", "--mode", "logicalunit",
		"--tid", fmt.Sprint(it.id), "--lun", fmt.Sprint(it.freeLun),
		"--backing-store", bstore).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("%v", string(out))
	}
	it.freeLun++
	return it.freeLun - 1, nil
}

func (it *iscsiTarget) BindAddr(addr string) error {
	exec := utilexec.New()
	//--lld <driver> --op bind --mode target --tid <id> --initiator-address <address>
	out, err := exec.Command("tgtadm", "--lld", "iscsi",
		"--op", "bind", "--mode", "target",
		"--tid", fmt.Sprint(it.id), "--initiator-address", addr).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v", string(out))
	}
	return nil
}

//tgtadm --lld iscsi --op new --mode portal --param portal=10.1.1.101:3260
