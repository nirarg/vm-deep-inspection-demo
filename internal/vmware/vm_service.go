package vmware

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nirarg/v2v-vm-validations/pkg/types"
	"github.com/sirupsen/logrus"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
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

// VMDiskInfo represents virtual disk information
type VMDiskInfo struct {
	Label            string `json:"label"`
	CapacityKB       int64  `json:"capacity_kb"`
	DiskPath         string `json:"disk_path"`
	Datastore        string `json:"datastore"`
	ThinProvisioned  bool   `json:"thin_provisioned"`
	DiskMode         string `json:"disk_mode"`
	ControllerKey    int32  `json:"controller_key"`
}

// VMNetworkAdapterInfo represents network adapter information
type VMNetworkAdapterInfo struct {
	Label          string   `json:"label"`
	NetworkName    string   `json:"network_name"`
	MacAddress     string   `json:"mac_address"`
	IPAddresses    []string `json:"ip_addresses"`
	Connected      bool     `json:"connected"`
	AdapterType    string   `json:"adapter_type"`
}

// VMSnapshotInfo represents snapshot information
type VMSnapshotInfo struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreateTime  time.Time `json:"create_time"`
	State       string    `json:"state"`
	Quiesced    bool      `json:"quiesced"`
	ID          int32     `json:"id"`
}

// VMResourceAllocation represents resource allocation settings
type VMResourceAllocation struct {
	CPUReservation    int64  `json:"cpu_reservation_mhz"`
	CPULimit          int64  `json:"cpu_limit_mhz"`
	CPUShares         int32  `json:"cpu_shares"`
	CPUSharesLevel    string `json:"cpu_shares_level"`
	MemoryReservation int64  `json:"memory_reservation_mb"`
	MemoryLimit       int64  `json:"memory_limit_mb"`
	MemoryShares      int32  `json:"memory_shares"`
	MemorySharesLevel string `json:"memory_shares_level"`
}

// VMDetailedInfo represents comprehensive information about a virtual machine
type VMDetailedInfo struct {
	// Basic Info
	UUID              string   `json:"uuid"`
	Name              string   `json:"name"`
	PowerState        string   `json:"power_state"`
	GuestFullName     string   `json:"guest_full_name"`
	GuestID           string   `json:"guest_id"`
	InstanceUUID      string   `json:"instance_uuid"`
	BiosUUID          string   `json:"bios_uuid"`
	Annotation        string   `json:"annotation"`

	// Hardware
	NumCPU            int32    `json:"num_cpu"`
	NumCoresPerSocket int32    `json:"num_cores_per_socket"`
	MemoryMB          int32    `json:"memory_mb"`
	Version           string   `json:"version"`
	FirmwareType      string   `json:"firmware_type"`
	CPUHotAddEnabled  bool     `json:"cpu_hot_add_enabled"`
	CPUHotRemoveEnabled bool   `json:"cpu_hot_remove_enabled"`
	MemoryHotAddEnabled bool   `json:"memory_hot_add_enabled"`

	// Guest Info
	ToolsStatus        string   `json:"tools_status"`
	ToolsVersion       string   `json:"tools_version"`
	ToolsRunningStatus string   `json:"tools_running_status"`
	IPAddresses        []string `json:"ip_addresses"`
	Hostname           string   `json:"hostname"`
	GuestState         string   `json:"guest_state"`

	// Runtime Info
	Host              string    `json:"host"`
	ConnectionState   string    `json:"connection_state"`
	BootTime          time.Time `json:"boot_time,omitempty"`
	UptimeSeconds     int64     `json:"uptime_seconds"`
	MaxCPUUsage       int32     `json:"max_cpu_usage_mhz"`
	MaxMemoryUsage    int32     `json:"max_memory_usage_mb"`
	ConsolidationNeeded bool    `json:"consolidation_needed"`

	// Storage
	Disks             []VMDiskInfo `json:"disks"`
	Datastores        []string     `json:"datastores"`
	CommittedStorage  int64        `json:"committed_storage_bytes"`
	UncommittedStorage int64       `json:"uncommitted_storage_bytes"`

	// Network
	NetworkAdapters   []VMNetworkAdapterInfo `json:"network_adapters"`

	// Resource Allocation
	ResourceAllocation VMResourceAllocation `json:"resource_allocation"`

	// Location
	Folder            string `json:"folder"`
	ResourcePool      string `json:"resource_pool"`

	// Snapshots
	Snapshots         []VMSnapshotInfo `json:"snapshots"`
	CurrentSnapshot   string           `json:"current_snapshot"`

	// Files
	VMPathName        string   `json:"vm_path_name"`
	ConfigFiles       []string `json:"config_files"`
	LogFiles          []string `json:"log_files"`

	// Advanced
	Template          bool              `json:"template"`
	ChangeTrackingEnabled bool          `json:"change_tracking_enabled"`
	FaultToleranceState string          `json:"fault_tolerance_state"`
	GuestHeartbeatStatus string         `json:"guest_heartbeat_status"`
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
	s.logger.Debug("Attempting to get default datacenter")
	datacenter, err := finder.DefaultDatacenter(ctx)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get default datacenter - this may indicate authentication or permission issues")
		return nil, fmt.Errorf("no default datacenter found: %w", err)
	}
	s.logger.WithField("datacenter", datacenter.Name()).Info("Found default datacenter")
	finder.SetDatacenter(datacenter)
	return datacenter, nil
}

// GetDatacenterName returns the datacenter name for a given VM
func (s *VMService) GetDatacenterName(ctx context.Context, vmName string) (string, error) {
	_, datacenter, err := s.findVMByName(ctx, vmName)
	if err != nil {
		return "", err
	}
	return datacenter.Name(), nil
}

// findVMByName is a helper to find a VM by name
func (s *VMService) findVMByName(ctx context.Context, name string) (*object.VirtualMachine, *object.Datacenter, error) {
	s.logger.WithField("vm_name", name).Debug("Finding VM by name")

	// Get govmomi client
	client, err := s.client.GetClient(ctx)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get vSphere client")
		return nil, nil, fmt.Errorf("failed to get vSphere client: %w", err)
	}
	s.logger.Debug("Successfully obtained vSphere client")

	// Create finder
	finder := find.NewFinder(client.Client, true)

	// Get default datacenter
	datacenter, err := s.getDefaultDatacenter(ctx, finder)
	if err != nil {
		return nil, nil, err
	}

	// Find VM by name
	s.logger.WithField("vm_name", name).Debug("Searching for VM in datacenter")
	vm, err := finder.VirtualMachine(ctx, name)
	if err != nil {
		s.logger.WithError(err).WithField("vm_name", name).Error("VM not found")
		return nil, nil, fmt.Errorf("VM with name '%s' not found: %w", name, err)
	}
	s.logger.WithField("vm_name", name).Info("Successfully found VM")

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
		// Basic
		"name",
		"config.uuid",
		"config.instanceUuid",
		"config.guestFullName",
		"config.guestId",
		"config.annotation",
		"config.template",

		// Hardware
		"config.hardware.numCPU",
		"config.hardware.numCoresPerSocket",
		"config.hardware.memoryMB",
		"config.hardware.device",
		"config.version",
		"config.firmware",
		"config.cpuHotAddEnabled",
		"config.cpuHotRemoveEnabled",
		"config.memoryHotAddEnabled",
		"config.changeTrackingEnabled",

		// Runtime
		"runtime.powerState",
		"runtime.host",
		"runtime.connectionState",
		"runtime.bootTime",
		"runtime.maxCpuUsage",
		"runtime.maxMemoryUsage",
		"runtime.consolidationNeeded",
		"runtime.faultToleranceState",

		// Guest
		"guest.toolsStatus",
		"guest.toolsVersion",
		"guest.toolsRunningStatus",
		"guest.ipAddress",
		"guest.hostName",
		"guest.net",
		"guest.guestState",
		"guestHeartbeatStatus",

		// Storage
		"datastore",
		"summary.storage",
		"layoutEx.file",
		"config.files.vmPathName",

		// Network
		"network",

		// Resource allocation
		"config.cpuAllocation",
		"config.memoryAllocation",
		"resourcePool",

		// Snapshots
		"snapshot",

		// Location
		"parent",
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
		// Find all VMs in datacenter (recursively through all folders)
		vms, err = finder.VirtualMachineList(ctx, "...")
		if err != nil {
			return nil, fmt.Errorf("failed to list VMs: %w", err)
		}
	}

	s.logger.WithField("vm_count", len(vms)).Info("Found VMs in vSphere")

	// Collect VM managed object references
	var vmRefs []vimtypes.ManagedObjectReference
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

	// Convert all VMs and apply filters
	var vmInfos []VMInfo
	for _, vmProp := range vmProperties {
		vmInfo := s.convertToVMInfo(vmProp)

		// Apply name filter (contains)
		if filter.Name != "" && !strings.Contains(strings.ToLower(vmInfo.Name), strings.ToLower(filter.Name)) {
			continue
		}

		// Apply power state filter
		if filter.PowerState != "" && vmInfo.PowerState != filter.PowerState {
			continue
		}

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

	// Basic Config properties
	if vm.Config != nil {
		info.InstanceUUID = vm.Config.InstanceUuid
		info.GuestFullName = vm.Config.GuestFullName
		info.GuestID = vm.Config.GuestId
		info.Version = vm.Config.Version
		info.Annotation = vm.Config.Annotation
		info.FirmwareType = vm.Config.Firmware
		info.Template = vm.Config.Template
		info.ChangeTrackingEnabled = vm.Config.ChangeTrackingEnabled != nil && *vm.Config.ChangeTrackingEnabled
		info.BiosUUID = vm.Config.Uuid
		info.CPUHotAddEnabled = vm.Config.CpuHotAddEnabled != nil && *vm.Config.CpuHotAddEnabled
		info.CPUHotRemoveEnabled = vm.Config.CpuHotRemoveEnabled != nil && *vm.Config.CpuHotRemoveEnabled
		info.MemoryHotAddEnabled = vm.Config.MemoryHotAddEnabled != nil && *vm.Config.MemoryHotAddEnabled

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

		// VM files
		if vm.Config.Files.VmPathName != "" {
			info.VMPathName = vm.Config.Files.VmPathName
		}

		// Resource allocation
		if vm.Config.CpuAllocation != nil {
			info.ResourceAllocation.CPUReservation = *vm.Config.CpuAllocation.Reservation
			if vm.Config.CpuAllocation.Limit != nil && *vm.Config.CpuAllocation.Limit != -1 {
				info.ResourceAllocation.CPULimit = *vm.Config.CpuAllocation.Limit
			}
			if vm.Config.CpuAllocation.Shares != nil {
				info.ResourceAllocation.CPUShares = vm.Config.CpuAllocation.Shares.Shares
				info.ResourceAllocation.CPUSharesLevel = string(vm.Config.CpuAllocation.Shares.Level)
			}
		}
		if vm.Config.MemoryAllocation != nil {
			info.ResourceAllocation.MemoryReservation = *vm.Config.MemoryAllocation.Reservation
			if vm.Config.MemoryAllocation.Limit != nil && *vm.Config.MemoryAllocation.Limit != -1 {
				info.ResourceAllocation.MemoryLimit = *vm.Config.MemoryAllocation.Limit
			}
			if vm.Config.MemoryAllocation.Shares != nil {
				info.ResourceAllocation.MemoryShares = vm.Config.MemoryAllocation.Shares.Shares
				info.ResourceAllocation.MemorySharesLevel = string(vm.Config.MemoryAllocation.Shares.Level)
			}
		}

		// Extract disk information
		info.Disks = s.extractDiskInfo(vm.Config.Hardware.Device)

		// Extract network adapter information
		info.NetworkAdapters = s.extractNetworkAdapters(vm.Config.Hardware.Device, vm.Guest)
	}

	// Runtime properties
	info.ConnectionState = string(vm.Runtime.ConnectionState)
	info.MaxCPUUsage = vm.Runtime.MaxCpuUsage
	info.MaxMemoryUsage = vm.Runtime.MaxMemoryUsage
	info.ConsolidationNeeded = vm.Runtime.ConsolidationNeeded != nil && *vm.Runtime.ConsolidationNeeded
	if vm.Runtime.FaultToleranceState != "" {
		info.FaultToleranceState = string(vm.Runtime.FaultToleranceState)
	}

	// Boot time and uptime
	if vm.Runtime.BootTime != nil {
		info.BootTime = *vm.Runtime.BootTime
		info.UptimeSeconds = int64(time.Since(*vm.Runtime.BootTime).Seconds())
	}

	// Host
	if vm.Runtime.Host != nil {
		info.Host = vm.Runtime.Host.Value
	}

	// Guest properties
	if vm.Guest != nil {
		info.ToolsStatus = string(vm.Guest.ToolsStatus)
		info.ToolsVersion = vm.Guest.ToolsVersion
		info.ToolsRunningStatus = vm.Guest.ToolsRunningStatus
		info.Hostname = vm.Guest.HostName
		info.GuestState = vm.Guest.GuestState

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

	// Guest heartbeat status
	info.GuestHeartbeatStatus = string(vm.GuestHeartbeatStatus)

	// Storage information from summary
	if vm.Summary.Storage.Committed > 0 {
		info.CommittedStorage = vm.Summary.Storage.Committed
	}
	if vm.Summary.Storage.Uncommitted > 0 {
		info.UncommittedStorage = vm.Summary.Storage.Uncommitted
	}

	// Datastores
	var datastores []string
	for _, ds := range vm.Datastore {
		datastores = append(datastores, ds.Value)
	}
	info.Datastores = datastores

	// Snapshot information
	if vm.Snapshot != nil {
		info.Snapshots = s.extractSnapshotInfo(vm.Snapshot.RootSnapshotList)
		if vm.Snapshot.CurrentSnapshot != nil {
			info.CurrentSnapshot = vm.Snapshot.CurrentSnapshot.Value
		}
	}

	// File layout
	if vm.LayoutEx != nil {
		var configFiles []string
		var logFiles []string
		for _, file := range vm.LayoutEx.File {
			if strings.HasSuffix(file.Name, ".vmx") || strings.HasSuffix(file.Name, ".nvram") {
				configFiles = append(configFiles, file.Name)
			} else if strings.HasSuffix(file.Name, ".log") {
				logFiles = append(logFiles, file.Name)
			}
		}
		info.ConfigFiles = configFiles
		info.LogFiles = logFiles
	}

	// Resource pool
	if vm.ResourcePool != nil {
		info.ResourcePool = vm.ResourcePool.Value
	}

	// Parent (folder)
	if vm.Parent != nil {
		info.Folder = vm.Parent.Value
	}

	return info
}

// GetSnapshotDiskInfo gets the VM moref, snapshot moref and disk path for a VM snapshot
// This is used by the inspection system to access snapshot disks via VDDK
func (s *VMService) GetSnapshotDiskInfo(ctx context.Context, vmName string, snapshotName string) (*types.SnapshotDiskInfo, error) {
	s.logger.WithFields(logrus.Fields{
		"vm_name":       vmName,
		"snapshot_name": snapshotName,
	}).Debug("Getting snapshot disk info for inspection")

	// Find VM by name
	vm, _, err := s.findVMByName(ctx, vmName)
	if err != nil {
		return nil, err
	}

	// Get the VM managed object reference value
	vmMoref := vm.Reference().Value

	// Get VM properties including snapshots and disk config
	client, err := s.client.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get vSphere client: %w", err)
	}

	var vmMo mo.VirtualMachine
	pc := property.DefaultCollector(client.Client)
	err = pc.RetrieveOne(ctx, vm.Reference(), []string{"snapshot", "config.hardware.device", "runtime.host"}, &vmMo)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM properties: %w", err)
	}

	// Check if VM has snapshots
	if vmMo.Snapshot == nil {
		return nil, fmt.Errorf("VM '%s' has no snapshots", vmName)
	}

	// Find the snapshot by name
	snapshotRef, err := s.findSnapshotInTree(vmMo.Snapshot.RootSnapshotList, snapshotName)
	if err != nil {
		return nil, fmt.Errorf("failed to find snapshot '%s': %w", snapshotName, err)
	}

	// Get snapshot moref
	snapshotMoref := snapshotRef.Snapshot.Value

	// Get disk paths from ALL virtual disks (not just the first one)
	// Use ParentFile (backing.Parent.FileName) if available
	// This is the base/parent disk file that the snapshot was created from
	var diskPaths []string
	var baseDiskPaths []string

	for _, device := range vmMo.Config.Hardware.Device {
		if disk, ok := device.(*vimtypes.VirtualDisk); ok {
			if backing, ok := disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo); ok {
				diskPath := backing.FileName
				diskPaths = append(diskPaths, diskPath)

				// Check if backing has a Parent
				// Parent points to the base disk file that the snapshot was created from
				var baseDiskPath string
				if backing.Parent != nil && backing.Parent.FileName != "" {
					baseDiskPath = backing.Parent.FileName
					s.logger.WithFields(logrus.Fields{
						"disk_path":   diskPath,
						"parent_file": baseDiskPath,
					}).Debug("Found parent file from disk backing")
				} else {
					// Fallback: calculate base disk path (remove delta disk suffix like -000002)
					baseDiskPath = s.getBaseDiskPath(diskPath)
					s.logger.WithFields(logrus.Fields{
						"disk_path":      diskPath,
						"calculated_base": baseDiskPath,
					}).Debug("Calculated base disk path (no parent in backing)")
				}
				baseDiskPaths = append(baseDiskPaths, baseDiskPath)
			}
		}
	}

	if len(diskPaths) == 0 {
		return nil, fmt.Errorf("no disks found for VM '%s'", vmName)
	}

	if len(baseDiskPaths) == 0 {
		return nil, fmt.Errorf("no base disk paths found for VM '%s'", vmName)
	}

	// Get compute resource path (host/cluster) for vpx:// URL
	var computeResourcePath string
	if vmMo.Runtime.Host != nil {
		finder := find.NewFinder(client.Client, true)
		host, err := finder.ObjectReference(ctx, *vmMo.Runtime.Host)
		if err == nil {
			if hostObj, ok := host.(*object.HostSystem); ok {
				// Get the host's inventory path
				computeResourcePath = hostObj.InventoryPath
				s.logger.WithField("compute_resource_path", computeResourcePath).Debug("Got compute resource path from host")
			}
		}
		// If we couldn't get the host path, try to get it from the host's parent (cluster)
		if computeResourcePath == "" && vmMo.Runtime.Host != nil {
			// Try to get cluster path by finding the host's parent
			var hostMo mo.HostSystem
			err = pc.RetrieveOne(ctx, *vmMo.Runtime.Host, []string{"parent"}, &hostMo)
			if err == nil && hostMo.Parent != nil {
				parentObj, err := finder.ObjectReference(ctx, *hostMo.Parent)
				if err == nil {
					if clusterObj, ok := parentObj.(*object.ClusterComputeResource); ok {
						computeResourcePath = clusterObj.InventoryPath
						s.logger.WithField("compute_resource_path", computeResourcePath).Debug("Got compute resource path from cluster")
					} else if computeResourceObj, ok := parentObj.(*object.ComputeResource); ok {
						computeResourcePath = computeResourceObj.InventoryPath
						s.logger.WithField("compute_resource_path", computeResourcePath).Debug("Got compute resource path from compute resource")
					}
				}
			}
		}
	}

	if computeResourcePath == "" {
		return nil, fmt.Errorf("failed to get compute resource path for VM '%s'", vmName)
	}

	s.logger.WithFields(logrus.Fields{
		"vm_moref":             vmMoref,
		"snapshot_moref":       snapshotMoref,
		"disk_count":           len(diskPaths),
		"disk_paths":           diskPaths,
		"base_disk_paths":      baseDiskPaths,
		"compute_resource_path": computeResourcePath,
	}).Debug("Got snapshot disk info")

	return &types.SnapshotDiskInfo{
		VMMoref:             vmMoref,
		SnapshotMoref:       snapshotMoref,
		DiskPaths:           diskPaths,
		BaseDiskPaths:       baseDiskPaths,
		ComputeResourcePath: computeResourcePath,
	}, nil
}

// findSnapshotInTree recursively searches for a snapshot by name in the snapshot tree
func (s *VMService) findSnapshotInTree(snapshots []vimtypes.VirtualMachineSnapshotTree, name string) (*vimtypes.VirtualMachineSnapshotTree, error) {
	for idx := range snapshots {
		if snapshots[idx].Name == name {
			return &snapshots[idx], nil
		}
		// Search in child snapshots
		if len(snapshots[idx].ChildSnapshotList) > 0 {
			result, err := s.findSnapshotInTree(snapshots[idx].ChildSnapshotList, name)
			if err == nil {
				return result, nil
			}
		}
	}
	return nil, fmt.Errorf("snapshot '%s' not found", name)
}

// getBaseDiskPath removes the -XXXXXX delta disk suffix to get the base VMDK path
// Example: "[datastore] vm/vm-000002.vmdk" -> "[datastore] vm/vm.vmdk"
func (s *VMService) getBaseDiskPath(diskPath string) string {
	// Find the last occurrence of .vmdk
	vmdkIndex := len(diskPath) - len(".vmdk")
	if vmdkIndex < 0 || diskPath[vmdkIndex:] != ".vmdk" {
		// Not a .vmdk file, return as-is
		return diskPath
	}

	// Find the part before .vmdk
	prefix := diskPath[:vmdkIndex]

	// Look for -XXXXXX pattern (6 digits) before .vmdk
	// Example: "vm-000002" -> "vm"
	if len(prefix) >= 7 && prefix[len(prefix)-7] == '-' {
		// Check if last 6 characters are digits
		isAllDigits := true
		for i := len(prefix) - 6; i < len(prefix); i++ {
			if prefix[i] < '0' || prefix[i] > '9' {
				isAllDigits = false
				break
			}
		}
		if isAllDigits {
			// Remove -XXXXXX suffix
			return prefix[:len(prefix)-7] + ".vmdk"
		}
	}

	// No delta disk suffix found, return original path
	return diskPath
}

// extractDiskInfo extracts disk information from hardware devices
func (s *VMService) extractDiskInfo(devices []vimtypes.BaseVirtualDevice) []VMDiskInfo {
	var disks []VMDiskInfo
	for _, device := range devices {
		if disk, ok := device.(*vimtypes.VirtualDisk); ok {
			diskInfo := VMDiskInfo{
				Label:         disk.DeviceInfo.GetDescription().Label,
				CapacityKB:    disk.CapacityInKB,
				ControllerKey: disk.ControllerKey,
			}

			if backing, ok := disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo); ok {
				diskInfo.DiskPath = backing.FileName
				diskInfo.ThinProvisioned = *backing.ThinProvisioned
				diskInfo.DiskMode = backing.DiskMode
				// Extract datastore from path [datastore] path/to/disk.vmdk
				if idx := strings.Index(backing.FileName, "]"); idx > 0 {
					diskInfo.Datastore = backing.FileName[1:idx]
				}
			}

			disks = append(disks, diskInfo)
		}
	}
	return disks
}

// extractNetworkAdapters extracts network adapter information from hardware devices
func (s *VMService) extractNetworkAdapters(devices []vimtypes.BaseVirtualDevice, guest *vimtypes.GuestInfo) []VMNetworkAdapterInfo {
	var adapters []VMNetworkAdapterInfo

	// Create a map of MAC to IPs from guest info
	macToIPs := make(map[string][]string)
	if guest != nil {
		for _, nic := range guest.Net {
			if nic.MacAddress != "" && nic.IpConfig != nil {
				var ips []string
				for _, ipConfig := range nic.IpConfig.IpAddress {
					if ipConfig.IpAddress != "" {
						ips = append(ips, ipConfig.IpAddress)
					}
				}
				macToIPs[nic.MacAddress] = ips
			}
		}
	}

	for _, device := range devices {
		var label, mac, network, adapterType string
		var connected bool

		switch nic := device.(type) {
		case *vimtypes.VirtualE1000:
			label = nic.DeviceInfo.GetDescription().Label
			mac = nic.MacAddress
			connected = nic.Connectable != nil && nic.Connectable.Connected
			adapterType = "E1000"
			if backing, ok := nic.Backing.(*vimtypes.VirtualEthernetCardNetworkBackingInfo); ok {
				network = backing.DeviceName
			}
		case *vimtypes.VirtualE1000e:
			label = nic.DeviceInfo.GetDescription().Label
			mac = nic.MacAddress
			connected = nic.Connectable != nil && nic.Connectable.Connected
			adapterType = "E1000e"
			if backing, ok := nic.Backing.(*vimtypes.VirtualEthernetCardNetworkBackingInfo); ok {
				network = backing.DeviceName
			}
		case *vimtypes.VirtualVmxnet3:
			label = nic.DeviceInfo.GetDescription().Label
			mac = nic.MacAddress
			connected = nic.Connectable != nil && nic.Connectable.Connected
			adapterType = "VMXNET3"
			if backing, ok := nic.Backing.(*vimtypes.VirtualEthernetCardNetworkBackingInfo); ok {
				network = backing.DeviceName
			}
		default:
			continue
		}

		adapter := VMNetworkAdapterInfo{
			Label:       label,
			NetworkName: network,
			MacAddress:  mac,
			Connected:   connected,
			AdapterType: adapterType,
			IPAddresses: macToIPs[mac],
		}
		adapters = append(adapters, adapter)
	}

	return adapters
}

// extractSnapshotInfo recursively extracts snapshot information
func (s *VMService) extractSnapshotInfo(snapshots []vimtypes.VirtualMachineSnapshotTree) []VMSnapshotInfo {
	var result []VMSnapshotInfo
	for _, snap := range snapshots {
		info := VMSnapshotInfo{
			Name:        snap.Name,
			Description: snap.Description,
			CreateTime:  snap.CreateTime,
			State:       string(snap.State),
			Quiesced:    snap.Quiesced,
			ID:          snap.Id,
		}
		result = append(result, info)

		// Recursively add child snapshots
		if len(snap.ChildSnapshotList) > 0 {
			result = append(result, s.extractSnapshotInfo(snap.ChildSnapshotList)...)
		}
	}
	return result
}

// FindSnapshotByName finds a snapshot by name on a VM
func (s *VMService) FindSnapshotByName(ctx context.Context, vmName string, snapshotName string) (*vimtypes.ManagedObjectReference, error) {
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
	var findSnapshot func(tree []vimtypes.VirtualMachineSnapshotTree) *vimtypes.ManagedObjectReference
	findSnapshot = func(tree []vimtypes.VirtualMachineSnapshotTree) *vimtypes.ManagedObjectReference {
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
func (s *VMService) CreateLinkedClone(ctx context.Context, vmName string, snapshotRef *vimtypes.ManagedObjectReference, cloneName string) error {
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
	cloneSpec := vimtypes.VirtualMachineCloneSpec{
		Location: vimtypes.VirtualMachineRelocateSpec{
			DiskMoveType: string(vimtypes.VirtualMachineRelocateDiskMoveOptionsCreateNewChildDiskBacking),
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
