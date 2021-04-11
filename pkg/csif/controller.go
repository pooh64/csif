package csif

import (
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (cd *csifDriver) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: cd.getCSCapabilities(),
	}, nil
}

func (cd *csifDriver) getCSCapabilities() []*csi.ControllerServiceCapability {
	rpcCap := []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT, // TODO: NI
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,          // TODO: NI
		//csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
		//csi.ControllerServiceCapability_RPC_CLONE_VOLUME, TODO: readonly
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

func (cd *csifDriver) validateCSCapability(c csi.ControllerServiceCapability_RPC_Type) error {
	if c == csi.ControllerServiceCapability_RPC_UNKNOWN {
		return nil
	}

	for _, cap := range cd.getCSCapabilities() {
		if c == cap.GetRpc().GetType() {
			return nil
		}
	}
	return status.Errorf(codes.InvalidArgument, "CSCapability unsupported: %s", c)
}

func obtainVolumeCapabilitiy(caps []*csi.VolumeCapability) (volAccessType, error) {
	isMount, isBlock := false, false

	for _, cap := range caps {
		if cap.GetMount() != nil {
			isMount = true
		}
		if cap.GetBlock() != nil {
			isBlock = true
		}
	}

	if isMount && isBlock {
		return volAccessMount, status.Error(codes.InvalidArgument, "block+mount access type")
	}

	if isBlock {
		return volAccessBlock, nil
	}
	return volAccessMount, nil
}

func (cd *csifDriver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (resp *csi.CreateVolumeResponse, finalErr error) {
	if err := cd.validateCSCapability(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		glog.V(3).Infof("invalid request: %v", req)
		return nil, err
	}

	if len(req.GetName()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "No volName in request")
	}

	caps := req.GetVolumeCapabilities()
	if caps == nil {
		return nil, status.Error(codes.InvalidArgument, "nil vol.caps")
	}

	accessType, err := obtainVolumeCapabilitiy(caps)
	if err != nil {
		return nil, err
	}

	// TODO: check capacity?
	capacity := int64(req.GetCapacityRange().GetRequiredBytes())

	// TODO: iscsi lun: topology restrictions same as the source volume?
	// note: identity.go: VOLUME_ACCESSIBILITY_CONSTRAINTS
	nodeTopo := csi.Topology{Segments: map[string]string{TopologyKeyNode: cd.nodeID}}
	topologies := []*csi.Topology{&nodeTopo}

	if req.GetVolumeContentSource() != nil {
		return nil, status.Error(codes.InvalidArgument, "VolumeContentSource feautures unsupported")
	}

	// If volume exists - verify parameters, respond
	if vol, err := cd.getVolumeByName(req.GetName()); err == nil {
		glog.V(4).Infof("%s volume exists, veifying parameters", req.GetName())
		if vol.Size != capacity {
			return nil, status.Errorf(codes.AlreadyExists, "vol.size mismatch")
		}
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:           vol.ID,
				CapacityBytes:      int64(vol.Size),
				VolumeContext:      req.GetParameters(),
				ContentSource:      req.GetVolumeContentSource(),
				AccessibleTopology: topologies,
			},
		}, nil
	}

	vol, err := cd.createVolume(req, capacity, accessType)
	if err != nil {
		return nil, fmt.Errorf("failed to create volume %v: %w", req.GetName(), err)
	}
	glog.V(4).Infof("volume: %s done", vol.ID)

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:           vol.ID,
			CapacityBytes:      req.GetCapacityRange().GetRequiredBytes(),
			VolumeContext:      req.GetParameters(),
			ContentSource:      req.GetVolumeContentSource(),
			AccessibleTopology: topologies, // TODO:
		},
	}, nil
}

func (cd *csifDriver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cd.validateCSCapability(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		glog.V(3).Infof("invalid request: %v", req)
		return nil, err
	}

	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "No volID in request")
	}

	volId := req.GetVolumeId()
	if err := cd.deleteVolume(volId); err != nil {
		return nil, fmt.Errorf("deleteVolume %v failed: %w", volId, err)
	}
	glog.V(4).Infof("volume %v deleted", volId)

	return &csi.DeleteVolumeResponse{}, nil
}

func (cd *csifDriver) ControllerPublishVolume(_ context.Context, _ *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (cd *csifDriver) ControllerUnpublishVolume(_ context.Context, _ *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (cd *csifDriver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (cd *csifDriver) ListVolumes(_ context.Context, _ *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (cd *csifDriver) GetCapacity(_ context.Context, _ *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (cd *csifDriver) CreateSnapshot(_ context.Context, _ *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "snapshots are unimplemented")
}

func (cd *csifDriver) DeleteSnapshot(_ context.Context, _ *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "snapshots are unimplemented")
}

func (cd *csifDriver) ListSnapshots(_ context.Context, _ *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "snapshots are unimplemented")
}

func (cd *csifDriver) ControllerExpandVolume(_ context.Context, _ *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (cd *csifDriver) ControllerGetVolume(_ context.Context, _ *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}
