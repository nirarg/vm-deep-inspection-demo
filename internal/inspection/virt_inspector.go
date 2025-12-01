package inspection

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/nirarg/vm-deep-inspection-demo/internal/types"
	apitypes "github.com/nirarg/vm-deep-inspection-demo/pkg/types"
	"github.com/sirupsen/logrus"
)

// UseVirtV2VOpen controls whether to use virt-v2v-open (true) or nbdkit directly (false)
// Default is false (use nbdkit directly)
const UseVirtV2VOpen = false

// Inspector handles VM inspection operations
type VirtInspector struct {
	virtInspectorPath string
	timeout           time.Duration
	logger            *logrus.Logger
}

// NewInspector creates a new Inspector instance
func NewVirtInspector(virtInspectorPath string, timeout time.Duration, logger *logrus.Logger) *VirtInspector {
	if virtInspectorPath == "" {
		virtInspectorPath = "virt-inspector" // Use system PATH
	}
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &VirtInspector{
		virtInspectorPath: virtInspectorPath,
		timeout:           timeout,
		logger:            logger,
	}
}

func (i *VirtInspector) Inspect(
	ctx context.Context,
	vmName string,
	snapshotName string,
	vcenterURL string,
	datacenter string,
	username string,
	password string,
	diskInfo *types.SnapshotDiskInfo, // Snapshot disk info from vm_service
) (*apitypes.InspectionData, error) {

	var nbdURL string
	var sessionCloser func()

	if UseVirtV2VOpen {
		i.logger.WithFields(logrus.Fields{
			"vm_name":       vmName,
			"snapshot_name": snapshotName,
			"vcenter_url":   vcenterURL,
			"datacenter":    datacenter,
		}).Info("Running virt-inspector using virt-v2v-open (VDDK + snapshot)")

		openCtx, cancel := context.WithTimeout(ctx, i.timeout)
		defer cancel()

		v2vSession, err := OpenWithVirtV2V(
			openCtx,
			vmName,
			datacenter,
			snapshotName,
			vcenterURL,
			username,
			password,
		)
		if err != nil {
			return nil, err
		}
		nbdURL = v2vSession.NBDURL
		sessionCloser = v2vSession.Close

		// Give NBD time to initialize
		time.Sleep(4 * time.Second)
	} else {
		i.logger.WithFields(logrus.Fields{
			"vm_name":       vmName,
			"snapshot_name": snapshotName,
			"vcenter_url":   vcenterURL,
			"datacenter":    datacenter,
		}).Info("Running virt-inspector using nbdkit-vddk (VDDK + snapshot)")

		// Use diskInfo passed from vm_service (no need to query vSphere here)
		i.logger.WithFields(logrus.Fields{
			"vm_moref":       diskInfo.VMMoref,
			"snapshot_moref": diskInfo.SnapshotMoref,
			"disk_path":      diskInfo.DiskPath,
			"base_disk_path": diskInfo.BaseDiskPath,
		}).Debug("Using snapshot disk info from vm_service")

		openCtx, cancel := context.WithTimeout(ctx, i.timeout)
		defer cancel()

		nbdkitSession, err := OpenWithNBDKitVDDK(
			openCtx,
			diskInfo.VMMoref,
			diskInfo.SnapshotMoref,
			diskInfo.BaseDiskPath,
			vcenterURL,
			username,
			password,
			i.logger,
		)
		if err != nil {
			return nil, err
		}
		nbdURL = nbdkitSession.NBDURL
		sessionCloser = nbdkitSession.Close

		// Wait for NBD server to be ready (more reliable than sleep)
		if err := nbdkitSession.WaitForReady(30 * time.Second); err != nil {
			i.logger.WithError(err).Error("NBD server not ready")
			nbdkitSession.Close()
			return nil, fmt.Errorf("NBD server not ready: %w", err)
		}
	}
	defer sessionCloser()

	inspectCtx, cancel := context.WithTimeout(ctx, i.timeout)
	defer cancel()

	i.logger.WithField("nbd_url", nbdURL).Info("Running virt-inspector on NBD")

	cmdString := fmt.Sprintf("unset LD_LIBRARY_PATH && %s --format=raw -a '%s'",
		i.virtInspectorPath, nbdURL)

	virtInspectorCmd := exec.CommandContext(inspectCtx, "sh", "-c", cmdString)

	output, err := virtInspectorCmd.CombinedOutput()
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
			"nbd_url":   nbdURL,
			"command":   cmdString,
		}).Error("virt-inspector failed")

		// Include output in error message for better debugging
		if outputStr != "" {
			return nil, fmt.Errorf("virt-inspector failed (exit code %d): %w\nOutput: %s", exitCode, err, outputStr)
		}
		return nil, fmt.Errorf("virt-inspector failed (exit code %d): %w", exitCode, err)
	}

	inspectionData, err := ParseInspectionXML(output)
	if err != nil {
		if i.logger != nil {
			i.logger.WithFields(logrus.Fields{
				"error":  err,
				"output": outputStr,
			}).Error("Failed to parse virt-inspector XML output")
		}
		return nil, fmt.Errorf("failed to parse inspection output: %w", err)
	}

	if UseVirtV2VOpen {
		i.logger.Info("virt-v2v-open snapshot inspection completed successfully")
	} else {
		i.logger.Info("nbdkit-vddk snapshot inspection completed successfully")
	}
	return inspectionData, nil
}
