package types

import "time"

// CreateSnapshotRequest represents the request to create a snapshot
type CreateSnapshotRequest struct {
	VMID        string `json:"vm_id" validate:"required" example:"vm-123"`
	Name        string `json:"name" validate:"required" example:"backup-2024-01-01"`
	Description string `json:"description" example:"Daily backup snapshot"`
	Memory      bool   `json:"memory" example:"false"`
	Quiesce     bool   `json:"quiesce" example:"true"`
}

// SnapshotResponse represents a snapshot operation response
type SnapshotResponse struct {
	TaskID     string `json:"task_id" example:"task-456"`
	SnapshotID string `json:"snapshot_id,omitempty" example:"snapshot-789"`
	Status     string `json:"status" example:"success"`
	Message    string `json:"message,omitempty" example:"Snapshot created successfully"`
}

// TaskStatusResponse represents the status of a snapshot task
type TaskStatusResponse struct {
	TaskID     string    `json:"task_id" example:"task-456"`
	Status     string    `json:"status" example:"success"`
	Progress   int       `json:"progress" example:"100"`
	SnapshotID string    `json:"snapshot_id,omitempty" example:"snapshot-789"`
	Message    string    `json:"message,omitempty" example:"Snapshot creation completed"`
	StartTime  time.Time `json:"start_time" example:"2024-01-01T10:00:00Z"`
	EndTime    *time.Time `json:"end_time,omitempty" example:"2024-01-01T10:05:00Z"`
}

// Snapshot represents a VM snapshot
type Snapshot struct {
	ID          string    `json:"id" example:"snapshot-789"`
	Name        string    `json:"name" example:"backup-2024-01-01"`
	Description string    `json:"description" example:"Daily backup snapshot"`
	CreateTime  time.Time `json:"create_time" example:"2024-01-01T10:00:00Z"`
	State       string    `json:"state" example:"poweredOn"`
	Quiesced    bool      `json:"quiesced" example:"true"`
	Memory      bool      `json:"memory" example:"false"`
}

// SnapshotListResponse represents a list of snapshots for a VM
type SnapshotListResponse struct {
	VMID      string     `json:"vm_id" example:"vm-123"`
	Snapshots []Snapshot `json:"snapshots"`
	Total     int        `json:"total" example:"5"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string    `json:"status" example:"healthy"`
	Timestamp time.Time `json:"timestamp" example:"2024-01-01T10:00:00Z"`
	Service   string    `json:"service" example:"vm-deep-inspection-demo"`
	Version   string    `json:"version,omitempty" example:"1.0.0"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error" example:"Invalid request"`
	Code    string `json:"code,omitempty" example:"VALIDATION_ERROR"`
	Details string `json:"details,omitempty" example:"VM ID is required"`
}