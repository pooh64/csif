package csif

import (
	"fmt"
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
	"k8s.io/utils/exec"
)

type csifNodeServer struct {
	cd      *csifDriver
	mounter mount.SafeFormatAndMount
	disks   map[string]*csifDisk
}

func newCsifNodeServer(driver *csifDriver) *csifNodeServer {
	mounter := mount.SafeFormatAndMount{
		Interface: mount.New(""),
		Exec:      exec.New(),
	}

	return &csifNodeServer{
		cd:      driver,
		mounter: mounter,
		disks:   map[string]*csifDisk{},
	}
}

func (ns *csifNodeServer) getDisk(volID string) (*csifDisk, error) {
	if vol, ok := ns.disks[volID]; ok {
		return vol, nil
	}
	return nil, fmt.Errorf("no volID=%s in volumes", volID)
}

func (ns *csifNodeServer) attachDisk(req *csi.NodeStageVolumeRequest) (*csifDisk, error) {
	disk := newCsifDisk(ns.cd)
	if err := disk.LoadContext(req.GetVolumeContext()); err != nil {
		return nil, fmt.Errorf("failed to load disk context: %v", err)
	}
	if err := disk.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect disk: %v", err)
	}

	ns.disks[req.VolumeId] = disk
	return disk, nil
}

func (ns *csifNodeServer) detachDisk(volumeID string) error {
	disk := ns.disks[volumeID]

	if err := disk.Disconnect(); err != nil {
		return fmt.Errorf("failed to disconnect disk: %v", err)
	}
	delete(ns.disks, volumeID)
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
	disk, err := ns.attachDisk(req)
	if err != nil {
		return nil, fmt.Errorf("failed to attach disk: %v", err)
	}
	bdev := disk.GetDevPath()

	if req.GetVolumeCapability().GetBlock() != nil {
		return &csi.NodeStageVolumeResponse{}, nil
	}

	if err := ns.stageDeviceMount(req, bdev); err != nil {
		if err := ns.detachDisk(req.GetVolumeId()); err != nil {
			glog.Errorf("failed to detach disk: %v", err)
		}
		return nil, fmt.Errorf("format and mount failed: %v", err)
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *csifNodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	glog.V(4).Infof("NodeUnstageVolume")
	if len(req.GetVolumeId()) == 0 || len(req.GetStagingTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "wrong args")
	}

	volID := req.GetVolumeId()
	_, err := ns.getDisk(volID)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
		// TODO: After plugin restart CO keeps asking me to unstage the volume. What to do?
	}

	ns.unstageDevice(req) // if staging MP exists - unmount

	if err := ns.detachDisk(volID); err != nil {
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
	disk, err := ns.getDisk(req.GetVolumeId())
	if err != nil {
		return status.Error(codes.NotFound, err.Error())
	}

	bdev := disk.GetDevPath()

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
	_, err := ns.getDisk(req.GetVolumeId()) // TODO: check mount capab.?
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

	_, err := ns.getDisk(req.GetVolumeId())
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
