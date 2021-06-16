package csif

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pooh64/csif-driver/pkg/filter"

	"golang.org/x/net/context"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	lib_iscsi "github.com/pooh64/csi-lib-iscsi/iscsi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Set by driver, not filter
// TODO: What to do?
const (
	CsifFilterPortGRPC       = 9820
	CsifFilterPortTGT        = 9821
	CsifFilterPortTGTControl = 9822
	CsifFilterBstoreSrc      = "/dev/csi-csif-bstore-src"
	СsifFilterForcedLoop0    = "/dev/loop0" // TODO: fix
)

// TODO: idempotent CS
// VerifyParam(req *csi.CreateVolumeRequest) error
type csifDisk struct {
	SourcePVC string      `json:"sourcePVC"`
	cd        *csifDriver `json:"-"`

	filterPod    *core.Pod            `json:"-"`
	filterConn   *grpc.ClientConn     `json:"-"`
	targetExists bool                 `json:"-"`
	targetConn   *lib_iscsi.Connector `json:"-"`
	dev          string               `json:"-"`
}

func newCsifDisk(driver *csifDriver) *csifDisk {
	return &csifDisk{
		cd: driver,
	}
}

// CS routine: process sclass volumeAttributes: check, provision
func (d *csifDisk) Create(req *csi.CreateVolumeRequest, volID string) error {
	/* no dynamic provisioning */
	return status.Errorf(codes.Unimplemented, "")
}

// CS routine: delete created disk
func (d *csifDisk) Destroy() error {
	/* no dynamic provisioning */
	return status.Errorf(codes.Unimplemented, "")
}

func waitPodCond(coreif v1.CoreV1Interface, pod *core.Pod, pred func(*core.Pod) bool, timeout time.Duration) (*core.Pod, error) {
	watcher, err := coreif.Pods(pod.Namespace).Watch(context.TODO(), metav1.ListOptions{
		Watch:           true,
		ResourceVersion: pod.ResourceVersion,
		FieldSelector:   fields.Set{"metadata.name": pod.Name}.AsSelector().String(),
		LabelSelector:   labels.Everything().String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %v", err)
	}

	func() {
		for {
			select {
			case event, ok := <-watcher.ResultChan():
				if !ok {
					return
				}
				podupd, ok := event.Object.(*core.Pod)
				if ok && pred(podupd) {
					watcher.Stop()
				}
			case <-time.After(timeout):
				watcher.Stop()
			}
		}
	}()
	return coreif.Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
}

func (d *csifDisk) createFilterPod() error {
	var err error
	pod := makeFilterPodConf(d)

	coreif := d.cd.clientset.CoreV1()

	pod, err = coreif.Pods(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create pod: %v", err)
	}
	d.filterPod = pod

	pred := func(pod *core.Pod) bool { return pod.Status.Phase != core.PodPending }

	pod, err = waitPodCond(coreif, pod, pred, 100*time.Second)
	if err != nil {
		if err := d.deleteFilterPod(); err != nil {
			glog.Errorf("failed to delete pod: %v", err)
		}
		return err
	}
	if pod.Status.Phase != core.PodRunning {
		if err := d.deleteFilterPod(); err != nil {
			glog.Errorf("failed to delete pod: %v", err)
		}
		return fmt.Errorf("pod failed to start within timeout: %v", pod.Status.Phase)
	}

	d.filterPod = pod
	return nil
}

func (d *csifDisk) deleteFilterPod() error {
	coreif := d.cd.clientset.CoreV1()
	err := coreif.Pods(d.filterPod.Namespace).Delete(context.TODO(), d.filterPod.Name, metav1.DeleteOptions{})
	if err != nil {
		d.filterPod = nil
	}

	// TODO: print log, in create... too
	// TODO: wait deletion
	return err
}

// NS routine: attach disk as block device
func (d *csifDisk) Connect() error {
	var err error

	if err := d.createFilterPod(); err != nil {
		return fmt.Errorf("create filter pod failed: %v", err)
	}

	opts := []grpc.DialOption{
		grpc.WithInsecure(),
	}
	d.filterConn, err = grpc.Dial(d.filterPod.Status.PodIP+":"+fmt.Sprint(CsifFilterPortGRPC), opts...)
	if err != nil {
		d.Disconnect()
		return fmt.Errorf("failed to connect to filter gRPC: %v", err)
	}

	client := filter.NewFilterClient(d.filterConn)
	resp, err := client.CreateTarget(context.Background(), &filter.CreateTargetRequest{})
	if err != nil {
		d.Disconnect() // BUG: deleteFilterPod was skipped here somehow
		return fmt.Errorf("failed to create filter target: %v", err)
	}
	d.targetExists = true

	resptgt := resp.GetTarget()
	d.targetConn = &lib_iscsi.Connector{
		VolumeName: getTargetInfoStr(resptgt),
		Targets: []lib_iscsi.TargetInfo{{
			Iqn:    resptgt.GetIqn(),
			Portal: resptgt.GetPortal(),
			Port:   fmt.Sprint(resptgt.GetPort())}},
		Lun:         csifTGTDdefaultLUN,
		Multipath:   false,
		DoDiscovery: true,
	}
	d.dev, err = lib_iscsi.Connect(*d.targetConn)
	if err != nil {
		d.targetConn = nil
		d.dev = ""
		d.Disconnect()
		return status.Errorf(codes.Internal, "iscsi connect failed: %v", err)
	}

	// TODO:
	//file := path.Join(req.GetTargetPath(), d.name+".json")
	//err = iscsi_lib.PersistConnector(d.conn, file)
	return nil
}

func (d *csifDisk) Disconnect() error {
	if d.targetConn != nil {
		t := &d.targetConn.Targets[0]
		err := lib_iscsi.Disconnect(t.Iqn, []string{t.Portal + ":" + t.Port})
		if err != nil {
			return fmt.Errorf("failed to disconnect server target: %v", err)
		}
		d.targetConn = nil
	}

	if d.targetExists {
		client := filter.NewFilterClient(d.filterConn)
		if _, err := client.DeleteTarget(context.Background(), &filter.DeleteTargetRequest{}); err != nil {
			return fmt.Errorf("failed to delete filter target: %v", err)
		}
		d.targetExists = false
	}

	if d.filterConn != nil {
		if err := d.filterConn.Close(); err != nil {
			return fmt.Errorf("failed to close filter conn: %v", err)
		}
		d.filterConn = nil
	}

	if d.filterPod != nil {
		if err := d.deleteFilterPod(); err != nil {
			return fmt.Errorf("failed to delete filter pod: %v", err)
		}
		d.filterPod = nil
	}
	return nil
}

func (d *csifDisk) GetDevPath() string {
	if d.dev == "" {
		panic("empty disk dev path")
	}
	return d.dev
}

// save to VolumeContext
func (disk *csifDisk) SaveContext() map[string]string {
	jbyt, err := json.Marshal(interface{}(disk))
	if err != nil {
		panic(err)
	}
	attr := make(map[string]string)
	if err := json.Unmarshal([]byte(jbyt), &attr); err != nil {
		panic(err)
	}
	return attr
}

// load from VolumeContext
func (disk *csifDisk) LoadContext(attr map[string]string) error {
	jbyt, _ := json.Marshal(attr)
	if err := json.Unmarshal([]byte(jbyt), disk); err != nil {
		glog.Errorf("failed to unmarshal disk data")
		return err
	}
	return nil
}

func makeFilterPodConf(d *csifDisk) *core.Pod {
	priv := true
	return &core.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "csi-csif-fs-" + d.SourcePVC,
			Namespace: "default",
			//Labels: map[string]string{
			//	"appxxx": "xxx",
			//},
		},
		Spec: core.PodSpec{
			Volumes: []core.Volume{
				{
					Name: "csi-csif-vol-src",
					VolumeSource: core.VolumeSource{
						PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
							ClaimName: d.SourcePVC,
							ReadOnly:  false,
						},
					},
				},
			},
			//DNSPolicy:   "ClusterFirstWithHostNet",
			HostNetwork: false,
			//Hostname:    "",
			//NodeName: d.cd.nodeID,

			Containers: []core.Container{
				{
					Name:            "filter",
					Image:           "pooh64/csif-filter:latest",
					ImagePullPolicy: core.PullAlways,
					Args: []string{
						"--endpoint=tcp://:" + fmt.Sprint(CsifFilterPortGRPC),
						"--tgtport=" + fmt.Sprint(CsifFilterPortTGT),
						"--tgtcontrol=" + fmt.Sprint(CsifFilterPortTGTControl),
						"--v=5",
					},
					VolumeDevices: []core.VolumeDevice{
						{
							Name:       "csi-csif-vol-src",
							DevicePath: CsifFilterBstoreSrc,
						},
					},
					SecurityContext: &core.SecurityContext{
						Privileged: &priv,
						Capabilities: &core.Capabilities{
							Add: []core.Capability{
								"SYS_ADMIN",
							},
						},
					},
				},
			},
		},
	}
}
