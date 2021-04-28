package csif

import (
	"fmt"
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	lib_iscsi "github.com/pooh64/csi-lib-iscsi/iscsi"
	"github.com/pooh64/csif-driver/pkg/filter"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
	"k8s.io/utils/exec"
)

type csifNodeServer struct {
	cd      *csifDriver
	mounter mount.SafeFormatAndMount
	volumes map[string]*csifVolumeAttachment
}

func newCsifNodeServer(driver *csifDriver) *csifNodeServer {
	mounter := mount.SafeFormatAndMount{
		Interface: mount.New(""),
		Exec:      exec.New(),
	}

	return &csifNodeServer{
		cd:      driver,
		mounter: mounter,
		volumes: map[string]*csifVolumeAttachment{},
	}
}

type csifVolumeAttachment struct {
	Disk   csifDisk
	target *iscsiTarget
	conn   *lib_iscsi.Connector
	outDev string

	StagingPath         string
	remoteFilterDeleted bool
}

func (vol *csifVolumeAttachment) getPath() string {
	return vol.outDev
}

func (ns *csifNodeServer) getVolumeAttachment(volID string) (*csifVolumeAttachment, error) {
	if vol, ok := ns.volumes[volID]; ok {
		return vol, nil
	}
	return nil, fmt.Errorf("no volID=%s in volumes", volID)
}

func (ns *csifNodeServer) createVolumeAttachment(req *csi.NodeStageVolumeRequest) (*csifVolumeAttachment, error) {
	disk, err := ns.cd.csifDiskAttach(req)
	if err != nil {
		return nil, fmt.Errorf("failed to attach disk: %v", err)
	}

	vol := &csifVolumeAttachment{
		Disk: disk,
	}
	if err := ns.createFilter(vol); err != nil {
		if err := vol.Disk.Detach(); err != nil {
			glog.Errorf("failed to detach disk: %v", err)
		}
		return nil, fmt.Errorf("failed to create filter: %v", err)
	}

	ns.volumes[req.VolumeId] = vol
	return vol, nil
}

func (ns *csifNodeServer) deleteVolumeAttachment(volumeID string) error {
	vol := ns.volumes[volumeID]

	if err := ns.deleteFilter(vol); err != nil { // force
		glog.Errorf("failed to delete filter: %v", err)
	}

	if err := vol.Disk.Detach(); err != nil { // force
		glog.Errorf("failed to detach disk while detaching pvc")
	}
	delete(ns.volumes, volumeID)
	return nil
}

func (ns *csifNodeServer) createFilter(att *csifVolumeAttachment) error {
	var errout error = nil
	src, err := att.Disk.GetPath()
	if err != nil {
		return fmt.Errorf("failed to get disk path: %v", err)
	}
	target, err := ns.cd.tgtd.CreateDisk(src)
	if err != nil {
		return fmt.Errorf("failed to create tgtd disk: %v", err)
	}
	glog.V(4).Infof("tgtd disk created")
	defer cleanup(&errout, func() { ns.cd.tgtd.DeleteDisk(target.id) })

	client := filter.NewFilterClient(ns.cd.filterConn)
	req := &filter.CreateFilterRequest{
		ClientDev: &filter.FilterDeviceInfo{
			Portal: ns.cd.tgtd.portal,
			Port:   ns.cd.tgtd.port,
			Iqn:    target.iqn,
		},
	}
	resp, err := client.CreateFilter(context.Background(), req)
	if errout = err; err != nil {
		return fmt.Errorf("failed to create filter: %v", err)
	}
	defer cleanup(&errout, func() {
		client.DeleteFilter(context.Background(),
			&filter.DeleteFilterRequest{ClientDev: req.ClientDev})
	})

	sdev := resp.GetServerDev()
	conn := &lib_iscsi.Connector{
		VolumeName: filterGetIdFromSrc(sdev),
		Targets: []lib_iscsi.TargetInfo{{
			Iqn:    sdev.GetIqn(),
			Portal: sdev.GetPortal(),
			Port:   fmt.Sprint(sdev.GetPort())}},
		Lun:         csifTGTDdefaultLUN,
		Multipath:   false,
		DoDiscovery: true,
	}
	dev, err := lib_iscsi.Connect(*conn)
	if errout = err; err != nil {
		return status.Errorf(codes.Internal, "iscsi connect failed: %v", err)
	}

	att.target = target
	att.conn = conn
	att.outDev = dev
	return nil
}

func (ns *csifNodeServer) deleteFilter(att *csifVolumeAttachment) error {
	if att.conn != nil {
		connTarget := &att.conn.Targets[0]
		err := lib_iscsi.Disconnect(connTarget.Iqn, []string{connTarget.Portal + ":" + connTarget.Port})
		if err != nil {
			return fmt.Errorf("failed to disconnect server target: %v", err)
		}
		att.conn = nil
	}

	if !att.remoteFilterDeleted {
		client := filter.NewFilterClient(ns.cd.filterConn)
		req := &filter.DeleteFilterRequest{
			ClientDev: &filter.FilterDeviceInfo{
				Portal: ns.cd.tgtd.portal,
				Port:   ns.cd.tgtd.port,
				Iqn:    att.target.iqn,
			},
		}
		_, err := client.DeleteFilter(context.Background(), req)
		if err != nil {
			return fmt.Errorf("failed to delete filter: %v", err)
		}
		att.remoteFilterDeleted = true
	}

	if err := ns.cd.tgtd.DeleteDisk(att.target.id); err != nil {
		return fmt.Errorf("failed to delete tgtd disk: %v", err)
	}
	return nil
}

func (ns *csifNodeServer) stageDeviceMount(req *csi.NodeStageVolumeRequest, devPath string) error {
	mntPath := req.GetStagingTargetPath()
	notMP, err := ns.mounter.IsLikelyNotMountPoint(mntPath)
	if err != nil && !os.IsNotExist(err) {
		if err := os.MkdirAll(mntPath, 0777); err != nil {
			return fmt.Errorf("mkdir failed: %s: %v", mntPath, err)
		}
	}
	if !notMP {
		glog.V(4).Infof("Volume %s: already mounted", req.GetVolumeId())
		return nil
	}

	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()
	options := append([]string{}, mountFlags...)

	if err := ns.mounter.FormatAndMount(devPath, mntPath, fsType, options); err != nil {
		return fmt.Errorf("mount failed: volume %s, bdev %s, fs %s, path %s: %v",
			req.GetVolumeId(), devPath, fsType, mntPath, err)
	}
	return nil
}

func (ns *csifNodeServer) unstageDevice(req *csi.NodeUnstageVolumeRequest) error {
	targetPath := req.GetStagingTargetPath()

	if ok, err := mount.PathExists(targetPath); err != nil {
		return fmt.Errorf("check PathExists failed: %v", err)
	} else if !ok {
		glog.V(4).Infof("volume not mounted, skip: %v", targetPath)
		return nil
	}

	notMP, err := ns.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		return err
	}
	if !notMP {
		if err := ns.mounter.Unmount(targetPath); err != nil {
			return fmt.Errorf("unmount %s failed: %v", targetPath, err)
		}
	}

	if err := os.RemoveAll(targetPath); err != nil {
		return fmt.Errorf("remove %s failed: %v", targetPath, err)
	}
	return nil
}

func (ns *csifNodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	glog.V(4).Infof("NodeStageVolume")
	if len(req.GetVolumeId()) == 0 || len(req.GetStagingTargetPath()) == 0 || req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "wrong args")
	}
	vol, err := ns.createVolumeAttachment(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create volume attachment: %v", err)
	}

	bdev := vol.getPath()

	if req.GetVolumeCapability().GetBlock() != nil {
		vol.StagingPath = req.GetStagingTargetPath()
		return &csi.NodeStageVolumeResponse{}, nil
	}

	if err := ns.stageDeviceMount(req, bdev); err != nil {
		if err := ns.deleteVolumeAttachment(req.GetVolumeId()); err != nil {
			glog.Errorf("delete volume attachment failed: %v", err)
		}
		return nil, fmt.Errorf("format and mount failed: %v", err)
	}

	vol.StagingPath = req.GetStagingTargetPath()
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *csifNodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	glog.V(4).Infof("NodeUnstageVolume")
	if len(req.GetVolumeId()) == 0 || len(req.GetStagingTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "wrong args")
	}

	volID := req.GetVolumeId()
	vol, err := ns.getVolumeAttachment(volID)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
		// TODO: After plugin restart CO keeps asking me to unstage the volume. What to do?
	}

	if vol.StagingPath == "" { // if not staged
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	ns.unstageDevice(req) // if staging MP exists - unmount

	if err := ns.deleteVolumeAttachment(volID); err != nil {
		return nil, fmt.Errorf("failed to delete volume attachment: %v", err)
	}
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func mergeOptions(opts1 []string, opts2 []string) []string {
	contains1 := func(opt string) bool {
		for _, o := range opts1 {
			if o == opt {
				return true
			}
		}
		return false
	}

	for _, opt := range opts2 {
		if !contains1(opt) {
			opts1 = append(opts1, opt)
		}
	}
	return opts1
}

func (ns *csifNodeServer) publishVolumeMount(req *csi.NodePublishVolumeRequest, mountOptions []string) error {
	target := req.GetTargetPath()
	staging := req.GetStagingTargetPath()
	mode := req.VolumeCapability.GetMount()
	mountOptions = mergeOptions(mountOptions, mode.GetMountFlags())

	fsType := mode.GetFsType()
	if len(fsType) == 0 {
		fsType = "ext4"
	}

	if err := os.MkdirAll(target, 0000); err != nil {
		return fmt.Errorf("mkdir failed: %s: %v", target, err)
	}
	if err := os.Chmod(target, 0000); err != nil {
		return fmt.Errorf("chmod failed: %s: %v", target, err)
	}

	if err := ns.mounter.Mount(staging, target, fsType, mountOptions); err != nil {
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("remove failed: %s: %v", target, err)
		}
		return fmt.Errorf("mount failed: staging %s, target %s, fs %s: %v",
			staging, target, fsType, err)
	}

	return nil
}

func (ns *csifNodeServer) publishVolumeBlock(req *csi.NodePublishVolumeRequest, mountOptions []string) error {
	target := req.GetTargetPath()
	vol, err := ns.getVolumeAttachment(req.GetVolumeId())
	if err != nil {
		return status.Error(codes.NotFound, err.Error())
	}

	bdev := vol.getPath()

	if err := makeFile(target); err != nil {
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("remove failed: %s: %v", target, err)
		}
		return fmt.Errorf("makeFile failed: %s: %v", target, err)
	}

	if err := ns.mounter.Mount(bdev, target, "", mountOptions); err != nil {
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("remove failed: %s: %v", target, err)
		}
		return fmt.Errorf("mount failed: device %s, target %s: %v", bdev, target, err)
	}
	return nil
}

func (ns *csifNodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	glog.V(4).Infof("NodePublishVolume")
	if len(req.GetVolumeId()) == 0 || len(req.GetStagingTargetPath()) == 0 || req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "Wrong args")
	}
	_, err := ns.getVolumeAttachment(req.GetVolumeId()) // TODO: check mount capab.?
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	mountOptions := []string{"bind"}
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	if req.GetVolumeCapability().GetMount() != nil {
		if err := ns.publishVolumeMount(req, mountOptions); err != nil {
			return nil, err
		}
	} else {
		if err := ns.publishVolumeBlock(req, mountOptions); err != nil {
			return nil, err
		}
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *csifNodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if len(req.GetVolumeId()) == 0 || len(req.GetTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "wrong args")
	}

	target := req.GetTargetPath()

	_, err := ns.getVolumeAttachment(req.GetVolumeId())
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	notMP, err := ns.mounter.IsLikelyNotMountPoint(target)
	if (err == nil && notMP) || os.IsNotExist(err) {
		glog.V(4).Infof("NodeUnpublishVolume: %s not mounted: %v", target, err)
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	if err := ns.mounter.Unmount(target); err != nil {
		return nil, fmt.Errorf("unmount %s failed: %v", target, err)
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *csifNodeServer) NodeGetVolumeStats(_ context.Context, _ *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (ns *csifNodeServer) NodeExpandVolume(_ context.Context, _ *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (ns *csifNodeServer) getNSCapabilities() []*csi.NodeServiceCapability {
	rpcCap := []csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
	}

	var nsCap []*csi.NodeServiceCapability
	for _, cap := range rpcCap {
		nsCap = append(nsCap, &csi.NodeServiceCapability{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: cap,
				},
			},
		})
	}
	return nsCap
}

func (ns *csifNodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: ns.getNSCapabilities(),
	}, nil
}

func (ns *csifNodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	topology := &csi.Topology{
		Segments: map[string]string{TopologyKeyNode: ns.cd.nodeID},
	}
	return &csi.NodeGetInfoResponse{
		NodeId:             ns.cd.nodeID,
		AccessibleTopology: topology,
		MaxVolumesPerNode:  ns.cd.maxVolumesPerNode,
	}, nil
}
