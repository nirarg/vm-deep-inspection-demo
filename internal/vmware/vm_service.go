package vmware

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// VMService provides VM discovery and management functionality
type VMService struct {
	client *Client
	logger *logrus.Logger
}

// VMFilter contains filtering options for VM discovery
type VMFilter struct {
	Datacenter  string `json:"datacenter,omitempty"`
	Cluster     string `json:"cluster,omitempty"`
	PowerState  string `json:"power_state,omitempty"`
	Name        string `json:"name,omitempty"`
	GuestOS     string `json:"guest_os,omitempty"`
	Limit       int    `json:"limit,omitempty"`
	Offset      int    `json:"offset,omitempty"`
}

// VMInfo represents basic information about a virtual machine
type VMInfo struct {
	UUID       string `json:"uuid"`
	Name       string `json:"name"`
	PowerState string `json:"power_state"`
}

// VMDetailedInfo represents comprehensive information about a virtual machine
type VMDetailedInfo struct {
	UUID              string   `json:"uuid"`
	Name              string   `json:"name"`
	PowerState        string   `json:"power_state"`
	GuestFullName     string   `json:"guest_full_name"`
	GuestID           string   `json:"guest_id"`
	NumCPU            int32    `json:"num_cpu"`
	NumCoresPerSocket int32    `json:"num_cores_per_socket"`
	MemoryMB          int32    `json:"memory_mb"`
	ToolsStatus       string   `json:"tools_status"`
	ToolsVersion      string   `json:"tools_version"`
	ToolsRunningStatus string  `json:"tools_running_status"`
	IPAddresses       []string `json:"ip_addresses"`
	Hostname          string   `json:"hostname"`
	Version           string   `json:"version"`
	InstanceUUID      string   `json:"instance_uuid"`
	BiosUUID          string   `json:"bios_uuid"`
	Annotation        string   `json:"annotation"`
	FirmwareType      string   `json:"firmware_type"`
}

// VMResult represents a single VM result
type VMResult struct {
	Datacenter string `json:"datacenter"`
	VM         VMInfo `json:"vm"`
}

// VMDetailedResult represents a detailed VM result
type VMDetailedResult struct {
	Datacenter string         `json:"datacenter"`
	VM         VMDetailedInfo `json:"vm"`
}

// VMListResult represents the result of VM listing
type VMListResult struct {
	Datacenter string   `json:"datacenter"`
	VMs        []VMInfo `json:"vms"`
	Total      int      `json:"total"`
}

// NewVMService creates a new VM service instance
func NewVMService(client *Client, logger *logrus.Logger) *VMService {
	return &VMService{
		client: client,
		logger: logger,
	}
}

// getDefaultDatacenter is a helper to get the default datacenter
func (s *VMService) getDefaultDatacenter(ctx context.Context, finder *find.Finder) (*object.Datacenter, error) {
	datacenter, err := finder.DefaultDatacenter(ctx)
	if err != nil {
		return nil, fmt.Errorf("no default datacenter found: %w", err)
	}
	finder.SetDatacenter(datacenter)
	return datacenter, nil
}

// findVMByName is a helper to find a VM by name
func (s *VMService) findVMByName(ctx context.Context, name string) (*object.VirtualMachine, *object.Datacenter, error) {
	// Get govmomi client
	client, err := s.client.GetClient(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get vSphere client: %w", err)
	}

	// Create finder
	finder := find.NewFinder(client.Client, true)

	// Get default datacenter
	datacenter, err := s.getDefaultDatacenter(ctx, finder)
	if err != nil {
		return nil, nil, err
	}

	// Find VM by name
	vm, err := finder.VirtualMachine(ctx, name)
	if err != nil {
		return nil, nil, fmt.Errorf("VM with name '%s' not found: %w", name, err)
	}

	return vm, datacenter, nil
}

// GetVMByName retrieves a single VM by its name with full details
func (s *VMService) GetVMByName(ctx context.Context, name string) (*VMDetailedResult, error) {
	s.logger.WithField("name", name).Info("Getting VM by name")

	// Find VM by name
	vm, datacenter, err := s.findVMByName(ctx, name)
	if err != nil {
		return nil, err
	}

	// Get govmomi client for property collector
	client, err := s.client.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get vSphere client: %w", err)
	}

	// Retrieve VM properties with comprehensive details
	var vmProp mo.VirtualMachine
	pc := property.DefaultCollector(client.Client)
	err = pc.RetrieveOne(ctx, vm.Reference(), []string{
		"name",
		"config.uuid",
		"config.instanceUuid",
		"config.guestFullName",
		"config.guestId",
		"config.hardware.numCPU",
		"config.hardware.numCoresPerSocket",
		"config.hardware.memoryMB",
		"config.version",
		"config.firmware",
		"config.annotation",
		"runtime.powerState",
		"guest.toolsStatus",
		"guest.toolsVersion",
		"guest.toolsRunningStatus",
		"guest.ipAddress",
		"guest.hostName",
		"guest.net",
	}, &vmProp)

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve VM properties: %w", err)
	}

	// Convert to VMDetailedInfo
	vmInfo := s.convertToVMDetailedInfo(vmProp)

	s.logger.Info("VM retrieval completed")

	return &VMDetailedResult{
		Datacenter: datacenter.Name(),
		VM:         *vmInfo,
	}, nil
}

// GetVMByUUID retrieves a single VM by its UUID
func (s *VMService) GetVMByUUID(ctx context.Context, uuid string) (*VMResult, error) {
	s.logger.WithField("uuid", uuid).Info("Getting VM by UUID")

	// Get govmomi client
	client, err := s.client.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get vSphere client: %w", err)
	}

	// Create finder for object discovery
	finder := find.NewFinder(client.Client, true)

	// Get default datacenter
	datacenter, err := finder.DefaultDatacenter(ctx)
	if err != nil {
		return nil, fmt.Errorf("no default datacenter found: %w", err)
	}
	finder.SetDatacenter(datacenter)

	// Use SearchIndex to find VM by UUID (fastest method)
	searchIndex := object.NewSearchIndex(client.Client)
	vmRef, err := searchIndex.FindByUuid(ctx, datacenter, uuid, true, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to search for VM with UUID '%s': %w", uuid, err)
	}
	if vmRef == nil {
		return nil, fmt.Errorf("VM with UUID '%s' not found", uuid)
	}

	// Retrieve VM properties
	var vmProp mo.VirtualMachine
	pc := property.DefaultCollector(client.Client)
	err = pc.RetrieveOne(ctx, vmRef.Reference(), []string{
		"name",
		"config.uuid",
		"runtime.powerState",
	}, &vmProp)

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve VM properties: %w", err)
	}

	// Convert to VMInfo
	vmInfo := s.convertToVMInfo(vmProp)

	s.logger.Info("VM retrieval completed")

	return &VMResult{
		Datacenter: datacenter.Name(),
		VM:         *vmInfo,
	}, nil
}

// ListVMs retrieves all virtual machines with optional filtering
func (s *VMService) ListVMs(ctx context.Context, filter VMFilter) (*VMListResult, error) {
	s.logger.WithFields(logrus.Fields{
		"filter": filter,
	}).Info("Starting VM discovery")

	// Get govmomi client
	client, err := s.client.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get vSphere client: %w", err)
	}

	// Create finder for object discovery
	finder := find.NewFinder(client.Client, true)

	// Set datacenter if specified in filter
	var datacenter *object.Datacenter
	if filter.Datacenter != "" {
		datacenter, err = finder.Datacenter(ctx, filter.Datacenter)
		if err != nil {
			return nil, fmt.Errorf("datacenter '%s' not found: %w", filter.Datacenter, err)
		}
		finder.SetDatacenter(datacenter)
	} else {
		// If no datacenter specified, use default (first one found)
		datacenter, err = finder.DefaultDatacenter(ctx)
		if err != nil {
			return nil, fmt.Errorf("no default datacenter found: %w", err)
		}
		finder.SetDatacenter(datacenter)
	}

	// Find all VMs or filter by cluster
	var vms []*object.VirtualMachine
	if filter.Cluster != "" {
		// Find VMs in specific cluster
		cluster, err := finder.ClusterComputeResource(ctx, filter.Cluster)
		if err != nil {
			return nil, fmt.Errorf("cluster '%s' not found: %w", filter.Cluster, err)
		}

		vms, err = finder.VirtualMachineList(ctx, cluster.InventoryPath+"/*")
		if err != nil {
			return nil, fmt.Errorf("failed to list VMs in cluster '%s': %w", filter.Cluster, err)
		}
	} else {
		// Find all VMs in datacenter
		vms, err = finder.VirtualMachineList(ctx, "*")
		if err != nil {
			return nil, fmt.Errorf("failed to list VMs: %w", err)
		}
	}

	s.logger.WithField("vm_count", len(vms)).Info("Found VMs in vSphere")

	// Collect VM managed object references
	var vmRefs []types.ManagedObjectReference
	for _, vm := range vms {
		vmRefs = append(vmRefs, vm.Reference())
	}

	if len(vmRefs) == 0 {
		return &VMListResult{
			Datacenter: datacenter.Name(),
			VMs:        []VMInfo{},
			Total:      0,
		}, nil
	}

	// Define properties to retrieve for all VMs
	var vmProperties []mo.VirtualMachine
	pc := property.DefaultCollector(client.Client)

	err = pc.Retrieve(ctx, vmRefs, []string{
		"name",
		"config.uuid",
		"runtime.powerState",
	}, &vmProperties)

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve VM properties: %w", err)
	}

	// Convert all VMs
	var vmInfos []VMInfo
	for _, vmProp := range vmProperties {
		vmInfo := s.convertToVMInfo(vmProp)
		vmInfos = append(vmInfos, *vmInfo)
	}

	s.logger.WithField("total_vms", len(vmInfos)).Info("VM discovery completed")

	return &VMListResult{
		Datacenter: datacenter.Name(),
		VMs:        vmInfos,
		Total:      len(vmInfos),
	}, nil
}

// convertToVMInfo converts a vSphere VM managed object to VMInfo
func (s *VMService) convertToVMInfo(vm mo.VirtualMachine) *VMInfo {
	return &VMInfo{
		UUID:       vm.Config.Uuid,
		Name:       vm.Name,
		PowerState: string(vm.Runtime.PowerState),
	}
}

// convertToVMDetailedInfo converts a vSphere VM managed object to VMDetailedInfo
func (s *VMService) convertToVMDetailedInfo(vm mo.VirtualMachine) *VMDetailedInfo {
	info := &VMDetailedInfo{
		UUID:       vm.Config.Uuid,
		Name:       vm.Name,
		PowerState: string(vm.Runtime.PowerState),
	}

	// Config properties
	if vm.Config != nil {
		info.InstanceUUID = vm.Config.InstanceUuid
		info.GuestFullName = vm.Config.GuestFullName
		info.GuestID = vm.Config.GuestId
		info.Version = vm.Config.Version
		info.Annotation = vm.Config.Annotation
		info.FirmwareType = vm.Config.Firmware

		// Hardware properties
		if vm.Config.Hardware.NumCPU > 0 {
			info.NumCPU = vm.Config.Hardware.NumCPU
		}
		if vm.Config.Hardware.NumCoresPerSocket > 0 {
			info.NumCoresPerSocket = vm.Config.Hardware.NumCoresPerSocket
		}
		if vm.Config.Hardware.MemoryMB > 0 {
			info.MemoryMB = vm.Config.Hardware.MemoryMB
		}
	}

	// Guest properties
	if vm.Guest != nil {
		info.ToolsStatus = string(vm.Guest.ToolsStatus)
		info.ToolsVersion = vm.Guest.ToolsVersion
		info.ToolsRunningStatus = vm.Guest.ToolsRunningStatus
		info.Hostname = vm.Guest.HostName

		// Collect all IP addresses from guest NICs
		var ipAddresses []string
		if vm.Guest.IpAddress != "" {
			ipAddresses = append(ipAddresses, vm.Guest.IpAddress)
		}
		for _, nic := range vm.Guest.Net {
			if nic.IpConfig != nil {
				for _, ipConfig := range nic.IpConfig.IpAddress {
					ip := ipConfig.IpAddress
					// Skip if already in list
					found := false
					for _, existing := range ipAddresses {
						if existing == ip {
							found = true
							break
						}
					}
					if !found && ip != "" {
						ipAddresses = append(ipAddresses, ip)
					}
				}
			}
		}
		info.IPAddresses = ipAddresses
	}

	// Get BIOS UUID (same as UUID in most cases)
	info.BiosUUID = vm.Config.Uuid

	return info
}

// FindSnapshotByName finds a snapshot by name on a VM
func (s *VMService) FindSnapshotByName(ctx context.Context, vmName string, snapshotName string) (*types.ManagedObjectReference, error) {
	s.logger.WithFields(logrus.Fields{
		"vm_name":       vmName,
		"snapshot_name": snapshotName,
	}).Info("Finding snapshot by name")

	// Find VM by name
	vm, _, err := s.findVMByName(ctx, vmName)
	if err != nil {
		return nil, err
	}

	// Get snapshot tree
	var vmProps mo.VirtualMachine
	client, err := s.client.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get vSphere client: %w", err)
	}

	pc := property.DefaultCollector(client.Client)
	err = pc.RetrieveOne(ctx, vm.Reference(), []string{"snapshot"}, &vmProps)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve VM snapshots: %w", err)
	}

	if vmProps.Snapshot == nil {
		return nil, fmt.Errorf("VM '%s' has no snapshots", vmName)
	}

	// Search for snapshot by name
	var findSnapshot func(tree []types.VirtualMachineSnapshotTree) *types.ManagedObjectReference
	findSnapshot = func(tree []types.VirtualMachineSnapshotTree) *types.ManagedObjectReference {
		for _, node := range tree {
			if node.Name == snapshotName {
				return &node.Snapshot
			}
			if len(node.ChildSnapshotList) > 0 {
				if ref := findSnapshot(node.ChildSnapshotList); ref != nil {
					return ref
				}
			}
		}
		return nil
	}

	snapshotRef := findSnapshot(vmProps.Snapshot.RootSnapshotList)
	if snapshotRef == nil {
		return nil, fmt.Errorf("snapshot '%s' not found on VM '%s'", snapshotName, vmName)
	}

	s.logger.Info("Snapshot found successfully")
	return snapshotRef, nil
}

// CreateLinkedClone creates a linked clone from a snapshot
func (s *VMService) CreateLinkedClone(ctx context.Context, vmName string, snapshotRef *types.ManagedObjectReference, cloneName string) error {
	s.logger.WithFields(logrus.Fields{
		"vm_name":    vmName,
		"clone_name": cloneName,
	}).Info("Creating linked clone from snapshot")

	// Find source VM
	vm, datacenter, err := s.findVMByName(ctx, vmName)
	if err != nil {
		return err
	}

	// Get govmomi client
	client, err := s.client.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to get vSphere client: %w", err)
	}

	// Get VM folder
	finder := find.NewFinder(client.Client, true)
	finder.SetDatacenter(datacenter)

	vmFolder, err := finder.FolderOrDefault(ctx, "vm")
	if err != nil {
		return fmt.Errorf("failed to find VM folder: %w", err)
	}

	// Create linked clone spec
	cloneSpec := types.VirtualMachineCloneSpec{
		Location: types.VirtualMachineRelocateSpec{
			DiskMoveType: string(types.VirtualMachineRelocateDiskMoveOptionsCreateNewChildDiskBacking),
		},
		Snapshot: snapshotRef,
		PowerOn:  false,
		Template: false,
	}

	// Create clone task
	task, err := vm.Clone(ctx, vmFolder, cloneName, cloneSpec)
	if err != nil {
		return fmt.Errorf("failed to create clone task: %w", err)
	}

	s.logger.WithField("task_id", task.Reference().Value).Info("Clone task created, waiting for completion")

	// Wait for task to complete
	err = task.Wait(ctx)
	if err != nil {
		return fmt.Errorf("clone creation failed: %w", err)
	}

	s.logger.Info("Linked clone created successfully")
	return nil
}

// DeleteVM deletes a VM
func (s *VMService) DeleteVM(ctx context.Context, vmName string) error {
	s.logger.WithField("vm_name", vmName).Info("Deleting VM")

	// Find VM
	vm, _, err := s.findVMByName(ctx, vmName)
	if err != nil {
		return err
	}

	// Destroy VM task
	task, err := vm.Destroy(ctx)
	if err != nil {
		return fmt.Errorf("failed to create delete task: %w", err)
	}

	s.logger.WithField("task_id", task.Reference().Value).Info("Delete task created, waiting for completion")

	// Wait for task to complete
	err = task.Wait(ctx)
	if err != nil {
		return fmt.Errorf("VM deletion failed: %w", err)
	}

	s.logger.Info("VM deleted successfully")
	return nil
}

// CreateSnapshot creates a snapshot for a VM
func (s *VMService) CreateSnapshot(ctx context.Context, vmName string, snapshotName string, description string, memory bool, quiesce bool) (string, error) {
	s.logger.WithFields(logrus.Fields{
		"vm_name":       vmName,
		"snapshot_name": snapshotName,
		"memory":        memory,
		"quiesce":       quiesce,
	}).Info("Creating VM snapshot")

	// Find VM by name using the helper function
	vm, _, err := s.findVMByName(ctx, vmName)
	if err != nil {
		return "", err
	}

	// Create snapshot task
	task, err := vm.CreateSnapshot(ctx, snapshotName, description, memory, quiesce)
	if err != nil {
		return "", fmt.Errorf("failed to create snapshot task: %w", err)
	}

	s.logger.WithField("task_id", task.Reference().Value).Info("Snapshot task created, waiting for completion")

	// Wait for task to complete
	err = task.Wait(ctx)
	if err != nil {
		return "", fmt.Errorf("snapshot creation failed: %w", err)
	}

	s.logger.Info("Snapshot created successfully")

	// Return the task reference as snapshot ID
	return task.Reference().Value, nil
}

// InspectVMFromSnapshot inspects a VM by creating a temporary clone from a snapshot
func (s *VMService) InspectVMFromSnapshot(ctx context.Context, vmName string, snapshotName string, inspector interface{}) error {
	// Generate unique clone name
	cloneName := fmt.Sprintf("%s-inspect-clone-%d", vmName, time.Now().Unix())

	s.logger.WithFields(logrus.Fields{
		"vm_name":       vmName,
		"snapshot_name": snapshotName,
		"clone_name":    cloneName,
	}).Info("Starting VM inspection from snapshot")

	// Find snapshot
	snapshotRef, err := s.FindSnapshotByName(ctx, vmName, snapshotName)
	if err != nil {
		return fmt.Errorf("failed to find snapshot: %w", err)
	}

	// Create linked clone
	err = s.CreateLinkedClone(ctx, vmName, snapshotRef, cloneName)
	if err != nil {
		return fmt.Errorf("failed to create linked clone: %w", err)
	}

	// Ensure cleanup of clone
	defer func() {
		s.logger.Info("Cleaning up inspection clone")
		cleanupErr := s.DeleteVM(ctx, cloneName)
		if cleanupErr != nil {
			s.logger.WithError(cleanupErr).Error("Failed to delete inspection clone")
		}
	}()

	// Note: The actual virt-inspector execution will be handled by the API handler
	// This method just manages the clone lifecycle

	s.logger.Info("Inspection clone ready for inspection")
	return nil
}

// matchesFilter checks if a VM matches the given filter criteria
func (s *VMService) matchesFilter(vm VMInfo, filter VMFilter) bool {
	if filter.PowerState != "" && !strings.EqualFold(vm.PowerState, filter.PowerState) {
		return false
	}

	if filter.Name != "" && !strings.Contains(strings.ToLower(vm.Name), strings.ToLower(filter.Name)) {
		return false
	}

	// GuestOS filtering not supported with minimal properties
	// Cluster filtering not supported with minimal properties

	return true
}