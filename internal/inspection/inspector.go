package inspection

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"time"

	apitypes "github.com/nirarg/vm-deep-inspection-demo/pkg/types"
	"github.com/sirupsen/logrus"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// Inspector handles VM inspection operations
type Inspector struct {
	virtInspectorPath string
	timeout           time.Duration
	logger            *logrus.Logger
}

// NewInspector creates a new Inspector instance
func NewInspector(virtInspectorPath string, timeout time.Duration, logger *logrus.Logger) *Inspector {
	if virtInspectorPath == "" {
		virtInspectorPath = "virt-inspector" // Use system PATH
	}
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &Inspector{
		virtInspectorPath: virtInspectorPath,
		timeout:           timeout,
		logger:            logger,
	}
}

// ParseInspectionXML parses virt-inspector XML output
func (i *Inspector) ParseInspectionXML(xmlData []byte) (*apitypes.InspectionData, error) {
	// virt-inspector XML structure
	type XMLOperatingsystems struct {
		Operatingsystems []struct {
			Name              string `xml:"name"`
			Distro            string `xml:"distro"`
			MajorVersion      string `xml:"major_version"`
			MinorVersion      string `xml:"minor_version"`
			Architecture      string `xml:"arch"`
			Hostname          string `xml:"hostname"`
			Product           string `xml:"product_name"`
			Root              string `xml:"root"`
			PackageFormat     string `xml:"package_format"`
			PackageManagement string `xml:"package_management"`
			OSInfo            string `xml:"osinfo"`
			Applications struct {
				Application []struct {
					Name        string `xml:"name"`
					Version     string `xml:"version"`
					Epoch       int    `xml:"epoch"`
					Release     string `xml:"release"`
					Arch        string `xml:"arch"`
					URL         string `xml:"url"`
					Summary     string `xml:"summary"`
					Description string `xml:"description"`
				} `xml:"application"`
			} `xml:"applications"`
			Filesystems struct {
				Filesystem []struct {
					Device string `xml:"dev,attr"`
					Type   string `xml:"type"`
					UUID   string `xml:"uuid"`
				} `xml:"filesystem"`
			} `xml:"filesystems"`
			Mountpoints struct {
				Mountpoint []struct {
					Device     string `xml:"dev,attr"`
					MountPoint string `xml:",chardata"`
				} `xml:"mountpoint"`
			} `xml:"mountpoints"`
			Drives struct {
				Drive []struct {
					Name string `xml:"name,attr"`
				} `xml:"drive"`
			} `xml:"drives"`
		} `xml:"operatingsystem"`
	}

	var xmlRoot XMLOperatingsystems
	err := xml.Unmarshal(xmlData, &xmlRoot)
	if err != nil {
		return nil, fmt.Errorf("XML parsing error: %w", err)
	}

	if len(xmlRoot.Operatingsystems) == 0 {
		return nil, fmt.Errorf("no operating systems found in inspection output")
	}

	// Convert to our types (using first OS found)
	os := xmlRoot.Operatingsystems[0]

	// Construct version string from major.minor
	version := os.MajorVersion
	if os.MinorVersion != "" && os.MinorVersion != "0" {
		version = os.MajorVersion + "." + os.MinorVersion
	}

	data := &apitypes.InspectionData{
		OperatingSystem: &apitypes.OSInfo{
			Name:              os.Name,
			Distro:            os.Distro,
			Version:           version,
			Architecture:      os.Architecture,
			Hostname:          os.Hostname,
			Product:           os.Product,
			Root:              os.Root,
			PackageFormat:     os.PackageFormat,
			PackageManagement: os.PackageManagement,
			OSInfo:            os.OSInfo,
		},
		Applications: make([]apitypes.Application, 0),
		Filesystems:  make([]apitypes.Filesystem, 0),
		Mountpoints:  make([]apitypes.Mountpoint, 0),
		Drives:       make([]apitypes.Drive, 0),
	}

	// Convert applications
	for _, app := range os.Applications.Application {
		data.Applications = append(data.Applications, apitypes.Application{
			Name:        app.Name,
			Version:     app.Version,
			Epoch:       app.Epoch,
			Release:     app.Release,
			Arch:        app.Arch,
			URL:         app.URL,
			Summary:     app.Summary,
			Description: app.Description,
		})
	}

	// Convert filesystems
	for _, fs := range os.Filesystems.Filesystem {
		data.Filesystems = append(data.Filesystems, apitypes.Filesystem{
			Device: fs.Device,
			Type:   fs.Type,
			UUID:   fs.UUID,
		})
	}

	// Convert mountpoints
	for _, mp := range os.Mountpoints.Mountpoint {
		data.Mountpoints = append(data.Mountpoints, apitypes.Mountpoint{
			Device:     mp.Device,
			MountPoint: mp.MountPoint,
		})
	}

	// Convert drives
	for _, drive := range os.Drives.Drive {
		data.Drives = append(data.Drives, apitypes.Drive{
			Name: drive.Name,
		})
	}

	return data, nil
}

// RunVirtInspectorWithVDDK uses virt-v2v-open with VDDK for full disk inspection
// This method requires VMware VDDK to be installed and virt-v2v available
func (i *Inspector) RunVirtInspectorWithVDDK(ctx context.Context, vmName string, vcenterURL string, username string, password string, vmClient interface{}) (*apitypes.InspectionData, error) {
	i.logger.WithFields(logrus.Fields{
		"vm_name":     vmName,
		"vcenter_url": vcenterURL,
	}).Info("Running virt-inspector with VDDK via virt-v2v-open")

	// Parse vCenter URL to extract hostname
	parsedURL, err := url.Parse(vcenterURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse vCenter URL: %w", err)
	}
	vcenterHost := parsedURL.Hostname()

	// Get VM moref (managed object reference) and disk path
	i.logger.Debug("Getting VM managed object reference and disk path")
	moref, diskPath, err := i.getVMDiskInfo(ctx, vmName, vmClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM info: %w", err)
	}
	i.logger.WithFields(logrus.Fields{
		"moref":     moref,
		"disk_path": diskPath,
	}).Debug("Got VM moref and disk path")

	// Get vCenter SSL thumbprint
	i.logger.Debug("Getting vCenter SSL thumbprint")
	thumbprint, err := i.getVCenterThumbprint(vcenterHost)
	if err != nil {
		i.logger.WithError(err).Warn("Failed to get thumbprint, proceeding without SSL verification")
		thumbprint = ""
	}
	if thumbprint != "" {
		i.logger.WithField("thumbprint", thumbprint).Debug("Got vCenter thumbprint")
	}

	// Build virt-v2v-open command with VDDK input transport
	// virt-v2v-open uses -it vddk to directly connect to VMware via VDDK
	// This eliminates the need for nbdkit and socket management
	i.logger.Info("Running virt-inspector via virt-v2v-open with VDDK")

	// Create context with timeout
	inspectCtx, cancel := context.WithTimeout(ctx, i.timeout)
	defer cancel()

	// Build VDDK connection parameters for virt-v2v-open
	// Format: server=HOST,user=USER,password=PASS,vm=moref=MOREF,file=PATH,libdir=DIR,single-link=true
	vddkParams := fmt.Sprintf("server=%s,user=%s,password=%s,vm=moref=%s,file=%s,libdir=/opt/vmware-vix-disklib,single-link=true",
		vcenterHost, username, password, moref, diskPath)

	// Add thumbprint if available
	if thumbprint != "" {
		vddkParams += fmt.Sprintf(",thumbprint=%s", thumbprint)
	}

	// virt-v2v-open command structure:
	// virt-v2v-open -it vddk -ip "PARAMS" --run 'COMMAND'
	// The @@ placeholder in the command is replaced with the disk device path
	// CRITICAL: Unset LD_LIBRARY_PATH to avoid VDDK library conflicts
	cmdString := fmt.Sprintf("unset LD_LIBRARY_PATH && virt-v2v-open -it vddk -ip '%s' --run '%s --format=raw -a @@'",
		vddkParams, i.virtInspectorPath)

	virtInspectorCmd := exec.CommandContext(inspectCtx, "sh", "-c", cmdString)

	output, err := virtInspectorCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("virt-inspector failed: %w, output: %s", err, string(output))
	}

	i.logger.Debug("virt-inspector completed, parsing output")

	// Parse XML output
	inspectionData, err := i.ParseInspectionXML(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse inspection output: %w", err)
	}

	i.logger.Info("VDDK inspection completed successfully")
	return inspectionData, nil
}

// RunVirtInspectorWithVDDKSnapshot uses virt-v2v-open with VDDK to inspect a VM snapshot directly
// This replaces the previous nbdkit-based approach and simplifies the implementation
func (i *Inspector) RunVirtInspectorWithVDDKSnapshot(ctx context.Context, vmName string, snapshotName string, vcenterURL string, username string, password string, vmClient interface{}) (*apitypes.InspectionData, error) {
	i.logger.WithFields(logrus.Fields{
		"vm_name":       vmName,
		"snapshot_name": snapshotName,
		"vcenter_url":   vcenterURL,
	}).Info("Running virt-inspector with VDDK on snapshot")

	// Parse vCenter URL to extract hostname
	parsedURL, err := url.Parse(vcenterURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse vCenter URL: %w", err)
	}
	vcenterHost := parsedURL.Hostname()

	// Get VM moref, snapshot moref and disk path
	i.logger.Debug("Getting VM and snapshot managed object references and disk path")
	vmMoref, snapshotMoref, diskPath, err := i.getSnapshotDiskInfo(ctx, vmName, snapshotName, vmClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot info: %w", err)
	}

	// For snapshots, we need to use the base VMDK file, not the delta disk
	// Remove -XXXXXX suffix from disk path to get the base disk
	baseDiskPath := i.getBaseDiskPath(diskPath)

	i.logger.WithFields(logrus.Fields{
		"vm_moref":       vmMoref,
		"snapshot_moref": snapshotMoref,
		"disk_path":      diskPath,
		"base_disk_path": baseDiskPath,
	}).Debug("Got VM moref, snapshot moref and disk paths")

	// Get vCenter SSL thumbprint
	i.logger.Debug("Getting vCenter SSL thumbprint")
	thumbprint, err := i.getVCenterThumbprint(vcenterHost)
	if err != nil {
		i.logger.WithError(err).Warn("Failed to get thumbprint, proceeding without SSL verification")
		thumbprint = ""
	}
	if thumbprint != "" {
		i.logger.WithField("thumbprint", thumbprint).Debug("Got vCenter thumbprint")
	}

	// Build virt-v2v-open command with VDDK input transport
	// virt-v2v-open uses -it vddk to directly connect to VMware via VDDK
	// This eliminates the need for nbdkit and socket management
	i.logger.Info("Running virt-inspector via virt-v2v-open with VDDK")

	// Create context with timeout
	inspectCtx, cancel := context.WithTimeout(ctx, i.timeout)
	defer cancel()

	// Build VDDK connection parameters for virt-v2v-open
	// Format: server=HOST,user=USER,password=PASS,vm=moref=MOREF,snapshot=SNAPSHOT,file=PATH,libdir=DIR
	vddkParams := fmt.Sprintf("server=%s,user=%s,password=%s,vm=moref=%s,snapshot=%s,file=%s,libdir=/opt/vmware-vix-disklib",
		vcenterHost, username, password, vmMoref, snapshotMoref, baseDiskPath)

	// Add thumbprint if available
	if thumbprint != "" {
		vddkParams += fmt.Sprintf(",thumbprint=%s", thumbprint)
	}

	// virt-v2v-open command structure:
	// virt-v2v-open -it vddk -ip "PARAMS" --run 'COMMAND'
	// The @@ placeholder in the command is replaced with the disk device path
	// CRITICAL: Unset LD_LIBRARY_PATH to avoid VDDK library conflicts
	cmdString := fmt.Sprintf("unset LD_LIBRARY_PATH && virt-v2v-open -it vddk -ip '%s' --run '%s --format=raw -a @@'",
		vddkParams, i.virtInspectorPath)

	virtInspectorCmd := exec.CommandContext(inspectCtx, "sh", "-c", cmdString)

	output, err := virtInspectorCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("virt-inspector failed: %w, output: %s", err, string(output))
	}

	// Parse XML output
	inspectionData, err := i.ParseInspectionXML(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse inspection output: %w", err)
	}

	i.logger.Info("VDDK snapshot inspection completed successfully")
	return inspectionData, nil
}

// getVMDiskInfo gets the managed object reference (moref) and first disk path for a VM
func (i *Inspector) getVMDiskInfo(ctx context.Context, vmName string, vmClient interface{}) (string, string, error) {
	// Type assert to get the vim25 client
	// The vmClient should be *vim25.Client passed from the caller
	client, ok := vmClient.(*vim25.Client)
	if !ok {
		return "", "", fmt.Errorf("invalid client type for moref lookup")
	}

	// Create finder
	finder := find.NewFinder(client, true)

	// Get default datacenter
	datacenter, err := finder.DefaultDatacenter(ctx)
	if err != nil {
		return "", "", fmt.Errorf("failed to find default datacenter: %w", err)
	}
	finder.SetDatacenter(datacenter)

	// Find VM by name
	vm, err := finder.VirtualMachine(ctx, vmName)
	if err != nil {
		return "", "", fmt.Errorf("failed to find VM '%s': %w", vmName, err)
	}

	// Get the managed object reference value
	moref := vm.Reference().Value

	// Get VM configuration to find disk path
	var vmMo mo.VirtualMachine
	err = vm.Properties(ctx, vm.Reference(), []string{"config.hardware.device"}, &vmMo)
	if err != nil {
		return "", "", fmt.Errorf("failed to get VM config: %w", err)
	}

	// Find first virtual disk
	var diskPath string
	for _, device := range vmMo.Config.Hardware.Device {
		if disk, ok := device.(*types.VirtualDisk); ok {
			if backing, ok := disk.Backing.(*types.VirtualDiskFlatVer2BackingInfo); ok {
				diskPath = backing.FileName
				break
			}
		}
	}

	if diskPath == "" {
		return "", "", fmt.Errorf("no disk found for VM '%s'", vmName)
	}

	return moref, diskPath, nil
}

// getSnapshotDiskInfo gets the VM moref, snapshot moref and disk path for a VM snapshot
func (i *Inspector) getSnapshotDiskInfo(ctx context.Context, vmName string, snapshotName string, vmClient interface{}) (string, string, string, error) {
	// Type assert to get the vim25 client
	client, ok := vmClient.(*vim25.Client)
	if !ok {
		return "", "", "", fmt.Errorf("invalid client type for snapshot lookup")
	}

	// Create finder
	finder := find.NewFinder(client, true)

	// Get default datacenter
	datacenter, err := finder.DefaultDatacenter(ctx)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to find default datacenter: %w", err)
	}
	finder.SetDatacenter(datacenter)

	// Find VM by name
	vm, err := finder.VirtualMachine(ctx, vmName)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to find VM '%s': %w", vmName, err)
	}

	// Get the VM managed object reference value
	vmMoref := vm.Reference().Value

	// Get VM properties including snapshots
	var vmMo mo.VirtualMachine
	err = vm.Properties(ctx, vm.Reference(), []string{"snapshot", "config.hardware.device"}, &vmMo)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get VM properties: %w", err)
	}

	// Check if VM has snapshots
	if vmMo.Snapshot == nil {
		return "", "", "", fmt.Errorf("VM '%s' has no snapshots", vmName)
	}

	// Find the snapshot by name
	snapshotRef, err := i.findSnapshot(vmMo.Snapshot.RootSnapshotList, snapshotName)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to find snapshot '%s': %w", snapshotName, err)
	}

	// Get snapshot moref
	snapshotMoref := snapshotRef.Snapshot.Value

	// Get disk path from first virtual disk
	var diskPath string
	for _, device := range vmMo.Config.Hardware.Device {
		if disk, ok := device.(*types.VirtualDisk); ok {
			if backing, ok := disk.Backing.(*types.VirtualDiskFlatVer2BackingInfo); ok {
				diskPath = backing.FileName
				break
			}
		}
	}

	if diskPath == "" {
		return "", "", "", fmt.Errorf("no disk found for VM '%s'", vmName)
	}

	return vmMoref, snapshotMoref, diskPath, nil
}

// findSnapshot recursively searches for a snapshot by name in the snapshot tree
func (i *Inspector) findSnapshot(snapshots []types.VirtualMachineSnapshotTree, name string) (*types.VirtualMachineSnapshotTree, error) {
	for idx := range snapshots {
		if snapshots[idx].Name == name {
			return &snapshots[idx], nil
		}
		// Search in child snapshots
		if len(snapshots[idx].ChildSnapshotList) > 0 {
			result, err := i.findSnapshot(snapshots[idx].ChildSnapshotList, name)
			if err == nil {
				return result, nil
			}
		}
	}
	return nil, fmt.Errorf("snapshot '%s' not found", name)
}

// getVCenterThumbprint gets the SSL certificate thumbprint from vCenter
func (i *Inspector) getVCenterThumbprint(vcenterHost string) (string, error) {
	// Connect to vCenter to get SSL certificate
	conn, err := tls.Dial("tcp", vcenterHost+":443", &tls.Config{
		InsecureSkipVerify: true, // We just need the cert, not to verify it
	})
	if err != nil {
		return "", fmt.Errorf("failed to connect to vCenter: %w", err)
	}
	defer conn.Close()

	// Get the certificate chain
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return "", fmt.Errorf("no certificates found")
	}

	// Use the first certificate (server certificate)
	cert := certs[0]

	// Calculate SHA-256 thumbprint
	thumbprint := sha256.Sum256(cert.Raw)

	// Format as colon-separated hex string (VMware format)
	hexThumbprint := hex.EncodeToString(thumbprint[:])
	formatted := ""
	for i := 0; i < len(hexThumbprint); i += 2 {
		if i > 0 {
			formatted += ":"
		}
		formatted += hexThumbprint[i : i+2]
	}

	return formatted, nil
}

// getBaseDiskPath removes the -XXXXXX delta disk suffix to get the base VMDK path
// Example: "[datastore] vm/vm-000002.vmdk" -> "[datastore] vm/vm.vmdk"
func (i *Inspector) getBaseDiskPath(diskPath string) string {
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
