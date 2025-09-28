package types

import "time"

// VMListRequest represents the request parameters for listing VMs
type VMListRequest struct {
	Datacenter string `form:"datacenter" json:"datacenter,omitempty" example:"Datacenter1"`
	Cluster    string `form:"cluster" json:"cluster,omitempty" example:"Cluster1"`
	PowerState string `form:"power_state" json:"power_state,omitempty" example:"poweredOn"`
	Name       string `form:"name" json:"name,omitempty" example:"web-server"`
	GuestOS    string `form:"guest_os" json:"guest_os,omitempty" example:"ubuntu"`
	Limit      int    `form:"limit" json:"limit,omitempty" example:"50"`
	Offset     int    `form:"offset" json:"offset,omitempty" example:"0"`
}

// VM represents a virtual machine with minimal information
type VM struct {
	UUID       string `json:"uuid" example:"502e7c6e-b5c3-4d0e-9a5a-8b9c1d2e3f4g"`
	Name       string `json:"name" example:"web-server-01"`
	PowerState string `json:"power_state" example:"poweredOn"`
}

// VMToolsInfo represents VMware Tools information
type VMToolsInfo struct {
	Status        string `json:"status" example:"toolsOk"`
	Version       string `json:"version" example:"12.1.5"`
	RunningStatus string `json:"running_status" example:"guestToolsRunning"`
	VersionStatus string `json:"version_status,omitempty" example:"guestToolsCurrent"`
}

// VMHardwareInfo represents VM hardware specifications
type VMHardwareInfo struct {
	NumCPU            int32  `json:"num_cpu" example:"2"`
	NumCoresPerSocket int32  `json:"num_cores_per_socket" example:"1"`
	MemoryMB          int32  `json:"memory_mb" example:"4096"`
	GuestFullName     string `json:"guest_full_name" example:"Ubuntu Linux (64-bit)"`
	Version           string `json:"version" example:"vmx-19"`
	FirmwareType      string `json:"firmware_type,omitempty" example:"bios"`
}

// VMNetworkInfo represents VM network configuration
type VMNetworkInfo struct {
	Name        string   `json:"name" example:"VM Network"`
	MacAddress  string   `json:"mac_address" example:"00:50:56:9a:12:34"`
	IPAddresses []string `json:"ip_addresses" example:"192.168.1.100"`
	NetworkType string   `json:"network_type" example:"portgroup"`
	Connected   bool     `json:"connected" example:"true"`
}

// VMStorageInfo represents VM storage information
type VMStorageInfo struct {
	Datastore string `json:"datastore" example:"datastore1"`
	SizeGB    int64  `json:"size_gb" example:"50"`
	UsedGB    int64  `json:"used_gb" example:"25"`
	Type      string `json:"type" example:"thin"`
	Path      string `json:"path" example:"[datastore1] web-server-01/web-server-01.vmdk"`
}

// VMListResponse represents the response for VM listing
type VMListResponse struct {
	Datacenter string `json:"datacenter" example:"Datacenter1"`
	VMs        []VM   `json:"vms"`
	Total      int    `json:"total" example:"150"`
}

// VMDetailsResponse represents detailed information about a single VM
type VMDetailsResponse struct {
	VM       VM                `json:"vm"`
	Networks []VMNetworkInfo   `json:"networks,omitempty"`
	Storage  []VMStorageInfo   `json:"storage,omitempty"`
	Events   []VMEvent         `json:"recent_events,omitempty"`
}

// VMEvent represents a VM-related event
type VMEvent struct {
	EventType   string    `json:"event_type" example:"VmPoweredOnEvent"`
	Description string    `json:"description" example:"Virtual machine powered on"`
	Timestamp   time.Time `json:"timestamp" example:"2024-01-15T14:30:00Z"`
	User        string    `json:"user,omitempty" example:"administrator@vsphere.local"`
	Host        string    `json:"host,omitempty" example:"esxi-host-01.example.com"`
}

// VMPowerState represents possible VM power states
type VMPowerState string

const (
	PowerStatePoweredOn  VMPowerState = "poweredOn"
	PowerStatePoweredOff VMPowerState = "poweredOff"
	PowerStateSuspended  VMPowerState = "suspended"
)

// VMToolsStatus represents possible VMware Tools statuses
type VMToolsStatus string

const (
	ToolsStatusNotInstalled VMToolsStatus = "toolsNotInstalled"
	ToolsStatusNotRunning   VMToolsStatus = "toolsNotRunning"
	ToolsStatusOld          VMToolsStatus = "toolsOld"
	ToolsStatusOk           VMToolsStatus = "toolsOk"
)

// VMSummary represents a lightweight VM summary for list operations
type VMSummary struct {
	UUID       string `json:"uuid" example:"502e7c6e-b5c3-4d0e-9a5a-8b9c1d2e3f4g"`
	Name       string `json:"name" example:"web-server-01"`
	PowerState string `json:"power_state" example:"poweredOn"`
	CPUCount   int32  `json:"cpu_count" example:"2"`
	MemoryMB   int32  `json:"memory_mb" example:"4096"`
	GuestOS    string `json:"guest_os" example:"Ubuntu Linux (64-bit)"`
	Datacenter string `json:"datacenter" example:"Datacenter1"`
	Cluster    string `json:"cluster" example:"Cluster1"`
}

// VMStatsResponse represents VM performance statistics
type VMStatsResponse struct {
	UUID        string            `json:"uuid" example:"502e7c6e-b5c3-4d0e-9a5a-8b9c1d2e3f4g"`
	Name        string            `json:"name" example:"web-server-01"`
	Timestamp   time.Time         `json:"timestamp" example:"2024-01-15T14:30:00Z"`
	CPUUsage    VMCPUStats        `json:"cpu_usage"`
	MemoryUsage VMMemoryStats     `json:"memory_usage"`
	DiskUsage   VMDiskStats       `json:"disk_usage"`
	NetworkUsage VMNetworkStats   `json:"network_usage"`
	Uptime      int64             `json:"uptime_seconds" example:"86400"`
}

// VMCPUStats represents CPU usage statistics
type VMCPUStats struct {
	UsagePercent float64 `json:"usage_percent" example:"45.2"`
	UsageMHz     int32   `json:"usage_mhz" example:"1200"`
	ReadyTime    int64   `json:"ready_time_ms" example:"50"`
}

// VMMemoryStats represents memory usage statistics
type VMMemoryStats struct {
	UsagePercent float64 `json:"usage_percent" example:"67.8"`
	UsageMB      int32   `json:"usage_mb" example:"2780"`
	ActiveMB     int32   `json:"active_mb" example:"2456"`
	BalloonedMB  int32   `json:"ballooned_mb" example:"0"`
	SwappedMB    int32   `json:"swapped_mb" example:"0"`
}

// VMDiskStats represents disk I/O statistics
type VMDiskStats struct {
	ReadIOPS     int64 `json:"read_iops" example:"150"`
	WriteIOPS    int64 `json:"write_iops" example:"75"`
	ReadMBps     float64 `json:"read_mbps" example:"12.5"`
	WriteMBps    float64 `json:"write_mbps" example:"8.3"`
	LatencyMS    float64 `json:"latency_ms" example:"2.1"`
}

// VMNetworkStats represents network I/O statistics
type VMNetworkStats struct {
	ReceiveMBps  float64 `json:"receive_mbps" example:"5.2"`
	TransmitMBps float64 `json:"transmit_mbps" example:"3.1"`
	ReceivePPS   int64   `json:"receive_pps" example:"450"`
	TransmitPPS  int64   `json:"transmit_pps" example:"280"`
}

// VMOperationRequest represents a request to perform an operation on a VM
type VMOperationRequest struct {
	Operation string                 `json:"operation" validate:"required,oneof=start stop restart suspend reset" example:"start"`
	Options   map[string]interface{} `json:"options,omitempty"`
}

// VMOperationResponse represents the response for a VM operation
type VMOperationResponse struct {
	TaskID    string `json:"task_id" example:"task-123"`
	Operation string `json:"operation" example:"start"`
	VMID      string `json:"vm_id" example:"vm-456"`
	Status    string `json:"status" example:"initiated"`
	Message   string `json:"message" example:"VM start operation initiated"`
}

// SnapshotCreateRequest represents a request to create a VM snapshot
type SnapshotCreateRequest struct {
	Name        string `json:"name" binding:"required" example:"backup-snapshot"`
	Description string `json:"description,omitempty" example:"Backup before upgrade"`
	Memory      bool   `json:"memory,omitempty" example:"false"`
	Quiesce     bool   `json:"quiesce,omitempty" example:"true"`
}

// SnapshotCreateResponse represents the response for snapshot creation
type SnapshotCreateResponse struct {
	SnapshotID  string `json:"snapshot_id" example:"snapshot-123"`
	Name        string `json:"name" example:"backup-snapshot"`
	VMID        string `json:"vm_id" example:"vm-456"`
	VMName      string `json:"vm_name" example:"web-server-01"`
	Status      string `json:"status" example:"completed"`
	Message     string `json:"message" example:"Snapshot created successfully"`
	CreatedTime string `json:"created_time,omitempty" example:"2024-01-15T14:30:00Z"`
}