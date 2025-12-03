package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nirarg/v2v-vm-validations/pkg/inspection"
	"github.com/nirarg/vm-deep-inspection-demo/internal/vmware"
	"github.com/nirarg/vm-deep-inspection-demo/pkg/types"
	"github.com/sirupsen/logrus"
)

// VMHandler handles VM-related API requests
type VMHandler struct {
	vmService *vmware.VMService
	vmClient  *vmware.Client
	logger    *logrus.Logger
}

// NewVMHandler creates a new VM handler instance
func NewVMHandler(vmService *vmware.VMService, vmClient *vmware.Client, logger *logrus.Logger) *VMHandler {
	return &VMHandler{
		vmService: vmService,
		vmClient:  vmClient,
		logger:    logger,
	}
}

// ListVMs godoc
// @Summary List all virtual machines
// @Description Get a list of all virtual machines with optional name filtering
// @Tags vms
// @Accept json
// @Produce json
// @Param name_contains query string false "Filter VMs where name contains this string" example("web")
// @Success 200 {object} types.VMListResponse "List of virtual machines"
// @Failure 500 {object} types.ErrorResponse "Internal server error"
// @Failure 503 {object} types.ErrorResponse "vSphere connection unavailable"
// @Router /api/v1/vms [get]
func (h *VMHandler) ListVMs(c *gin.Context) {
	nameContains := c.Query("name_contains")

	h.logger.WithField("name_contains", nameContains).Info("Listing VMs")

	// Build filter from query parameters
	filter := vmware.VMFilter{
		Name: nameContains,
	}

	result, err := h.vmService.ListVMs(c.Request.Context(), filter)
	if err != nil {
		h.logger.WithError(err).Error("Failed to list VMs")

		if isConnectionError(err) {
			c.JSON(http.StatusServiceUnavailable, types.ErrorResponse{
				Error:   "vSphere connection unavailable",
				Code:    "VSPHERE_UNAVAILABLE",
				Details: "Unable to connect to vSphere. Please try again later.",
			})
			return
		}

		if isAuthenticationError(err) {
			c.JSON(http.StatusServiceUnavailable, types.ErrorResponse{
				Error:   "vSphere authentication failed",
				Code:    "VSPHERE_AUTH_FAILED",
				Details: "Authentication to vSphere failed. Please check configuration.",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, types.ErrorResponse{
			Error:   "Failed to retrieve VMs",
			Code:    "VM_LIST_FAILED",
			Details: "An error occurred while retrieving virtual machines from vSphere",
		})
		return
	}

	// Convert VMInfos to VMs
	var vms []types.VM
	for _, vmInfo := range result.VMs {
		vms = append(vms, h.convertVMInfoToVM(vmInfo))
	}

	response := types.VMListResponse{
		Datacenter: result.Datacenter,
		VMs:        vms,
		Total:      result.Total,
	}

	h.logger.WithField("total_vms", result.Total).Info("Successfully retrieved VMs")

	c.JSON(http.StatusOK, response)
}

// GetVM godoc
// @Summary Get virtual machine details
// @Description Get detailed information about a specific virtual machine by name
// @Tags vms
// @Accept json
// @Produce json
// @Param name path string true "VM name" example("web-server-01")
// @Success 200 {object} types.VMDetailsResponse "Virtual machine details"
// @Failure 400 {object} types.ErrorResponse "Invalid request"
// @Failure 404 {object} types.ErrorResponse "VM not found"
// @Failure 500 {object} types.ErrorResponse "Internal server error"
// @Failure 503 {object} types.ErrorResponse "vSphere connection unavailable"
// @Router /api/v1/vms/{name} [get]
func (h *VMHandler) GetVM(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, types.ErrorResponse{
			Error:   "VM name is required",
			Code:    "MISSING_VM_NAME",
			Details: "VM name must be provided in the URL path",
		})
		return
	}

	h.logger.WithField("vm_name", name).Info("Getting VM details")

	result, err := h.vmService.GetVMByName(c.Request.Context(), name)
	if err != nil {
		h.logger.WithError(err).Error("Failed to get VM")

		if isConnectionError(err) {
			c.JSON(http.StatusServiceUnavailable, types.ErrorResponse{
				Error:   "vSphere connection unavailable",
				Code:    "VSPHERE_UNAVAILABLE",
				Details: "Unable to connect to vSphere. Please try again later.",
			})
			return
		}

		if isNotFoundError(err) {
			c.JSON(http.StatusNotFound, types.ErrorResponse{
				Error:   "VM not found",
				Code:    "VM_NOT_FOUND",
				Details: err.Error(),
			})
			return
		}

		c.JSON(http.StatusInternalServerError, types.ErrorResponse{
			Error:   "Failed to retrieve VM",
			Code:    "VM_GET_FAILED",
			Details: "An error occurred while retrieving the virtual machine",
		})
		return
	}

	// Convert detailed VM info to API response
	vm := types.VM{
		UUID:       result.VM.UUID,
		Name:       result.VM.Name,
		PowerState: result.VM.PowerState,
	}

	// Convert disks
	var disks []types.VMDisk
	for _, disk := range result.VM.Disks {
		disks = append(disks, types.VMDisk{
			Label:           disk.Label,
			CapacityKB:      disk.CapacityKB,
			CapacityGB:      disk.CapacityKB / 1024 / 1024,
			DiskPath:        disk.DiskPath,
			Datastore:       disk.Datastore,
			ThinProvisioned: disk.ThinProvisioned,
			DiskMode:        disk.DiskMode,
		})
	}

	// Convert network adapters
	var networkAdapters []types.VMNetworkAdapter
	for _, adapter := range result.VM.NetworkAdapters {
		networkAdapters = append(networkAdapters, types.VMNetworkAdapter{
			Label:       adapter.Label,
			NetworkName: adapter.NetworkName,
			MacAddress:  adapter.MacAddress,
			IPAddresses: adapter.IPAddresses,
			Connected:   adapter.Connected,
			AdapterType: adapter.AdapterType,
		})
	}

	// Convert snapshots
	var snapshots []types.VMSnapshot
	for _, snap := range result.VM.Snapshots {
		snapshots = append(snapshots, types.VMSnapshot{
			Name:        snap.Name,
			Description: snap.Description,
			CreateTime:  snap.CreateTime,
			State:       snap.State,
			Quiesced:    snap.Quiesced,
			ID:          snap.ID,
		})
	}

	// Build detailed response with all available information
	response := types.VMDetailsResponse{
		VM: vm,
		Hardware: types.VMHardwareInfo{
			NumCPU:            result.VM.NumCPU,
			NumCoresPerSocket: result.VM.NumCoresPerSocket,
			MemoryMB:          result.VM.MemoryMB,
			GuestFullName:     result.VM.GuestFullName,
			Version:           result.VM.Version,
			FirmwareType:      result.VM.FirmwareType,
		},
		Tools: types.VMToolsInfo{
			Status:        result.VM.ToolsStatus,
			Version:       result.VM.ToolsVersion,
			RunningStatus: result.VM.ToolsRunningStatus,
		},
		GuestInfo: types.VMGuestInfo{
			Hostname:             result.VM.Hostname,
			IPAddresses:          result.VM.IPAddresses,
			GuestID:              result.VM.GuestID,
			GuestState:           result.VM.GuestState,
			GuestHeartbeatStatus: result.VM.GuestHeartbeatStatus,
		},
		Metadata: types.VMMetadata{
			InstanceUUID: result.VM.InstanceUUID,
			BiosUUID:     result.VM.BiosUUID,
			Annotation:   result.VM.Annotation,
			Template:     result.VM.Template,
		},
		Runtime: types.VMRuntimeInfo{
			Host:                result.VM.Host,
			ConnectionState:     result.VM.ConnectionState,
			BootTime:            result.VM.BootTime,
			UptimeSeconds:       result.VM.UptimeSeconds,
			MaxCPUUsage:         result.VM.MaxCPUUsage,
			MaxMemoryUsage:      result.VM.MaxMemoryUsage,
			ConsolidationNeeded: result.VM.ConsolidationNeeded,
			FaultToleranceState: result.VM.FaultToleranceState,
		},
		Disks:           disks,
		NetworkAdapters: networkAdapters,
		Snapshots:       snapshots,
		CurrentSnapshot: result.VM.CurrentSnapshot,
		Resources: types.VMResourceInfo{
			CPUReservationMHz:   result.VM.ResourceAllocation.CPUReservation,
			CPULimitMHz:         result.VM.ResourceAllocation.CPULimit,
			CPUShares:           result.VM.ResourceAllocation.CPUShares,
			CPUSharesLevel:      result.VM.ResourceAllocation.CPUSharesLevel,
			MemoryReservationMB: result.VM.ResourceAllocation.MemoryReservation,
			MemoryLimitMB:       result.VM.ResourceAllocation.MemoryLimit,
			MemoryShares:        result.VM.ResourceAllocation.MemoryShares,
			MemorySharesLevel:   result.VM.ResourceAllocation.MemorySharesLevel,
		},
		Storage: types.VMStorageSummary{
			CommittedBytes:   result.VM.CommittedStorage,
			CommittedGB:      result.VM.CommittedStorage / 1024 / 1024 / 1024,
			UncommittedBytes: result.VM.UncommittedStorage,
			UncommittedGB:    result.VM.UncommittedStorage / 1024 / 1024 / 1024,
			Datastores:       result.VM.Datastores,
		},
		Files: types.VMFileInfo{
			VMPathName:  result.VM.VMPathName,
			ConfigFiles: result.VM.ConfigFiles,
			LogFiles:    result.VM.LogFiles,
		},
		Location: types.VMLocationInfo{
			Folder:       result.VM.Folder,
			ResourcePool: result.VM.ResourcePool,
		},
		Advanced: types.VMAdvancedInfo{
			CPUHotAddEnabled:      result.VM.CPUHotAddEnabled,
			CPUHotRemoveEnabled:   result.VM.CPUHotRemoveEnabled,
			MemoryHotAddEnabled:   result.VM.MemoryHotAddEnabled,
			ChangeTrackingEnabled: result.VM.ChangeTrackingEnabled,
		},
	}

	h.logger.WithFields(logrus.Fields{
		"vm_name": vm.Name,
		"vm_uuid": vm.UUID,
	}).Info("Successfully retrieved VM details")

	c.JSON(http.StatusOK, response)
}

// CreateClone godoc
// @Summary Create a clone from VM snapshot
// @Description Create a linked clone from a VM snapshot for inspection
// @Tags vms
// @Accept json
// @Produce json
// @Param name query string true "VM name" example("web-server-01")
// @Param request body types.CloneRequest true "Clone request"
// @Success 200 {object} types.CloneResponse "Clone created successfully"
// @Failure 400 {object} types.ErrorResponse "Invalid request"
// @Failure 404 {object} types.ErrorResponse "VM or snapshot not found"
// @Failure 500 {object} types.ErrorResponse "Internal server error"
// @Router /api/v1/vms/clone [post]
func (h *VMHandler) CreateClone(c *gin.Context) {
	vmName := c.Query("name")
	if vmName == "" {
		c.JSON(http.StatusBadRequest, types.ErrorResponse{
			Error:   "VM name is required",
			Code:    "MISSING_VM_NAME",
			Details: "Please provide VM name as query parameter: ?name=xxx",
		})
		return
	}

	var req types.CloneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.WithError(err).Error("Failed to bind clone request")
		c.JSON(http.StatusBadRequest, types.ErrorResponse{
			Error:   "Invalid request body",
			Code:    "INVALID_REQUEST",
			Details: err.Error(),
		})
		return
	}

	// Generate clone name if not provided
	cloneName := req.CloneName
	if cloneName == "" {
		cloneName = vmName + "-clone-" + time.Now().Format("20060102150405")
	}

	h.logger.WithFields(logrus.Fields{
		"vm_name":       vmName,
		"snapshot_name": req.SnapshotName,
		"clone_name":    cloneName,
	}).Info("Creating clone from snapshot")

	// Find snapshot
	snapshotRef, err := h.vmService.FindSnapshotByName(c.Request.Context(), vmName, req.SnapshotName)
	if err != nil {
		h.logger.WithError(err).Error("Failed to find snapshot")
		if isNotFoundError(err) {
			c.JSON(http.StatusNotFound, types.ErrorResponse{
				Error:   "Snapshot not found",
				Code:    "SNAPSHOT_NOT_FOUND",
				Details: err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, types.ErrorResponse{
			Error:   "Failed to find snapshot",
			Code:    "SNAPSHOT_FIND_FAILED",
			Details: err.Error(),
		})
		return
	}

	// Create clone
	err = h.vmService.CreateLinkedClone(c.Request.Context(), vmName, snapshotRef, cloneName)
	if err != nil {
		h.logger.WithError(err).Error("Failed to create clone")
		c.JSON(http.StatusInternalServerError, types.ErrorResponse{
			Error:   "Failed to create clone",
			Code:    "CLONE_CREATE_FAILED",
			Details: err.Error(),
		})
		return
	}

	response := types.CloneResponse{
		CloneName:    cloneName,
		VMName:       vmName,
		SnapshotName: req.SnapshotName,
		Status:       "completed",
		Message:      "Clone created successfully",
	}

	h.logger.WithFields(logrus.Fields{
		"clone_name": cloneName,
	}).Info("Clone created successfully")

	c.JSON(http.StatusOK, response)
}

// InspectSnapshot godoc
// @Summary Inspect a VM snapshot directly
// @Description Run virt-inspector or virt-v2v-inspector on a VM snapshot using VDDK
// @Tags vms
// @Accept json
// @Produce json
// @Param vm query string true "Original VM name" example("web-server-01")
// @Param snapshot query string true "Snapshot name" example("inspection-snapshot")
// @Param inspector query string false "Inspector type: 'virt-inspector' (default) or 'virt-v2v-inspector'" example("virt-inspector")
// @Success 200 {object} types.VMInspectionResponse "Inspection completed successfully"
// @Failure 400 {object} types.ErrorResponse "Invalid request"
// @Failure 404 {object} types.ErrorResponse "VM or snapshot not found"
// @Failure 500 {object} types.ErrorResponse "Internal server error"
// @Router /api/v1/vms/inspect-snapshot [post]
func (h *VMHandler) InspectSnapshot(c *gin.Context) {
	vmName := c.Query("vm")
	snapshotName := c.Query("snapshot")
	inspectorType := c.DefaultQuery("inspector", "virt-inspector") // Default to virt-inspector

	if vmName == "" {
		c.JSON(http.StatusBadRequest, types.ErrorResponse{
			Error:   "VM name is required",
			Code:    "MISSING_VM_NAME",
			Details: "Please provide VM name as query parameter: ?vm=xxx",
		})
		return
	}

	if snapshotName == "" {
		c.JSON(http.StatusBadRequest, types.ErrorResponse{
			Error:   "Snapshot name is required",
			Code:    "MISSING_SNAPSHOT_NAME",
			Details: "Please provide snapshot name as query parameter: &snapshot=xxx",
		})
		return
	}

	h.logger.WithFields(logrus.Fields{
		"vm_name":        vmName,
		"snapshot_name":  snapshotName,
		"inspector_type": inspectorType,
	}).Info("Inspecting VM snapshot with VDDK")

	// Validate inspector type
	if inspectorType != "virt-inspector" && inspectorType != "virt-v2v-inspector" {
		c.JSON(http.StatusBadRequest, types.ErrorResponse{
			Error:   "Invalid inspector type",
			Code:    "INVALID_INSPECTOR_TYPE",
			Details: fmt.Sprintf("inspector must be 'virt-inspector' or 'virt-v2v-inspector', got: %s", inspectorType),
		})
		return
	}

	// Get vCenter credentials from config
	vcenterURL := h.vmClient.GetConfig().VCenterURL
	username := h.vmClient.GetConfig().Username
	password := h.vmClient.GetConfig().Password
	
	// SSL verification option for vpx:// URL
	// Using no_verify=1 for now to simplify (can be enhanced later with certificate support)
	sslVerify := "no_verify=1"

	datacenter, err := h.vmService.GetDatacenterName(c.Request.Context(), vmName)
	if err != nil {
		h.logger.WithError(err).Error("failed to get datacenter name")
		c.JSON(http.StatusInternalServerError, types.ErrorResponse{
			Error:   "Inspection failed",
			Code:    "INSPECTION_FAILED",
			Details: err.Error(),
		})
		return
	}

	// Get snapshot disk info (morefs and disk path) from vm_service
	h.logger.Debug("Getting snapshot disk info from vm_service")
	diskInfo, err := h.vmService.GetSnapshotDiskInfo(c.Request.Context(), vmName, snapshotName)
	if err != nil {
		h.logger.WithError(err).Error("failed to get snapshot disk info")
		c.JSON(http.StatusInternalServerError, types.ErrorResponse{
			Error:   "Inspection failed",
			Code:    "INSPECTION_FAILED",
			Details: fmt.Sprintf("failed to get snapshot disk info: %v", err),
		})
		return
	}

	// Use the selected inspector to inspect snapshot
	var response types.VMInspectionResponse
	message := fmt.Sprintf("Snapshot inspection completed successfully using %s", inspectorType)

	if inspectorType == "virt-v2v-inspector" {
		inspector := inspection.NewVirtV2vInspector("", 30*time.Minute, h.logger)
		h.logger.Info("Running virt-v2v-inspector with VDDK on snapshot")
		inspectionData, err := inspector.Inspect(
			c.Request.Context(),
			vmName,
			snapshotName,
			vcenterURL,
			datacenter,
			username,
			password,
			diskInfo,
			sslVerify,
		)
		if err != nil {
			h.logger.WithError(err).WithField("inspector_type", inspectorType).Error("inspection execution failed")
			c.JSON(http.StatusInternalServerError, types.ErrorResponse{
				Error:   "Inspection failed",
				Code:    "INSPECTION_FAILED",
				Details: err.Error(),
			})
			return
		}
		response = types.NewVirtV2VInspectorResponse(vmName, snapshotName, message, inspectionData)
	} else {
		// Default: use virt-inspector
		inspector := inspection.NewVirtInspector("", 30*time.Minute, h.logger)
		h.logger.Info("Running virt-inspector with VDDK on snapshot")
		inspectionData, err := inspector.Inspect(
			c.Request.Context(),
			vmName,
			snapshotName,
			vcenterURL,
			datacenter,
			username,
			password,
			diskInfo,
		)
		if err != nil {
			h.logger.WithError(err).WithField("inspector_type", inspectorType).Error("inspection execution failed")
			c.JSON(http.StatusInternalServerError, types.ErrorResponse{
				Error:   "Inspection failed",
				Code:    "INSPECTION_FAILED",
				Details: err.Error(),
			})
			return
		}
		response = types.NewVirtInspectorResponse(vmName, snapshotName, message, inspectionData)
	}

	h.logger.WithField("inspector_type", inspectorType).Info("Snapshot inspection completed successfully")
	c.JSON(http.StatusOK, response)
}

// DeleteClone godoc
// @Summary Delete a cloned VM
// @Description Delete a cloned VM created for inspection
// @Tags vms
// @Accept json
// @Produce json
// @Param name query string true "Clone VM name" example("web-server-01-clone-123")
// @Success 200 {object} types.ErrorResponse "Clone deleted successfully"
// @Failure 400 {object} types.ErrorResponse "Invalid request"
// @Failure 404 {object} types.ErrorResponse "Clone not found"
// @Failure 500 {object} types.ErrorResponse "Internal server error"
// @Router /api/v1/vms/delete-clone [delete]
func (h *VMHandler) DeleteClone(c *gin.Context) {
	cloneName := c.Query("name")
	if cloneName == "" {
		c.JSON(http.StatusBadRequest, types.ErrorResponse{
			Error:   "Clone name is required",
			Code:    "MISSING_CLONE_NAME",
			Details: "Please provide clone name as query parameter: ?name=xxx",
		})
		return
	}

	h.logger.WithField("clone_name", cloneName).Info("Deleting clone")

	err := h.vmService.DeleteVM(c.Request.Context(), cloneName)
	if err != nil {
		h.logger.WithError(err).Error("Failed to delete clone")
		if isNotFoundError(err) {
			c.JSON(http.StatusNotFound, types.ErrorResponse{
				Error:   "Clone not found",
				Code:    "CLONE_NOT_FOUND",
				Details: err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, types.ErrorResponse{
			Error:   "Failed to delete clone",
			Code:    "CLONE_DELETE_FAILED",
			Details: err.Error(),
		})
		return
	}

	h.logger.Info("Clone deleted successfully")
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Clone deleted successfully",
	})
}

// CreateVMSnapshot godoc
// @Summary Create a VM snapshot
// @Description Create a snapshot for a specific virtual machine
// @Tags vms
// @Accept json
// @Produce json
// @Param name query string true "VM name" example("web-server-01")
// @Param request body types.SnapshotCreateRequest true "Snapshot creation request"
// @Success 200 {object} types.SnapshotCreateResponse "Snapshot created successfully"
// @Failure 400 {object} types.ErrorResponse "Invalid request"
// @Failure 404 {object} types.ErrorResponse "VM not found"
// @Failure 500 {object} types.ErrorResponse "Internal server error"
// @Failure 503 {object} types.ErrorResponse "vSphere connection unavailable"
// @Router /api/v1/vms/snapshot [post]
func (h *VMHandler) CreateVMSnapshot(c *gin.Context) {
	// Get VM name from query parameter
	vmName := c.Query("name")
	if vmName == "" {
		c.JSON(http.StatusBadRequest, types.ErrorResponse{
			Error:   "VM name is required",
			Code:    "MISSING_VM_NAME",
			Details: "Please provide VM name as query parameter: ?name=xxx",
		})
		return
	}

	// Parse request body
	var req types.SnapshotCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.WithError(err).Error("Failed to bind snapshot request")
		c.JSON(http.StatusBadRequest, types.ErrorResponse{
			Error:   "Invalid request body",
			Code:    "INVALID_REQUEST",
			Details: err.Error(),
		})
		return
	}

	h.logger.WithFields(logrus.Fields{
		"vm_name":       vmName,
		"snapshot_name": req.Name,
		"memory":        req.Memory,
		"quiesce":       req.Quiesce,
	}).Info("Creating VM snapshot")

	// Create snapshot
	snapshotID, err := h.vmService.CreateSnapshot(
		c.Request.Context(),
		vmName,
		req.Name,
		req.Description,
		req.Memory,
		req.Quiesce,
	)

	if err != nil {
		h.logger.WithError(err).Error("Failed to create snapshot")

		if isConnectionError(err) {
			c.JSON(http.StatusServiceUnavailable, types.ErrorResponse{
				Error:   "vSphere connection unavailable",
				Code:    "VSPHERE_UNAVAILABLE",
				Details: "Unable to connect to vSphere. Please try again later.",
			})
			return
		}

		if isNotFoundError(err) {
			c.JSON(http.StatusNotFound, types.ErrorResponse{
				Error:   "VM not found",
				Code:    "VM_NOT_FOUND",
				Details: err.Error(),
			})
			return
		}

		c.JSON(http.StatusInternalServerError, types.ErrorResponse{
			Error:   "Failed to create snapshot",
			Code:    "SNAPSHOT_CREATE_FAILED",
			Details: err.Error(),
		})
		return
	}

	response := types.SnapshotCreateResponse{
		SnapshotID: snapshotID,
		Name:       req.Name,
		VMID:       "",
		VMName:     vmName,
		Status:     "completed",
		Message:    "Snapshot created successfully",
	}

	h.logger.WithFields(logrus.Fields{
		"snapshot_id": snapshotID,
		"vm_name":     vmName,
	}).Info("Snapshot created successfully")

	c.JSON(http.StatusOK, response)
}

// convertVMInfoToVM converts internal VMInfo to API VM type
func (h *VMHandler) convertVMInfoToVM(vmInfo vmware.VMInfo) types.VM {
	return types.VM{
		UUID:       vmInfo.UUID,
		Name:       vmInfo.Name,
		PowerState: vmInfo.PowerState,
	}
}

// Helper functions to determine error types
func isConnectionError(err error) bool {
	// Check for common connection-related errors
	errStr := err.Error()
	return contains(errStr, "connection") ||
		   contains(errStr, "timeout") ||
		   contains(errStr, "network") ||
		   contains(errStr, "dial")
}

func isAuthenticationError(err error) bool {
	// Check for authentication-related errors
	errStr := err.Error()
	return contains(errStr, "authentication") ||
		   contains(errStr, "login") ||
		   contains(errStr, "unauthorized") ||
		   contains(errStr, "permission")
}

func isNotFoundError(err error) bool {
	// Check for not found errors
	errStr := err.Error()
	return contains(errStr, "not found") ||
		   contains(errStr, "does not exist")
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		   (s == substr ||
			len(s) > len(substr) &&
			(hasSubstring(s, substr)))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if toLower(s[i+j]) != toLower(substr[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func toLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}
