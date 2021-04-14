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
)

func (cd *csifDriver) stageDeviceMount(req *csi.NodeStageVolumeRequest, devPath string) error {
	mntPath := req.GetStagingTargetPath()
	notMP, err := cd.mounter.IsLikelyNotMountPoint(mntPath)
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

	if err := cd.mounter.FormatAndMount(devPath, mntPath, fsType, options); err != nil {
		return fmt.Errorf("mount failed: volume %s, bdev %s, fs %s, path %s: %v",
			req.GetVolumeId(), devPath, fsType, mntPath, err)
	}
	return nil
}

func (cd *csifDriver) unstageDevice(req *csi.NodeUnstageVolumeRequest) error {
	targetPath := req.GetStagingTargetPath()

	if ok, err := mount.PathExists(targetPath); err != nil {
		return fmt.Errorf("check PathExists failed: %v", err)
	} else if !ok {
		glog.V(4).Infof("volume not mounted, skip: %v", targetPath)
		return nil
	}

	notMP, err := cd.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		return err
	}
	if !notMP {
		if err := cd.mounter.Unmount(targetPath); err != nil {
			return fmt.Errorf("unmount %s failed: %v", targetPath, err)
		}
	}

	if err := os.RemoveAll(targetPath); err != nil {
		return fmt.Errorf("remove %s failed: %v", targetPath, err)
	}
	return nil
}

func (cd *csifDriver) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	glog.V(4).Infof("NodeStageVolume")
	if len(req.GetVolumeId()) == 0 || len(req.GetStagingTargetPath()) == 0 || req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "wrong args")
	}
	vol, err := cd.getVolumeByID(req.GetVolumeId())
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	bdev, err := vol.Disk.Attach()
	if err != nil {
		return nil, fmt.Errorf("failed to attach disk: %v", err)
	}

	if req.GetVolumeCapability().GetBlock() != nil {
		vol.StagingPath = req.GetStagingTargetPath()
		return &csi.NodeStageVolumeResponse{}, nil
	}

	if err := cd.stageDeviceMount(req, bdev); err != nil {
		if err := vol.Disk.Detach(); err != nil {
			glog.Errorf("destroy bdev failed: %v", err)
		}
		return nil, fmt.Errorf("format and mount failed: %v", err)
	}

	vol.StagingPath = req.GetStagingTargetPath()
	return &csi.NodeStageVolumeResponse{}, nil
}

func (cd *csifDriver) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	glog.V(4).Infof("NodeUnstageVolume")
	if len(req.GetVolumeId()) == 0 || len(req.GetStagingTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "wrong args")
	}

	vol, err := cd.getVolumeByID(req.GetVolumeId())
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	if vol.StagingPath == "" { // if not staged
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	cd.unstageDevice(req) // if staging MP exists - unmount

	if err = vol.Disk.Detach(); err != nil {
		return nil, fmt.Errorf("detach disk failed: %v", err)
	}
	vol.StagingPath = "" // unstaged
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

func (cd *csifDriver) publishVolumeMount(req *csi.NodePublishVolumeRequest, mountOptions []string) error {
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

	if err := cd.mounter.Mount(staging, target, fsType, mountOptions); err != nil {
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("remove failed: %s: %v", target, err)
		}
		return fmt.Errorf("mount failed: staging %s, target %s, fs %s: %v",
			staging, target, fsType, err)
	}

	return nil
}

func (cd *csifDriver) publishVolumeBlock(req *csi.NodePublishVolumeRequest, mountOptions []string) error {
	target := req.GetTargetPath()
	vol, err := cd.getVolumeByID(req.GetVolumeId())
	if err != nil {
		return status.Error(codes.NotFound, err.Error())
	}

	dev, err := vol.Disk.GetPath()
	if err != nil {
		return fmt.Errorf("disk getpath failed: %v", err)
	}

	if err := makeFile(target); err != nil {
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("remove failed: %s: %v", target, err)
		}
		return fmt.Errorf("makeFile failed: %s: %v", target, err)
	}

	if err := cd.mounter.Mount(dev, target, "", mountOptions); err != nil {
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("remove failed: %s: %v", target, err)
		}
		return fmt.Errorf("mount failed: device %s, target %s: %v", dev, target, err)
	}
	return nil
}

func (cd *csifDriver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	glog.V(4).Infof("NodePublishVolume")
	if len(req.GetVolumeId()) == 0 || len(req.GetStagingTargetPath()) == 0 || req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "Wrong args")
	}
	_, err := cd.getVolumeByID(req.GetVolumeId()) // TODO: check mount capab.?
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	mountOptions := []string{"bind"}
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	if req.GetVolumeCapability().GetMount() != nil {
		if err := cd.publishVolumeMount(req, mountOptions); err != nil {
			return nil, err
		}
	} else {
		if err := cd.publishVolumeBlock(req, mountOptions); err != nil {
			return nil, err
		}
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

func (cd *csifDriver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if len(req.GetVolumeId()) == 0 || len(req.GetTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "wrong args")
	}

	target := req.GetTargetPath()

	_, err := cd.getVolumeByID(req.GetVolumeId())
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	notMP, err := cd.mounter.IsLikelyNotMountPoint(target)
	if (err == nil && notMP) || os.IsNotExist(err) {
		glog.V(4).Infof("NodeUnpublishVolume: %s not mounted: %v", target, err)
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	if err := cd.mounter.Unmount(target); err != nil {
		return nil, fmt.Errorf("unmount %s failed: %v", target, err)
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (cd *csifDriver) NodeGetVolumeStats(_ context.Context, _ *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (cd *csifDriver) NodeExpandVolume(_ context.Context, _ *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (cd *csifDriver) getNSCapabilities() []*csi.NodeServiceCapability {
	rpcCap := []csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
		csi.NodeServiceCapability_RPC_EXPAND_VOLUME, // TODO: NI
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

func (cd *csifDriver) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: cd.getNSCapabilities(),
	}, nil
}

func (cd *csifDriver) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	topology := &csi.Topology{
		Segments: map[string]string{TopologyKeyNode: cd.nodeID},
	}
	return &csi.NodeGetInfoResponse{
		NodeId:             cd.nodeID,
		AccessibleTopology: topology,
		MaxVolumesPerNode:  cd.maxVolumesPerNode,
	}, nil
}
