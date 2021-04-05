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

func (cd *csifDriver) formatAndMount(req *csi.NodeStageVolumeRequest, devPath string) error {
	mntPath := req.GetStagingTargetPath()
	mounter := mount.SafeFormatAndMount{
		Interface: mount.New(""),
		Exec:      exec.New(),
	}
	notMP, err := mounter.IsLikelyNotMountPoint(mntPath)
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

	if err := mounter.FormatAndMount(devPath, mntPath, fsType, options); err != nil {
		return fmt.Errorf("mount failed: volume %s, bdev %s, fs %s, path %s: %v",
			req.GetVolumeId(), devPath, fsType, mntPath, err)
	}
	return nil
}

func (cd *csifDriver) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	glog.V(4).Infof("NodeStageVolume")
	if len(req.GetVolumeId()) == 0 || len(req.GetStagingTargetPath()) == 0 || req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "Wrong args")
	}
	vol, err := cd.getVolumeByID(req.GetVolumeId())
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// TODO: attach to iscsi target
	bdev, err := createBDev(vol.ImgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create bdev: %v", err)
	}

	if req.GetVolumeCapability().GetBlock() != nil {
		return &csi.NodeStageVolumeResponse{}, nil
	}

	if err := cd.formatAndMount(req, bdev); err != nil {
		if err := destroyBDev(vol.ImgPath); err != nil {
			glog.Errorf("destroy bdev failed: %v", err)
		}
		return nil, fmt.Errorf("format and mount failed: %v", err)
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (cd *csifDriver) NodeUnstageVolume(_ context.Context, _ *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
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

	mounter := mount.SafeFormatAndMount{
		Interface: mount.New(""),
		Exec:      exec.New(),
	}

	if err := mounter.Mount(staging, target, fsType, mountOptions); err != nil {
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("remove failed: %s: %v", target, err)
		}
		return fmt.Errorf("mount failed: staging %s, target %s, fs %s: %v",
			staging, target, fsType, err)
	}

	return nil
}

func (cd *csifDriver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	glog.V(4).Infof("NodePublishVolume")
	if len(req.GetVolumeId()) == 0 || len(req.GetStagingTargetPath()) == 0 || req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "Wrong args")
	}
	_, err := cd.getVolumeByID(req.GetVolumeId()) // TODO: check mount cap?
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
		panic("")
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

func (cd *csifDriver) NodeUnpublishVolume(_ context.Context, _ *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (cd *csifDriver) NodeGetVolumeStats(_ context.Context, _ *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (cd *csifDriver) NodeExpandVolume(_ context.Context, _ *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (cd *csifDriver) NodeGetCapabilities(_ context.Context, _ *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (cd *csifDriver) NodeGetInfo(_ context.Context, _ *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	panic("not implemented") // TODO: Implement
}
