package inspection

import (
	"context"
	"fmt"
	"time"
	"os/exec"

	apitypes "github.com/nirarg/vm-deep-inspection-demo/pkg/types"
	"github.com/sirupsen/logrus"
)

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

// Inspect uses nbdkit-vddk plugin to inspect a VM snapshot directly
func (i *VirtInspector) Inspect(
	ctx context.Context,
	vmName string,
	snapshotName string,
	vcenterURL string,
	datacenter string,
	username string,
	password string,
) (*apitypes.InspectionData, error) {

	i.logger.WithFields(logrus.Fields{
		"vm_name":       vmName,
		"snapshot_name": snapshotName,
		"vcenter_url":   vcenterURL,
		"datacenter":    datacenter,
	}).Info("Running virt-inspector using virt-v2v-open (VDDK + snapshot)")

	openCtx, cancel := context.WithTimeout(ctx, i.timeout)
	defer cancel()

	// ✅ Step 1: Open remote disk via virt-v2v-open
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
	defer v2vSession.Close()

	// Give NBD time to initialize
	time.Sleep(4 * time.Second)

	// ✅ Step 2: Run virt-inspector on NBD
	inspectCtx, cancel := context.WithTimeout(ctx, i.timeout)
	defer cancel()

	i.logger.WithField("nbd_url", v2vSession.NBDURL).Info("Running virt-inspector on NBD")

	cmdString := fmt.Sprintf(
		"unset LD_LIBRARY_PATH && %s --format=raw -a %s",
		i.virtInspectorPath,
		v2vSession.NBDURL,
	)

	virtInspectorCmd := exec.CommandContext(inspectCtx, "sh", "-c", cmdString)

	output, err := virtInspectorCmd.CombinedOutput()
	if err != nil {
		i.logger.WithField("output", string(output)).Error("virt-inspector failed")
		return nil, fmt.Errorf("virt-inspector failed: %w", err)
	}

	inspectionData, err := ParseInspectionXML(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse inspection output: %w", err)
	}

	i.logger.Info("virt-v2v-open snapshot inspection completed successfully")
	return inspectionData, nil
}
