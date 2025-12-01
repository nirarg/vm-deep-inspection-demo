package inspection

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nirarg/vm-deep-inspection-demo/internal/types"
	apitypes "github.com/nirarg/vm-deep-inspection-demo/pkg/types"
	"github.com/sirupsen/logrus"
)

// VirtV2vInspector handles VM inspection operations using virt-v2v-inspector
type VirtV2vInspector struct {
	virtV2vInspectorPath string
	timeout              time.Duration
	logger               *logrus.Logger
}

// NewVirtV2vInspector creates a new VirtV2vInspector instance
func NewVirtV2vInspector(virtV2vInspectorPath string, timeout time.Duration, logger *logrus.Logger) *VirtV2vInspector {
	if virtV2vInspectorPath == "" {
		virtV2vInspectorPath = "virt-v2v-inspector" // Use system PATH
	}
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &VirtV2vInspector{
		virtV2vInspectorPath: virtV2vInspectorPath,
		timeout:              timeout,
		logger:               logger,
	}
}

// Inspect uses virt-v2v-inspector to inspect a VM snapshot directly via VDDK
func (i *VirtV2vInspector) Inspect(
	ctx context.Context,
	vmName string,
	snapshotName string,
	vcenterURL string,
	datacenter string,
	username string,
	password string,
	diskInfo *types.SnapshotDiskInfo, // Snapshot disk info from vm_service
	sslVerify string, // SSL verification option for vpx:// URL (e.g., "no_verify=1" or "cacert=/path/to/ca-bundle.crt")
) (*apitypes.InspectionData, error) {
	i.logger.WithFields(logrus.Fields{
		"vm_name":       vmName,
		"snapshot_name": snapshotName,
		"vcenter_url":   vcenterURL,
		"datacenter":    datacenter,
	}).Info("Running virt-v2v-inspector on snapshot")

	// Build libvirt connection URL for vSphere
	// Format: vpx://username@vcenter/compute-resource-path?ssl-verify
	// The path must point to a compute resource (host/cluster), not the datacenter or VM
	// The VM name is specified as a positional argument after "--"
	// Extract hostname from vCenter URL
	vcenterHost := extractHostname(vcenterURL)

	// URL-encode username to handle special characters like @
	// The @ symbol in the username needs to be percent-encoded as %40
	// because @ is used as a delimiter between username and hostname in URLs
	// We use url.QueryEscape which encodes @ as %40
	encodedUsername := url.QueryEscape(username)

	// Use the compute resource path from diskInfo (e.g., "/Datacenter/Cluster/host.example.com")
	// This is required for vpx:// URLs - they need a compute resource, not just a datacenter
	computeResourcePath := diskInfo.ComputeResourcePath
	if computeResourcePath == "" {
		return nil, fmt.Errorf("compute resource path is required for vpx:// URL")
	}

	// Build vpx:// URL (without snapshot parameter)
	// Inspect the base/parent disk file directly
	// The snapshot parameter is not needed when using the parent file
	// Add SSL verification parameter (provided by caller)
	libvirtURL := fmt.Sprintf("vpx://%s@%s%s?%s",
		encodedUsername, vcenterHost, computeResourcePath, sslVerify)

	// Create context with timeout
	inspectCtx, cancel := context.WithTimeout(ctx, i.timeout)
	defer cancel()

	// virt-v2v-inspector expects -ip to be a file path, not the password directly
	// Create a temporary file with the password
	passwordFile, err := i.createPasswordFile(password)
	if err != nil {
		return nil, fmt.Errorf("failed to create password file: %w", err)
	}
	defer os.Remove(passwordFile) // Clean up the temporary file

	var output []byte

	// Build virt-v2v-inspector command
	args := []string{
		"-v",            // Verbose
		"-x",            // Debug
		"-i", "libvirt", // Input type: libvirt
		"-ic", libvirtURL, // libvirt connection URI (vpx://...)
		"-ip", passwordFile, // libvirt password file path (not password directly)
		"-it", "vddk", // Input transport: VDDK
	}

	// Add VDDK options
	// Get vCenter thumbprint
	thumbprint, err := getVCenterThumbprint(vcenterHost)
	if err != nil {
		i.logger.WithError(err).Warn("Failed to get thumbprint, proceeding without SSL verification")
	} else if thumbprint != "" {
		args = append(args, "-io", fmt.Sprintf("vddk-thumbprint=%s", thumbprint))
	}

	// Add VDDK library directory
	vddkLibDir := findVDDKLibDir()
	if vddkLibDir != "" {
		args = append(args, "-io", fmt.Sprintf("vddk-libdir=%s", vddkLibDir))
	}

	// Add disk file specification
	// virt-v2v-inspector needs the disk file path in VDDK format
	// Format: vddk-file=[datastore] path/to/disk.vmdk
	if diskInfo.BaseDiskPath != "" {
		args = append(args, "-io", fmt.Sprintf("vddk-file=%s", diskInfo.BaseDiskPath))
	}

	args = append(args, "--", vmName)

	// Log the command (without password file path)
	if i.logger != nil {
		logArgs := make([]string, len(args))
		copy(logArgs, args)
		// Mask password file path in log
		for idx, arg := range logArgs {
			if arg == "-ip" && idx+1 < len(logArgs) {
				logArgs[idx+1] = "***"
			}
		}
		i.logger.WithFields(logrus.Fields{
			"command": "virt-v2v-inspector",
			"args":    logArgs,
		}).Info("Running virt-v2v-inspector command")
	}

	// Execute virt-v2v-inspector
	cmd := exec.CommandContext(inspectCtx, i.virtV2vInspectorPath, args...)

	// Filter out VDDK library paths from LD_LIBRARY_PATH to prevent supermin
	// (called by libguestfs) from picking up VDDK's OpenSSL library
	// virt-v2v-inspector will spawn nbdkit internally, and nbdkit's wrapper
	// will set LD_LIBRARY_PATH only for nbdkit itself
	env := os.Environ()
	filteredEnv := make([]string, 0, len(env))
	vddkLibPath := "/opt/vmware-vix-disklib/lib64"
	for _, e := range env {
		// Remove VDDK library path from LD_LIBRARY_PATH if present
		if strings.HasPrefix(e, "LD_LIBRARY_PATH=") {
			ldPath := strings.TrimPrefix(e, "LD_LIBRARY_PATH=")
			// Filter out VDDK library path
			paths := strings.Split(ldPath, ":")
			filteredPaths := make([]string, 0, len(paths))
			for _, p := range paths {
				if p != vddkLibPath && !strings.Contains(p, "vmware-vix-disklib") {
					filteredPaths = append(filteredPaths, p)
				}
			}
			if len(filteredPaths) > 0 {
				filteredEnv = append(filteredEnv, fmt.Sprintf("LD_LIBRARY_PATH=%s", strings.Join(filteredPaths, ":")))
			}
			// If LD_LIBRARY_PATH becomes empty, don't set it at all
		} else {
			filteredEnv = append(filteredEnv, e)
		}
	}
	cmd.Env = filteredEnv

	// Log environment filtering for debugging
	if i.logger != nil {
		hasVddkPath := false
		for _, e := range filteredEnv {
			if strings.HasPrefix(e, "LD_LIBRARY_PATH=") && strings.Contains(e, "vmware-vix-disklib") {
				hasVddkPath = true
				break
			}
		}
		if hasVddkPath {
			i.logger.Warn("LD_LIBRARY_PATH still contains VDDK paths after filtering")
		} else {
			i.logger.Debug("LD_LIBRARY_PATH filtered successfully (VDDK paths removed)")
		}
	}

	// Capture output with timeout handling
	// Use a goroutine to capture output so we can monitor for context cancellation
	type result struct {
		output []byte
		err    error
	}
	resultChan := make(chan result, 1)

	go func() {
		output, err := cmd.CombinedOutput()
		resultChan <- result{output: output, err: err}
	}()

	// Wait for either completion or context cancellation
	select {
	case res := <-resultChan:
		output = res.output
		err = res.err
	case <-inspectCtx.Done():
		// Context was cancelled (timeout or parent cancellation)
		// Kill the process if it's still running
		if cmd.Process != nil {
			if killErr := cmd.Process.Kill(); killErr != nil {
				if i.logger != nil {
					i.logger.WithError(killErr).Warn("Failed to kill virt-v2v-inspector process after timeout")
				}
			} else if i.logger != nil {
				i.logger.Warn("Killed virt-v2v-inspector process due to timeout")
			}
		}
		if inspectCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("virt-v2v-inspector command timed out after %v", i.timeout)
		}
		return nil, fmt.Errorf("virt-v2v-inspector command was cancelled: %w", inspectCtx.Err())
	}

	outputStr := string(output)
	if err != nil {
		// Get exit code if available
		exitCode := -1
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		}
		i.logger.WithFields(logrus.Fields{
			"output":    outputStr,
			"exit_code": exitCode,
			"command":   i.virtV2vInspectorPath,
			"args":      args,
		}).Error("virt-v2v-inspector failed")

		// Include output in error message for better debugging
		if outputStr != "" {
			return nil, fmt.Errorf("virt-v2v-inspector failed (exit code %d): %w\nOutput: %s", exitCode, err, outputStr)
		}
		return nil, fmt.Errorf("virt-v2v-inspector failed (exit code %d): %w", exitCode, err)
	}

	// Extract XML from output (virt-v2v-inspector with -v -x may output debug messages)
	// Look for XML content - it should start with <?xml or <v2v-inspection>
	xmlStart := strings.Index(outputStr, "<?xml")
	if xmlStart == -1 {
		xmlStart = strings.Index(outputStr, "<v2v-inspection")
	}
	if xmlStart == -1 {
		xmlStart = strings.Index(outputStr, "<operatingsystem")
	}
	if xmlStart == -1 {
		xmlStart = strings.Index(outputStr, "<inspection")
	}

	var xmlData []byte
	if xmlStart >= 0 {
		// Extract XML portion from output
		xmlData = []byte(outputStr[xmlStart:])
		// Find the end of XML (look for closing </v2v-inspection> tag first, then fallback to </operatingsystem>)
		xmlEnd := strings.LastIndex(string(xmlData), "</v2v-inspection>")
		if xmlEnd > 0 {
			xmlEnd += len("</v2v-inspection>")
			xmlData = xmlData[:xmlEnd]
		} else {
			// Fallback: look for </operatingsystem> if </v2v-inspection> not found
			xmlEnd = strings.LastIndex(string(xmlData), "</operatingsystem>")
			if xmlEnd > 0 {
				xmlEnd += len("</operatingsystem>")
				xmlData = xmlData[:xmlEnd]
			}
		}
		if i.logger != nil {
			xmlPreview := string(xmlData)
			if len(xmlPreview) > 1000 {
				xmlPreview = xmlPreview[:1000] + "... (truncated)"
			}
			i.logger.WithField("xml_extracted", xmlPreview).Debug("Extracted XML from output")
		}
	} else {
		// No XML found, try parsing the whole output
		xmlData = output
		if i.logger != nil {
			i.logger.Warn("No XML markers found in output, attempting to parse entire output")
		}
	}

	inspectionData, err := ParseV2VInspectionXML(xmlData)
	if err != nil {
		if i.logger != nil {
			i.logger.WithFields(logrus.Fields{
				"error":  err,
				"output": outputStr,
			}).Error("Failed to parse virt-v2v-inspector XML output")
		}
		return nil, fmt.Errorf("failed to parse virt-v2v-inspector output: %w", err)
	}

	i.logger.Info("virt-v2v-inspector snapshot inspection completed successfully")
	return inspectionData, nil
}

// extractHostname extracts hostname from a URL
func extractHostname(urlStr string) string {
	if urlStr == "" {
		return ""
	}

	// Try parsing as URL
	parsedURL, err := url.Parse(urlStr)
	if err == nil && parsedURL.Hostname() != "" {
		return parsedURL.Hostname()
	}

	// If parsing fails, assume it's already a hostname
	return urlStr
}

// findVDDKLibDir finds the VDDK library directory
func findVDDKLibDir() string {
	vddkLibDir := "/opt/vmware-vix-disklib"
	if _, err := os.Stat(vddkLibDir); err == nil {
		return vddkLibDir
	}

	// Try alternative locations
	altPaths := []string{
		"/usr/lib64/vmware-vix-disklib",
		"/usr/local/vmware-vix-disklib",
	}
	for _, altPath := range altPaths {
		if _, err := os.Stat(altPath); err == nil {
			return altPath
		}
	}

	return ""
}

// createPasswordFile creates a temporary file with the password
// virt-v2v-inspector expects -ip to be a file path, not the password directly
func (i *VirtV2vInspector) createPasswordFile(password string) (string, error) {
	tmpFile, err := os.CreateTemp("", "v2v-password-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary password file: %w", err)
	}

	// Write password to file
	if _, err := tmpFile.WriteString(password); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write password to file: %w", err)
	}

	// Close the file
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to close password file: %w", err)
	}

	// Set restrictive permissions (read-only for owner)
	if err := os.Chmod(tmpFile.Name(), 0600); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to set password file permissions: %w", err)
	}

	return tmpFile.Name(), nil
}
