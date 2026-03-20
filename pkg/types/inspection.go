package types

import (
	validationtypes "github.com/kubev2v/vm-migration-detective/pkg/types"
)

// VMInspectionRequest represents a request to inspect a VM snapshot
type VMInspectionRequest struct {
	SnapshotName string `json:"snapshot_name" binding:"required" example:"backup-snapshot"`
}

// CloneRequest represents a request to create a clone from snapshot
type CloneRequest struct {
	SnapshotName string `json:"snapshot_name" binding:"required" example:"backup-snapshot"`
	CloneName    string `json:"clone_name,omitempty" example:"my-clone"`
}

// CloneResponse represents the response from clone creation
type CloneResponse struct {
	CloneName    string `json:"clone_name" example:"vm-clone-123"`
	VMName       string `json:"vm_name" example:"web-server-01"`
	SnapshotName string `json:"snapshot_name" example:"backup-snapshot"`
	Status       string `json:"status" example:"completed"`
	Message      string `json:"message" example:"Clone created successfully"`
}

// VMInspectionResponse represents the response from VM inspection
type VMInspectionResponse struct {
	VMName        string      `json:"vm_name" example:"web-server-01"`
	SnapshotName  string      `json:"snapshot_name" example:"backup-snapshot"`
	Status        string      `json:"status" example:"completed"`
	Message       string      `json:"message" example:"Inspection completed successfully"`
	InspectorType string      `json:"inspector_type" example:"virt-inspector"`
	VirtInspector interface{} `json:"virt_inspector,omitempty"`
	VirtV2V       interface{} `json:"virt_v2v,omitempty"`
}

// NewVirtInspectorResponse creates a response with virt-inspector data
func NewVirtInspectorResponse(vmName, snapshotName, message string, data *validationtypes.VirtInspectorXML) VMInspectionResponse {
	return VMInspectionResponse{
		VMName:        vmName,
		SnapshotName:  snapshotName,
		Status:        "completed",
		Message:       message,
		InspectorType: "virt-inspector",
		VirtInspector: data,
	}
}

// NewVirtV2VInspectorResponse creates a response with virt-v2v-inspector data
func NewVirtV2VInspectorResponse(vmName, snapshotName, message string, data *validationtypes.VirtV2VInspectorXML) VMInspectionResponse {
	return VMInspectionResponse{
		VMName:        vmName,
		SnapshotName:  snapshotName,
		Status:        "completed",
		Message:       message,
		InspectorType: "virt-v2v-inspector",
		VirtV2V:       data,
	}
}

// CheckResult represents the result of a single validation check
type CheckResult struct {
	CheckType string  `json:"check_type" example:"fstab"`
	Valid     bool    `json:"valid" example:"true"`
	Message   string  `json:"message" example:"Fstab is migrateable - no /dev/disk/by-path/ entries found"`
	Error     *string `json:"error,omitempty" example:"Failed to run inspection: connection timeout"`
}

// CheckResponse represents the response from running validation checks
type CheckResponse struct {
	VMName       string        `json:"vm_name" example:"web-server-01"`
	SnapshotName string        `json:"snapshot_name" example:"backup-snapshot"`
	Results      []CheckResult `json:"results"`
	AllValid     bool          `json:"all_valid" example:"true"`
}

// DetectConcern represents a concern found during detection checks (vmdetect API)
type DetectConcern struct {
	ID       string `json:"id" example:"fstab-by-path-device"`
	Category string `json:"category" example:"Warning"`
	Label    string `json:"label" example:"Fstab contains /dev/disk/by-path/ entry"`
	Message  string `json:"message" example:"Fstab contains /dev/disk/by-path/ entries which are not migrateable"`
}

// DetectCheckResult represents the result of a single detection check
type DetectCheckResult struct {
	CheckType string          `json:"check_type" example:"fstab"`
	Passed    bool            `json:"passed" example:"true"`
	Concerns  []DetectConcern `json:"concerns,omitempty"`
	Error     *string         `json:"error,omitempty"`
}

// DetectResponse represents the response from the vmdetect API
type DetectResponse struct {
	VMName       string              `json:"vm_name" example:"web-server-01"`
	SnapshotName string              `json:"snapshot_name" example:"backup-snapshot"`
	Results      []DetectCheckResult `json:"results"`
	AllConcerns  []DetectConcern     `json:"all_concerns"`
	Passed       bool                `json:"passed" example:"true"`
}
