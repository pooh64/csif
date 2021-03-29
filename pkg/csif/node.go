package csif

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
)

func (cd *csifDriver) NodeStageVolume(_ context.Context, _ *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (cd *csifDriver) NodeUnstageVolume(_ context.Context, _ *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (cd *csifDriver) NodePublishVolume(_ context.Context, _ *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
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
