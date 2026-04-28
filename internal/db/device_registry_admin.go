package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"
)

const defaultDiagnosticsKeepLatestPerDevice = 20

type DeviceMountStateReport struct {
	DeviceID      string
	Mounted       bool
	MountPath     string
	LastMountTime int64
	LastError     string
}

type DevicePolicyRecord struct {
	DeviceID           string
	MountPath          string
	SyncRoots          []string
	ConflictPolicy     string
	ReadOnly           bool
	AutoConnect        bool
	BandwidthLimit     int64
	LogLevel           string
	LargeFileThreshold int64
	PolicyRevision     uint64
	UpdatedAtUnix      int64
}

type DeviceActionRecord struct {
	ActionID        string
	DeviceID        string
	ActionType      string
	PayloadJSON     string
	Status          string
	RequestedAtUnix int64
	DeliveredAtUnix int64
	CompletedAtUnix int64
}

type DeviceActionRequest struct {
	DeviceID              string
	ActionType            string
	PayloadJSON           string
	ReplaceSimilarPending bool
}

type DeviceDiagnosticRecord struct {
	DiagnosticsID  string
	DeviceID       string
	Category       string
	ContentType    string
	Summary        string
	Payload        []byte
	SizeBytes      int64
	UploadedAtUnix int64
}

func (r *DeviceRegistry) ensureAdminSchema() error {
	if err := ensureTableColumns(r.db, "device_registry", []columnDefinition{
		{Name: "mounted", Definition: "mounted INTEGER NOT NULL DEFAULT 0"},
		{Name: "mount_path", Definition: "mount_path TEXT NOT NULL DEFAULT ''"},
		{Name: "last_mount_time_unix", Definition: "last_mount_time_unix INTEGER NOT NULL DEFAULT 0"},
		{Name: "mount_last_error", Definition: "mount_last_error TEXT NOT NULL DEFAULT ''"},
		{Name: "last_mount_report_at_unix", Definition: "last_mount_report_at_unix INTEGER NOT NULL DEFAULT 0"},
	}); err != nil {
		return err
	}

	if _, err := r.db.Exec(`
CREATE TABLE IF NOT EXISTS device_policy (
    device_id TEXT PRIMARY KEY,
    mount_path TEXT NOT NULL DEFAULT '',
    sync_roots_json TEXT NOT NULL DEFAULT '[]',
    conflict_policy TEXT NOT NULL DEFAULT '',
    read_only INTEGER NOT NULL DEFAULT 0,
    auto_connect INTEGER NOT NULL DEFAULT 1,
    bandwidth_limit INTEGER NOT NULL DEFAULT 0,
    log_level TEXT NOT NULL DEFAULT '',
    large_file_threshold INTEGER NOT NULL DEFAULT 0,
    policy_revision INTEGER NOT NULL DEFAULT 1,
    updated_at_unix INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS device_actions (
    action_id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    action_type TEXT NOT NULL,
    payload_json TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    requested_at_unix INTEGER NOT NULL DEFAULT 0,
    delivered_at_unix INTEGER NOT NULL DEFAULT 0,
    completed_at_unix INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_device_actions_pending
ON device_actions(device_id, status, requested_at_unix DESC);

CREATE TABLE IF NOT EXISTS device_diagnostics (
    diagnostics_id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    payload BLOB NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    uploaded_at_unix INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_device_diagnostics_recent
ON device_diagnostics(device_id, uploaded_at_unix DESC, diagnostics_id DESC);
`); err != nil {
		return err
	}

	return nil
}

func (r *DeviceRegistry) UpdateMountState(input DeviceMountStateReport) (*DeviceRecord, error) {
	deviceID := strings.TrimSpace(input.DeviceID)
	if deviceID == "" {
		return nil, os.ErrInvalid
	}

	nowUnix := time.Now().Unix()
	lastMountTime := input.LastMountTime
	result, err := r.db.Exec(`
UPDATE device_registry
SET mounted = ?,
    mount_path = ?,
    last_mount_time_unix = CASE WHEN ? != 0 THEN ? ELSE last_mount_time_unix END,
    mount_last_error = ?,
    mount_state = CASE
        WHEN ? != 0 THEN 'mounted'
        WHEN ? != '' THEN 'error'
        ELSE 'unmounted'
    END,
    last_error = CASE WHEN ? != '' THEN ? ELSE last_error END,
    updated_at_unix = ?,
    last_mount_report_at_unix = ?,
    last_seen_at_unix = ?
WHERE device_id = ?;
`,
		boolToInt(input.Mounted),
		strings.TrimSpace(input.MountPath),
		lastMountTime,
		lastMountTime,
		strings.TrimSpace(input.LastError),
		boolToInt(input.Mounted),
		strings.TrimSpace(input.LastError),
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

func (r *DeviceRegistry) GetPolicy(deviceID string) (*DevicePolicyRecord, error) {
	row := r.db.QueryRow(`
SELECT device_id, mount_path, sync_roots_json, conflict_policy, read_only, auto_connect,
       bandwidth_limit, log_level, large_file_threshold, policy_revision, updated_at_unix
FROM device_policy
WHERE device_id = ?;
`, strings.TrimSpace(deviceID))

	var record DevicePolicyRecord
	var syncRootsJSON string
	var readOnly int64
	var autoConnect int64
	var policyRevision int64
	if err := row.Scan(
		&record.DeviceID,
		&record.MountPath,
		&syncRootsJSON,
		&record.ConflictPolicy,
		&readOnly,
		&autoConnect,
		&record.BandwidthLimit,
		&record.LogLevel,
		&record.LargeFileThreshold,
		&policyRevision,
		&record.UpdatedAtUnix,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, os.ErrNotExist
		}
		return nil, err
	}

	syncRoots, err := unmarshalStringList(syncRootsJSON)
	if err != nil {
		return nil, err
	}
	record.SyncRoots = syncRoots
	record.ReadOnly = readOnly != 0
	record.AutoConnect = autoConnect != 0
	record.PolicyRevision = uint64(policyRevision)
	return &record, nil
}

func (r *DeviceRegistry) UpsertPolicy(input DevicePolicyRecord) (*DevicePolicyRecord, error) {
	deviceID := strings.TrimSpace(input.DeviceID)
	if deviceID == "" {
		return nil, os.ErrInvalid
	}

	syncRootsJSON, err := marshalStringList(input.SyncRoots)
	if err != nil {
		return nil, err
	}

	nowUnix := time.Now().Unix()
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer rollbackTx(tx)

	if _, err := tx.Exec(`
INSERT INTO device_policy (
    device_id, mount_path, sync_roots_json, conflict_policy, read_only, auto_connect,
    bandwidth_limit, log_level, large_file_threshold, policy_revision, updated_at_unix
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(device_id) DO UPDATE SET
    mount_path = excluded.mount_path,
    sync_roots_json = excluded.sync_roots_json,
    conflict_policy = excluded.conflict_policy,
    read_only = excluded.read_only,
    auto_connect = excluded.auto_connect,
    bandwidth_limit = excluded.bandwidth_limit,
    log_level = excluded.log_level,
    large_file_threshold = excluded.large_file_threshold,
    policy_revision = excluded.policy_revision,
    updated_at_unix = excluded.updated_at_unix;
`,
		deviceID,
		strings.TrimSpace(input.MountPath),
		syncRootsJSON,
		strings.TrimSpace(input.ConflictPolicy),
		boolToInt(input.ReadOnly),
		boolToInt(input.AutoConnect),
		input.BandwidthLimit,
		strings.TrimSpace(input.LogLevel),
		input.LargeFileThreshold,
		int64(input.PolicyRevision),
		nowUnix,
	); err != nil {
		return nil, err
	}

	result, err := tx.Exec(`
UPDATE device_registry
SET policy_revision = ?,
    requires_policy_pull = 1,
    updated_at_unix = ?
WHERE device_id = ?;
`, int64(input.PolicyRevision), nowUnix, deviceID)
	if err != nil {
		return nil, err
	}
	if err := ensureRowsAffected(result); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetPolicy(deviceID)
}

func (r *DeviceRegistry) ResetPolicy(deviceID string, policyRevision uint64) error {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return os.ErrInvalid
	}

	nowUnix := time.Now().Unix()
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer rollbackTx(tx)

	if _, err := tx.Exec(`DELETE FROM device_policy WHERE device_id = ?;`, deviceID); err != nil {
		return err
	}

	result, err := tx.Exec(`
UPDATE device_registry
SET policy_revision = ?,
    requires_policy_pull = 1,
    updated_at_unix = ?
WHERE device_id = ?;
`, int64(policyRevision), nowUnix, deviceID)
	if err != nil {
		return err
	}
	if err := ensureRowsAffected(result); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *DeviceRegistry) EnqueueAction(input DeviceActionRequest) (*DeviceActionRecord, error) {
	deviceID := strings.TrimSpace(input.DeviceID)
	actionType := strings.TrimSpace(input.ActionType)
	if deviceID == "" || actionType == "" {
		return nil, os.ErrInvalid
	}

	actionID, err := newRandomID()
	if err != nil {
		return nil, err
	}

	nowUnix := time.Now().Unix()
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer rollbackTx(tx)

	if input.ReplaceSimilarPending {
		if _, err := tx.Exec(`
UPDATE device_actions
SET status = 'cancelled',
    completed_at_unix = ?
WHERE device_id = ? AND action_type = ? AND status = 'pending';
`, nowUnix, deviceID, actionType); err != nil {
			return nil, err
		}
	}

	if _, err := tx.Exec(`
INSERT INTO device_actions (
    action_id, device_id, action_type, payload_json, status,
    requested_at_unix, delivered_at_unix, completed_at_unix
) VALUES (?, ?, ?, ?, 'pending', ?, 0, 0);
`, actionID, deviceID, actionType, strings.TrimSpace(input.PayloadJSON), nowUnix); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(`
UPDATE device_registry
SET updated_at_unix = ?
WHERE device_id = ?;
`, nowUnix, deviceID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &DeviceActionRecord{
		ActionID:        actionID,
		DeviceID:        deviceID,
		ActionType:      actionType,
		PayloadJSON:     strings.TrimSpace(input.PayloadJSON),
		Status:          "pending",
		RequestedAtUnix: nowUnix,
	}, nil
}

func (r *DeviceRegistry) ListPendingActions(deviceID string, limit int) ([]*DeviceActionRecord, error) {
	return r.listActionsByStatus(deviceID, []string{"pending"}, limit)
}

func (r *DeviceRegistry) ListActiveActions(deviceID string, limit int) ([]*DeviceActionRecord, error) {
	return r.listActionsByStatus(deviceID, []string{"pending", "delivered"}, limit)
}

func (r *DeviceRegistry) MarkActionsDelivered(actionIDs []string) error {
	if len(actionIDs) == 0 {
		return nil
	}

	nowUnix := time.Now().Unix()
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(actionIDs)), ",")
	args := make([]any, 0, len(actionIDs)+1)
	args = append(args, nowUnix)
	for _, actionID := range actionIDs {
		args = append(args, strings.TrimSpace(actionID))
	}

	_, err := r.db.Exec(fmt.Sprintf(`
UPDATE device_actions
SET status = 'delivered',
    delivered_at_unix = CASE WHEN delivered_at_unix = 0 THEN ? ELSE delivered_at_unix END
WHERE action_id IN (%s) AND status = 'pending';
`, placeholders), args...)
	return err
}

func (r *DeviceRegistry) InsertDiagnostic(input DeviceDiagnosticRecord, keepLatest int) (*DeviceDiagnosticRecord, error) {
	deviceID := strings.TrimSpace(input.DeviceID)
	if deviceID == "" {
		return nil, os.ErrInvalid
	}

	diagnosticsID := strings.TrimSpace(input.DiagnosticsID)
	if diagnosticsID == "" {
		var err error
		diagnosticsID, err = newRandomID()
		if err != nil {
			return nil, err
		}
	}

	nowUnix := time.Now().Unix()
	payload := append([]byte(nil), input.Payload...)
	sizeBytes := input.SizeBytes
	if sizeBytes == 0 {
		sizeBytes = int64(len(payload))
	}
	if keepLatest <= 0 {
		keepLatest = defaultDiagnosticsKeepLatestPerDevice
	}

	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer rollbackTx(tx)

	if _, err := tx.Exec(`
INSERT INTO device_diagnostics (
    diagnostics_id, device_id, category, content_type, summary, payload, size_bytes, uploaded_at_unix
) VALUES (?, ?, ?, ?, ?, ?, ?, ?);
`,
		diagnosticsID,
		deviceID,
		strings.TrimSpace(input.Category),
		strings.TrimSpace(input.ContentType),
		strings.TrimSpace(input.Summary),
		payload,
		sizeBytes,
		nowUnix,
	); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(`
DELETE FROM device_diagnostics
WHERE device_id = ?
  AND diagnostics_id NOT IN (
      SELECT diagnostics_id
      FROM device_diagnostics
      WHERE device_id = ?
      ORDER BY uploaded_at_unix DESC, diagnostics_id DESC
      LIMIT ?
  );
`, deviceID, deviceID, keepLatest); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(`
UPDATE device_registry
SET updated_at_unix = ?
WHERE device_id = ?;
`, nowUnix, deviceID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &DeviceDiagnosticRecord{
		DiagnosticsID:  diagnosticsID,
		DeviceID:       deviceID,
		Category:       strings.TrimSpace(input.Category),
		ContentType:    strings.TrimSpace(input.ContentType),
		Summary:        strings.TrimSpace(input.Summary),
		Payload:        payload,
		SizeBytes:      sizeBytes,
		UploadedAtUnix: nowUnix,
	}, nil
}

func (r *DeviceRegistry) ListDiagnostics(deviceID string, limit int) ([]*DeviceDiagnosticRecord, error) {
	if limit <= 0 {
		limit = defaultDiagnosticsKeepLatestPerDevice
	}

	rows, err := r.db.Query(`
SELECT diagnostics_id, device_id, category, content_type, summary, payload, size_bytes, uploaded_at_unix
FROM device_diagnostics
WHERE device_id = ?
ORDER BY uploaded_at_unix DESC, diagnostics_id DESC
LIMIT ?;
`, strings.TrimSpace(deviceID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*DeviceDiagnosticRecord, 0, limit)
	for rows.Next() {
		record := &DeviceDiagnosticRecord{}
		if err := rows.Scan(
			&record.DiagnosticsID,
			&record.DeviceID,
			&record.Category,
			&record.ContentType,
			&record.Summary,
			&record.Payload,
			&record.SizeBytes,
			&record.UploadedAtUnix,
		); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type columnDefinition struct {
	Name       string
	Definition string
}

type tableQueryExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
}

func ensureTableColumns(database tableQueryExecer, table string, columns []columnDefinition) error {
	rows, err := database.Query(fmt.Sprintf(`PRAGMA table_info(%s);`, table))
	if err != nil {
		return err
	}
	defer rows.Close()

	existing := map[string]struct{}{}
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &primaryKey); err != nil {
			return err
		}
		existing[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, column := range columns {
		if _, ok := existing[strings.ToLower(column.Name)]; ok {
			continue
		}
		if _, err := database.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s;`, table, column.Definition)); err != nil {
			return err
		}
	}
	return nil
}

func rollbackTx(tx *sql.Tx) {
	if tx != nil {
		_ = tx.Rollback()
	}
}

func newRandomID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (r *DeviceRegistry) listActionsByStatus(deviceID string, statuses []string, limit int) ([]*DeviceActionRecord, error) {
	if limit <= 0 {
		limit = 16
	}

	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil, os.ErrInvalid
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(statuses)), ",")
	args := make([]any, 0, len(statuses)+2)
	args = append(args, deviceID)
	for _, status := range statuses {
		args = append(args, status)
	}
	args = append(args, limit)

	rows, err := r.db.Query(fmt.Sprintf(`
SELECT action_id, device_id, action_type, payload_json, status, requested_at_unix, delivered_at_unix, completed_at_unix
FROM device_actions
WHERE device_id = ? AND status IN (%s)
ORDER BY requested_at_unix DESC, action_id DESC
LIMIT ?;
`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]*DeviceActionRecord, 0, limit)
	for rows.Next() {
		record := &DeviceActionRecord{}
		if err := rows.Scan(
			&record.ActionID,
			&record.DeviceID,
			&record.ActionType,
			&record.PayloadJSON,
			&record.Status,
			&record.RequestedAtUnix,
			&record.DeliveredAtUnix,
			&record.CompletedAtUnix,
		); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}
