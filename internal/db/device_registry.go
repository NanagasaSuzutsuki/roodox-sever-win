package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
)

type DeviceRegistry struct {
	db *sql.DB
}

type DeviceRecord struct {
	DeviceID                 string
	DeviceName               string
	DeviceRole               string
	AssignedDeviceLabel      string
	ClientVersion            string
	Platform                 string
	OverlayProvider          string
	OverlayAddress           string
	Capabilities             []string
	ServerID                 string
	DeviceGroup              string
	LastSessionID            string
	HeartbeatIntervalSeconds uint32
	PolicyRevision           uint64
	RequiresPolicyPull       bool
	OverlayConnected         bool
	GRPCConnected            bool
	LastSyncTimeUnix         int64
	LastError                string
	MountState               string
	Mounted                  bool
	MountPath                string
	LastMountTimeUnix        int64
	MountLastError           string
	SyncStateSummary         string
	CurrentTaskCount         uint32
	SyncLastSuccessTime      int64
	SyncLastError            string
	ConflictCount            uint64
	QueueDepth               uint32
	SyncSummary              string
	CreatedAtUnix            int64
	UpdatedAtUnix            int64
	LastRegisteredAtUnix     int64
	LastHeartbeatAtUnix      int64
	LastSyncReportAtUnix     int64
	LastMountReportAtUnix    int64
	LastSeenAtUnix           int64
}

type DeviceRegistration struct {
	DeviceID                 string
	DeviceName               string
	DeviceRole               string
	AssignedDeviceLabel      string
	ClientVersion            string
	Platform                 string
	OverlayProvider          string
	OverlayAddress           string
	Capabilities             []string
	ServerID                 string
	DeviceGroup              string
	HeartbeatIntervalSeconds uint32
	PolicyRevision           uint64
	RequiresPolicyPull       bool
	OverlayConnected         bool
}

type DeviceHeartbeat struct {
	DeviceID         string
	SessionID        string
	TimestampUnix    int64
	OverlayConnected bool
	GRPCConnected    bool
	LastSyncTimeUnix int64
	LastError        string
	MountState       string
	SyncStateSummary string
}

type DeviceSyncStateReport struct {
	DeviceID         string
	CurrentTaskCount uint32
	LastSuccessTime  int64
	LastError        string
	ConflictCount    uint64
	QueueDepth       uint32
	Summary          string
}

func NewDeviceRegistry(database *DB) (*DeviceRegistry, error) {
	return &DeviceRegistry{db: database.Sql}, nil
}

func (r *DeviceRegistry) Register(input DeviceRegistration) (*DeviceRecord, error) {
	nowUnix := time.Now().Unix()
	deviceID := strings.TrimSpace(input.DeviceID)
	if deviceID == "" {
		return nil, os.ErrInvalid
	}

	label := strings.TrimSpace(input.AssignedDeviceLabel)
	if label == "" {
		label = fallbackDeviceLabel(input.DeviceName, deviceID)
	}

	capabilitiesJSON, err := marshalStringList(input.Capabilities)
	if err != nil {
		return nil, err
	}

	overlayConnected := input.OverlayConnected
	if !overlayConnected && strings.TrimSpace(input.OverlayProvider) == "" {
		overlayConnected = true
	}

	_, err = r.db.Exec(`
INSERT INTO device_registry (
    device_id, device_name, device_role, assigned_device_label, client_version, platform,
    overlay_provider, overlay_address, capabilities_json, server_id, device_group,
    heartbeat_interval_seconds, policy_revision, requires_policy_pull,
    overlay_connected, grpc_connected, created_at_unix, updated_at_unix,
    last_registered_at_unix, last_seen_at_unix
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(device_id) DO UPDATE SET
    device_name = excluded.device_name,
    device_role = excluded.device_role,
    assigned_device_label = excluded.assigned_device_label,
    client_version = excluded.client_version,
    platform = excluded.platform,
    overlay_provider = excluded.overlay_provider,
    overlay_address = excluded.overlay_address,
    capabilities_json = excluded.capabilities_json,
    server_id = excluded.server_id,
    device_group = excluded.device_group,
    heartbeat_interval_seconds = excluded.heartbeat_interval_seconds,
    overlay_connected = excluded.overlay_connected,
    grpc_connected = excluded.grpc_connected,
    updated_at_unix = excluded.updated_at_unix,
    last_registered_at_unix = excluded.last_registered_at_unix,
    last_seen_at_unix = excluded.last_seen_at_unix;
`,
		deviceID,
		strings.TrimSpace(input.DeviceName),
		strings.TrimSpace(input.DeviceRole),
		label,
		strings.TrimSpace(input.ClientVersion),
		strings.TrimSpace(input.Platform),
		strings.TrimSpace(input.OverlayProvider),
		strings.TrimSpace(input.OverlayAddress),
		capabilitiesJSON,
		strings.TrimSpace(input.ServerID),
		strings.TrimSpace(input.DeviceGroup),
		int64(input.HeartbeatIntervalSeconds),
		int64(input.PolicyRevision),
		boolToInt(input.RequiresPolicyPull),
		boolToInt(overlayConnected),
		1,
		nowUnix,
		nowUnix,
		nowUnix,
		nowUnix,
	)
	if err != nil {
		return nil, err
	}

	return r.Get(deviceID)
}

func (r *DeviceRegistry) UpdateHeartbeat(input DeviceHeartbeat) (*DeviceRecord, error) {
	deviceID := strings.TrimSpace(input.DeviceID)
	if deviceID == "" {
		return nil, os.ErrInvalid
	}

	timestampUnix := input.TimestampUnix
	if timestampUnix == 0 {
		timestampUnix = time.Now().Unix()
	}

	result, err := r.db.Exec(`
UPDATE device_registry
SET last_session_id = ?,
    overlay_connected = ?,
    grpc_connected = ?,
    last_sync_time_unix = CASE WHEN ? != 0 THEN ? ELSE last_sync_time_unix END,
    last_error = ?,
    mount_state = ?,
    sync_state_summary = ?,
    updated_at_unix = ?,
    last_heartbeat_at_unix = ?,
    last_seen_at_unix = ?
WHERE device_id = ?;
`,
		strings.TrimSpace(input.SessionID),
		boolToInt(input.OverlayConnected),
		boolToInt(input.GRPCConnected),
		input.LastSyncTimeUnix,
		input.LastSyncTimeUnix,
		strings.TrimSpace(input.LastError),
		strings.TrimSpace(input.MountState),
		strings.TrimSpace(input.SyncStateSummary),
		timestampUnix,
		timestampUnix,
		timestampUnix,
		deviceID,
	)
	if err != nil {
		return nil, err
	}
	if err := ensureRowsAffected(result); err != nil {
		return nil, err
	}

	return r.Get(deviceID)
}

func (r *DeviceRegistry) UpdateSyncState(input DeviceSyncStateReport) (*DeviceRecord, error) {
	deviceID := strings.TrimSpace(input.DeviceID)
	if deviceID == "" {
		return nil, os.ErrInvalid
	}

	nowUnix := time.Now().Unix()
	result, err := r.db.Exec(`
UPDATE device_registry
SET current_task_count = ?,
    sync_last_success_time = CASE WHEN ? != 0 THEN ? ELSE sync_last_success_time END,
    sync_last_error = ?,
    conflict_count = ?,
    queue_depth = ?,
    sync_summary = ?,
    sync_state_summary = ?,
    last_sync_time_unix = CASE WHEN ? != 0 THEN ? ELSE last_sync_time_unix END,
    last_error = CASE WHEN ? != '' THEN ? ELSE last_error END,
    grpc_connected = 1,
    updated_at_unix = ?,
    last_sync_report_at_unix = ?,
    last_seen_at_unix = ?
WHERE device_id = ?;
`,
		int64(input.CurrentTaskCount),
		input.LastSuccessTime,
		input.LastSuccessTime,
		strings.TrimSpace(input.LastError),
		int64(input.ConflictCount),
		int64(input.QueueDepth),
		strings.TrimSpace(input.Summary),
		strings.TrimSpace(input.Summary),
		input.LastSuccessTime,
		input.LastSuccessTime,
		strings.TrimSpace(input.LastError),
		strings.TrimSpace(input.LastError),
		nowUnix,
		nowUnix,
		nowUnix,
		deviceID,
	)
	if err != nil {
		return nil, err
	}
	if err := ensureRowsAffected(result); err != nil {
		return nil, err
	}

	return r.Get(deviceID)
}

func (r *DeviceRegistry) Get(deviceID string) (*DeviceRecord, error) {
	row := r.db.QueryRow(`
SELECT device_id, device_name, device_role, assigned_device_label, client_version, platform,
       overlay_provider, overlay_address, capabilities_json, server_id, device_group,
       last_session_id, heartbeat_interval_seconds, policy_revision, requires_policy_pull,
       overlay_connected, grpc_connected, last_sync_time_unix, last_error, mount_state,
       mounted, mount_path, last_mount_time_unix, mount_last_error,
       sync_state_summary, current_task_count, sync_last_success_time, sync_last_error,
       conflict_count, queue_depth, sync_summary, created_at_unix, updated_at_unix,
       last_registered_at_unix, last_heartbeat_at_unix, last_sync_report_at_unix,
       last_mount_report_at_unix, last_seen_at_unix
FROM device_registry
WHERE device_id = ?;
`, strings.TrimSpace(deviceID))

	record, err := scanDeviceRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, os.ErrNotExist
	}
	return record, err
}

func (r *DeviceRegistry) MarkPolicyPulled(deviceID string) error {
	result, err := r.db.Exec(`
UPDATE device_registry
SET requires_policy_pull = 0,
    updated_at_unix = ?
WHERE device_id = ?;
`, time.Now().Unix(), strings.TrimSpace(deviceID))
	if err != nil {
		return err
	}
	return ensureRowsAffected(result)
}

func (r *DeviceRegistry) List() ([]*DeviceRecord, error) {
	rows, err := r.db.Query(`
SELECT device_id, device_name, device_role, assigned_device_label, client_version, platform,
       overlay_provider, overlay_address, capabilities_json, server_id, device_group,
       last_session_id, heartbeat_interval_seconds, policy_revision, requires_policy_pull,
       overlay_connected, grpc_connected, last_sync_time_unix, last_error, mount_state,
       mounted, mount_path, last_mount_time_unix, mount_last_error,
       sync_state_summary, current_task_count, sync_last_success_time, sync_last_error,
       conflict_count, queue_depth, sync_summary, created_at_unix, updated_at_unix,
       last_registered_at_unix, last_heartbeat_at_unix, last_sync_report_at_unix,
       last_mount_report_at_unix, last_seen_at_unix
FROM device_registry
ORDER BY last_seen_at_unix DESC, updated_at_unix DESC, device_id ASC;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*DeviceRecord
	for rows.Next() {
		record, scanErr := scanDeviceRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDeviceRecord(scanner rowScanner) (*DeviceRecord, error) {
	var record DeviceRecord
	var capabilitiesJSON string
	var heartbeatInterval int64
	var policyRevision int64
	var requiresPolicyPull int64
	var overlayConnected int64
	var grpcConnected int64
	var mounted int64
	var currentTaskCount int64
	var conflictCount int64
	var queueDepth int64

	err := scanner.Scan(
		&record.DeviceID,
		&record.DeviceName,
		&record.DeviceRole,
		&record.AssignedDeviceLabel,
		&record.ClientVersion,
		&record.Platform,
		&record.OverlayProvider,
		&record.OverlayAddress,
		&capabilitiesJSON,
		&record.ServerID,
		&record.DeviceGroup,
		&record.LastSessionID,
		&heartbeatInterval,
		&policyRevision,
		&requiresPolicyPull,
		&overlayConnected,
		&grpcConnected,
		&record.LastSyncTimeUnix,
		&record.LastError,
		&record.MountState,
		&mounted,
		&record.MountPath,
		&record.LastMountTimeUnix,
		&record.MountLastError,
		&record.SyncStateSummary,
		&currentTaskCount,
		&record.SyncLastSuccessTime,
		&record.SyncLastError,
		&conflictCount,
		&queueDepth,
		&record.SyncSummary,
		&record.CreatedAtUnix,
		&record.UpdatedAtUnix,
		&record.LastRegisteredAtUnix,
		&record.LastHeartbeatAtUnix,
		&record.LastSyncReportAtUnix,
		&record.LastMountReportAtUnix,
		&record.LastSeenAtUnix,
	)
	if err != nil {
		return nil, err
	}

	capabilities, err := unmarshalStringList(capabilitiesJSON)
	if err != nil {
		return nil, err
	}
	record.Capabilities = capabilities
	record.HeartbeatIntervalSeconds = uint32(heartbeatInterval)
	record.PolicyRevision = uint64(policyRevision)
	record.RequiresPolicyPull = requiresPolicyPull != 0
	record.OverlayConnected = overlayConnected != 0
	record.GRPCConnected = grpcConnected != 0
	record.Mounted = mounted != 0
	record.CurrentTaskCount = uint32(currentTaskCount)
	record.ConflictCount = uint64(conflictCount)
	record.QueueDepth = uint32(queueDepth)

	return &record, nil
}

func ensureRowsAffected(result sql.Result) error {
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return os.ErrNotExist
	}
	return nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func marshalStringList(values []string) (string, error) {
	normalized := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func unmarshalStringList(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return values, nil
}

func fallbackDeviceLabel(deviceName, deviceID string) string {
	if trimmed := strings.TrimSpace(deviceName); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(deviceID)
}
