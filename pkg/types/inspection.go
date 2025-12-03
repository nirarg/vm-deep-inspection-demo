package types

import (
	validationtypes "github.com/nirarg/v2v-vm-validations/pkg/types"
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
	VMName         string      `json:"vm_name" example:"web-server-01"`
	SnapshotName   string      `json:"snapshot_name" example:"backup-snapshot"`
	Status         string      `json:"status" example:"completed"`
	Message        string      `json:"message" example:"Inspection completed successfully"`
	InspectorType  string      `json:"inspector_type" example:"virt-inspector"`
	VirtInspector  interface{} `json:"virt_inspector,omitempty"`
	VirtV2V        interface{} `json:"virt_v2v,omitempty"`
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
