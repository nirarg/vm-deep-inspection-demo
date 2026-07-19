package api

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	validationtypes "github.com/kubev2v/vm-migration-detective/pkg/types"
	"github.com/sirupsen/logrus"
)

const virtV2VInspectorTimeout = 30 * time.Minute

// runVirtV2VInspector executes virt-v2v-inspector directly (forklift-style)
// against a VM snapshot via VDDK, and returns parsed XML output.
func runVirtV2VInspector(
	ctx context.Context,
	vmName string,
	vcenterURL string,
	username string,
	password string,
	computeResourcePath string,
	baseDiskPaths []string,
	logger *logrus.Logger,
) (*validationtypes.VirtV2VInspectorXML, error) {
	vcenterHost := extractHostname(vcenterURL)
	encodedUsername := url.QueryEscape(username)

	libvirtURL := fmt.Sprintf("vpx://%s@%s%s?no_verify=1",
		encodedUsername, vcenterHost, computeResourcePath)

	passwordFile, err := createPasswordFile(password)
	if err != nil {
		return nil, fmt.Errorf("failed to create password file: %w", err)
	}
	defer func() { _ = os.Remove(passwordFile) }()

	thumbprint, err := getVCenterThumbprint(vcenterHost)
	if err != nil && logger != nil {
		logger.WithError(err).Warn("Failed to get thumbprint, proceeding without it")
	}

	vddkLibDir := os.Getenv("VDDK_LIB_DIR")
	if vddkLibDir == "" {
		vddkLibDir = "/opt/vmware-vix-disklib"
	}

	// Write XML output to temp file (like forklift) to avoid stdout debug contamination
	outputFile, err := os.CreateTemp("", "v2v-output-*.xml")
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}
	outputFilePath := outputFile.Name()
	_ = outputFile.Close()
	defer func() { _ = os.Remove(outputFilePath) }()

	args := []string{"-v", "-x"}
	args = append(args, "-O", outputFilePath)
	args = append(args, "-i", "libvirt")
	args = append(args, "-ic", libvirtURL)
	args = append(args, "-ip", passwordFile)
	args = append(args, "-it", "vddk")

	if thumbprint != "" {
		args = append(args, "-io", fmt.Sprintf("vddk-thumbprint=%s", thumbprint))
	}
	if vddkLibDir != "" {
		args = append(args, "-io", fmt.Sprintf("vddk-libdir=%s", vddkLibDir))
	}
	for _, diskPath := range baseDiskPaths {
		if diskPath != "" {
			args = append(args, "-io", fmt.Sprintf("vddk-file=%s", diskPath))
		}
	}

	args = append(args, "--", vmName)

	inspectCtx, cancel := context.WithTimeout(ctx, virtV2VInspectorTimeout)
	defer cancel()

	if logger != nil {
		logger.WithField("vm_name", vmName).Info("Running virt-v2v-inspector")
	}

	cmd := exec.CommandContext(inspectCtx, "virt-v2v-inspector", args...)

	// Strip VDDK paths from LD_LIBRARY_PATH
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "LD_LIBRARY_PATH=") {
			val := strings.TrimPrefix(env, "LD_LIBRARY_PATH=")
			var kept []string
			for _, p := range strings.Split(val, ":") {
				if !strings.Contains(p, "vmware-vix-disklib") {
					kept = append(kept, p)
				}
			}
			cmd.Env = append(cmd.Env, "LD_LIBRARY_PATH="+strings.Join(kept, ":"))
		} else {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env, "LIBGUESTFS_DEBUG=1")

	output, err := cmd.CombinedOutput()

	if inspectCtx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("virt-v2v-inspector timed out after %v", virtV2VInspectorTimeout)
	}
	if err != nil {
		if logger != nil {
			logger.WithFields(logrus.Fields{
				"error":  err,
				"output": string(output),
			}).Error("virt-v2v-inspector failed")
		}
		return nil, fmt.Errorf("virt-v2v-inspector failed: %w\nOutput: %s", err, string(output))
	}

	// Read XML from output file (clean, no debug contamination)
	xmlData, err := os.ReadFile(outputFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read virt-v2v-inspector output file: %w", err)
	}

	var result validationtypes.VirtV2VInspectorXML
	if err := xml.Unmarshal(xmlData, &result); err != nil {
		return nil, fmt.Errorf("failed to parse virt-v2v-inspector XML: %w", err)
	}
	return &result, nil
}

func extractHostname(urlStr string) string {
	if urlStr == "" {
		return ""
	}
	parsedURL, err := url.Parse(urlStr)
	if err == nil && parsedURL.Hostname() != "" {
		return parsedURL.Hostname()
	}
	return urlStr
}

func createPasswordFile(password string) (string, error) {
	tmpFile, err := os.CreateTemp("", "v2v-password-*")
	if err != nil {
		return "", err
	}
	if _, err := tmpFile.WriteString(password); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", err
	}
	if err := os.Chmod(tmpFile.Name(), 0600); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", err
	}
	return tmpFile.Name(), nil
}

func getVCenterThumbprint(host string) (string, error) {
	// Try to get thumbprint using openssl
	cmd := exec.Command("bash", "-c",
		fmt.Sprintf("echo | openssl s_client -connect %s:443 2>/dev/null | openssl x509 -noout -fingerprint -sha1 2>/dev/null | sed 's/sha1 Fingerprint=//;s/SHA1 Fingerprint=//'", host))
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	thumbprint := strings.TrimSpace(string(output))
	if thumbprint == "" {
		return "", fmt.Errorf("empty thumbprint")
	}
	return thumbprint, nil
}
