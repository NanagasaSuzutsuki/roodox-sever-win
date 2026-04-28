package server

import (
	"errors"
	"os"
	"strings"
	"time"

	"roodox_server/internal/db"
	pb "roodox_server/proto"
)

func assignedConfigFromPolicyRecord(record *db.DevicePolicyRecord) AssignedConfig {
	if record == nil {
		return AssignedConfig{}
	}
	return AssignedConfig{
		MountPath:          record.MountPath,
		SyncRoots:          append([]string(nil), record.SyncRoots...),
		ConflictPolicy:     record.ConflictPolicy,
		ReadOnly:           record.ReadOnly,
		AutoConnect:        record.AutoConnect,
		BandwidthLimit:     record.BandwidthLimit,
		LogLevel:           record.LogLevel,
		LargeFileThreshold: record.LargeFileThreshold,
	}
}

func assignedConfigToProto(config AssignedConfig) *pb.AssignedConfigPolicy {
	return &pb.AssignedConfigPolicy{
		MountPath:          config.MountPath,
		SyncRoots:          append([]string(nil), config.SyncRoots...),
		ConflictPolicy:     config.ConflictPolicy,
		ReadOnly:           config.ReadOnly,
		AutoConnect:        config.AutoConnect,
		BandwidthLimit:     config.BandwidthLimit,
		LogLevel:           config.LogLevel,
		LargeFileThreshold: config.LargeFileThreshold,
	}
}

func assignedConfigFromProto(policy *pb.AssignedConfigPolicy) AssignedConfig {
	if policy == nil {
		return AssignedConfig{}
	}
	return AssignedConfig{
		MountPath:          strings.TrimSpace(policy.GetMountPath()),
		SyncRoots:          normalizeStringList(policy.GetSyncRoots()),
		ConflictPolicy:     strings.TrimSpace(policy.GetConflictPolicy()),
		ReadOnly:           policy.GetReadOnly(),
		AutoConnect:        policy.GetAutoConnect(),
		BandwidthLimit:     policy.GetBandwidthLimit(),
		LogLevel:           strings.TrimSpace(policy.GetLogLevel()),
		LargeFileThreshold: policy.GetLargeFileThreshold(),
	}
}

func buildDeviceSummary(record *db.DeviceRecord, now time.Time) *pb.DeviceSummary {
	if record == nil {
		return nil
	}
	return &pb.DeviceSummary{
		DeviceId:        record.DeviceID,
		DisplayName:     deviceDisplayName(record),
		Role:            record.DeviceRole,
		OverlayProvider: record.OverlayProvider,
		OverlayAddress:  record.OverlayAddress,
		OnlineState:     deviceOnlineState(record, now),
		LastSeenAt:      record.LastSeenAtUnix,
		SyncState:       deviceSyncState(record),
		MountState:      deviceMountState(record),
		ClientVersion:   record.ClientVersion,
		PolicyRevision:  record.PolicyRevision,
	}
}

func clientActionToProto(record *db.DeviceActionRecord, delivered bool) *pb.ClientAction {
	if record == nil {
		return nil
	}

	status := record.Status
	deliveredAt := record.DeliveredAtUnix
	if delivered {
		status = "delivered"
		if deliveredAt == 0 {
			deliveredAt = time.Now().Unix()
		}
	}

	return &pb.ClientAction{
		ActionId:        record.ActionID,
		ActionType:      record.ActionType,
		PayloadJson:     record.PayloadJSON,
		Status:          status,
		RequestedAtUnix: record.RequestedAtUnix,
		DeliveredAtUnix: deliveredAt,
		CompletedAtUnix: record.CompletedAtUnix,
	}
}

func diagnosticSummaryToProto(record *db.DeviceDiagnosticRecord) *pb.DiagnosticSummary {
	if record == nil {
		return nil
	}
	return &pb.DiagnosticSummary{
		DiagnosticsId:  record.DiagnosticsID,
		Category:       record.Category,
		ContentType:    record.ContentType,
		Summary:        record.Summary,
		SizeBytes:      record.SizeBytes,
		UploadedAtUnix: record.UploadedAtUnix,
	}
}

func isNotExistError(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
