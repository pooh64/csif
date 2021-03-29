package csif

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
)

func (cd *csifDriver) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: cd.getControllerServiceCapabilities(),
	}, nil
}

func (cd *csifDriver) getControllerServiceCapabilities() []*csi.ControllerServiceCapability {
	rpcCap := []csi.ControllerServiceCapability_RPC_Type{
		//csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		//csi.ControllerServiceCapability_RPC_GET_CAPACITY,
	}
	var csCap []*csi.ControllerServiceCapability

	for _, cap := range rpcCap {
		csCap = append(csCap, &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cap,
				},
			},
		})
	}
	return csCap
}

func (cd *csifDriver) CreateVolume(_ context.Context, _ *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (cd *csifDriver) DeleteVolume(_ context.Context, _ *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (cd *csifDriver) ControllerPublishVolume(_ context.Context, _ *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (cd *csifDriver) ControllerUnpublishVolume(_ context.Context, _ *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (cd *csifDriver) ValidateVolumeCapabilities(_ context.Context, _ *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (cd *csifDriver) ListVolumes(_ context.Context, _ *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (cd *csifDriver) GetCapacity(_ context.Context, _ *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (cd *csifDriver) CreateSnapshot(_ context.Context, _ *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (cd *csifDriver) DeleteSnapshot(_ context.Context, _ *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (cd *csifDriver) ListSnapshots(_ context.Context, _ *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (cd *csifDriver) ControllerExpandVolume(_ context.Context, _ *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (cd *csifDriver) ControllerGetVolume(_ context.Context, _ *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}
