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

	"github.com/google/uuid"
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

// RunVirtInspector executes virt-inspector on a VM
func (i *Inspector) RunVirtInspector(ctx context.Context, vmName string, vcenterURL string, username string, password string) (*apitypes.InspectionData, error) {
	i.logger.WithFields(logrus.Fields{
		"vm_name":     vmName,
		"vcenter_url": vcenterURL,
	}).Info("Running virt-inspector")

	// Parse vCenter URL to extract hostname
	parsedURL, err := url.Parse(vcenterURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse vCenter URL: %w", err)
	}
	vcenterHost := parsedURL.Hostname()

	// URL-encode username (may contain @ symbol)
	encodedUsername := url.QueryEscape(username)

	// Build VMware connection string
	// Format: vpx://username@vcenter/vm-name?no_verify=1
	// Note: Simplified format without datacenter path
	connectionString := fmt.Sprintf("vpx://%s@%s/%s?no_verify=1", encodedUsername, vcenterHost, vmName)

	// Create context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, i.timeout)
	defer cancel()

	// Build command
	cmd := exec.CommandContext(timeoutCtx, i.virtInspectorPath, "-a", connectionString)

	// Set password via environment
	cmd.Env = append(cmd.Env, fmt.Sprintf("LIBGUESTFS_BACKEND_SETTINGS=password=%s", password))

	i.logger.Debug("Executing virt-inspector command")

	// Execute command
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("virt-inspector execution failed: %w, output: %s", err, string(output))
	}

	i.logger.Debug("virt-inspector completed, parsing output")

	// Parse XML output
	inspectionData, err := i.ParseInspectionXML(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse inspection output: %w", err)
	}

	i.logger.Info("Inspection completed successfully")
	return inspectionData, nil
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

// RunVirtInspectorWithVDDK uses nbdkit-vddk plugin for full disk inspection
// This method requires VMware VDDK to be installed and nbdkit-vddk-plugin available
func (i *Inspector) RunVirtInspectorWithVDDK(ctx context.Context, vmName string, vcenterURL string, username string, password string, vmClient interface{}) (*apitypes.InspectionData, error) {
	i.logger.WithFields(logrus.Fields{
		"vm_name":     vmName,
		"vcenter_url": vcenterURL,
	}).Info("Running virt-inspector with VDDK/nbdkit")

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

	// Create temporary socket for nbdkit
	socketPath := fmt.Sprintf("/tmp/nbdkit-%s.sock", uuid.New().String())
	defer os.Remove(socketPath)

	i.logger.WithField("socket", socketPath).Debug("Starting nbdkit with VDDK plugin")

	// For linked clones, we need to open with snapshot reference
	// Use single-link mode which reads from the current VM state
	nbdkitArgs := []string{
		"-U", socketPath,     // Unix socket path
		"--foreground",       // Run in foreground
		"--exit-with-parent", // Exit when parent process exits
		"vddk",               // VDDK plugin
		fmt.Sprintf("server=%s", vcenterHost),
		fmt.Sprintf("user=%s", username),
		fmt.Sprintf("password=%s", password),
		fmt.Sprintf("vm=moref=%s", moref),
		fmt.Sprintf("file=%s", diskPath),    // VMDK file path
		"libdir=/opt/vmware-vix-disklib",    // VDDK library location
		"single-link=true",                  // Read current state (works with clones)
	}

	// Add thumbprint if available (for SSL verification)
	if thumbprint != "" {
		nbdkitArgs = append(nbdkitArgs, fmt.Sprintf("thumbprint=%s", thumbprint))
	}

	// Start nbdkit with VDDK plugin
	nbdkitCmd := exec.CommandContext(ctx, "nbdkit", nbdkitArgs...)

	if err := nbdkitCmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start nbdkit: %w", err)
	}

	// Ensure nbdkit is killed when we're done
	defer func() {
		if nbdkitCmd.Process != nil {
			nbdkitCmd.Process.Kill()
		}
	}()

	// Wait for socket to be ready
	if err := i.waitForSocket(socketPath, 30*time.Second); err != nil {
		return nil, fmt.Errorf("nbdkit socket not ready: %w", err)
	}

	// Create context with timeout for virt-inspector
	inspectCtx, cancel := context.WithTimeout(ctx, i.timeout)
	defer cancel()

	// Run virt-inspector on the NBD socket
	i.logger.WithField("socket", socketPath).Info("Running virt-inspector on nbdkit socket")
	virtInspectorCmd := exec.CommandContext(inspectCtx, i.virtInspectorPath,
		"-a", fmt.Sprintf("nbd+unix:///?socket=%s", socketPath),
	)

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

// RunVirtInspectorWithVDDKSnapshot uses nbdkit-vddk plugin to inspect a VM snapshot directly
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

	// Create temporary socket for nbdkit
	socketPath := fmt.Sprintf("/tmp/nbdkit-%s.sock", uuid.New().String())
	defer os.Remove(socketPath)

	i.logger.WithField("socket", socketPath).Debug("Starting nbdkit with VDDK plugin for snapshot")

	// Build nbdkit command arguments for snapshot access
	// Note: When using snapshot parameter, we specify the base VMDK file (not the delta disk)
	// and MUST use readonly mode (-r flag) since snapshots are read-only by definition
	nbdkitArgs := []string{
		"-U", socketPath,     // Unix socket path
		"--foreground",       // Run in foreground
		"--exit-with-parent", // Exit when parent process exits
		"-r",                 // Read-only mode for snapshots
		"vddk",               // VDDK plugin
		fmt.Sprintf("server=%s", vcenterHost),
		fmt.Sprintf("user=%s", username),
		fmt.Sprintf("password=%s", password),
		fmt.Sprintf("vm=moref=%s", vmMoref),       // VM moref (required)
		fmt.Sprintf("snapshot=%s", snapshotMoref), // Snapshot moref to read from
		fmt.Sprintf("file=%s", baseDiskPath),      // Base VMDK file path (without -XXXXXX suffix)
		"libdir=/opt/vmware-vix-disklib",          // VDDK library location
	}

	// Add thumbprint if available (for SSL verification)
	if thumbprint != "" {
		nbdkitArgs = append(nbdkitArgs, fmt.Sprintf("thumbprint=%s", thumbprint))
	}

	// Start nbdkit with VDDK plugin
	nbdkitCmd := exec.CommandContext(ctx, "nbdkit", nbdkitArgs...)

	if err := nbdkitCmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start nbdkit: %w", err)
	}

	// Ensure nbdkit is killed when we're done
	defer func() {
		if nbdkitCmd.Process != nil {
			nbdkitCmd.Process.Kill()
		}
	}()

	// Wait for socket to be ready
	if err := i.waitForSocket(socketPath, 30*time.Second); err != nil {
		return nil, fmt.Errorf("nbdkit socket not ready: %w", err)
	}

	// Create context with timeout for virt-inspector
	inspectCtx, cancel := context.WithTimeout(ctx, i.timeout)
	defer cancel()

	// Run virt-inspector on the NBD socket
	// Use --format=raw to avoid disk format probing issues with NBD
	//
	// CRITICAL: Use shell wrapper to explicitly unset LD_LIBRARY_PATH before running virt-inspector
	// This ensures virt-inspector and ALL its child processes (libguestfs, supermin, qemu)
	// have a clean environment without VDDK's libcrypto.so.3, which conflicts with system libraries.
	i.logger.WithField("socket", socketPath).Info("Running virt-inspector on nbdkit socket")

	cmdString := fmt.Sprintf("unset LD_LIBRARY_PATH && %s --format=raw -a 'nbd+unix:///?socket=%s'",
		i.virtInspectorPath, socketPath)

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

// waitForSocket waits for a Unix socket to be created
func (i *Inspector) waitForSocket(socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if _, err := os.Stat(socketPath); err == nil {
				// Socket exists
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for socket after %v", timeout)
			}
		}
	}
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
