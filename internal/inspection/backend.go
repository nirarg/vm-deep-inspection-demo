package inspection

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	BackendAuto = "auto"
	BackendVDDK = "vddk"
	BackendNFC  = "nfc"
)

// BackendStatus reports the requested mode, selected transport, and installed backends.
type BackendStatus struct {
	Mode          string `json:"inspection_backend_mode"`
	Selected      string `json:"inspection_backend"`
	VDDKAvailable bool   `json:"vddk_available"`
	NFCAvailable  bool   `json:"nfc_available"`
}

func (s BackendStatus) Available() bool {
	switch s.Selected {
	case BackendVDDK:
		return s.VDDKAvailable
	case BackendNFC:
		return s.NFCAvailable
	default:
		return false
	}
}

// ResolveInspectionBackend selects one global inspection transport. Auto prefers VDDK;
// explicit vddk/nfc modes retain the selection even when unavailable so callers can
// report the requested backend accurately and fail with a useful error.
func ResolveInspectionBackend(mode, vddkLibDir string) (BackendStatus, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = BackendAuto
	}
	if mode != BackendAuto && mode != BackendVDDK && mode != BackendNFC {
		return BackendStatus{}, fmt.Errorf("unsupported INSPECTION_BACKEND %q (valid values: auto, vddk, nfc)", mode)
	}

	status := BackendStatus{
		Mode:          mode,
		VDDKAvailable: vddkLibraryAvailable(vddkLibDir),
		NFCAvailable:  nfcPluginAvailable(),
	}
	switch mode {
	case BackendAuto:
		if status.VDDKAvailable {
			status.Selected = BackendVDDK
		} else if status.NFCAvailable {
			status.Selected = BackendNFC
		}
	case BackendVDDK:
		status.Selected = BackendVDDK
	case BackendNFC:
		status.Selected = BackendNFC
	}
	return status, nil
}

// DisabledVDDKLibDir returns a unique path that does not exist. Detective uses the
// configured VDDK directory's existence to choose its nbdkit plugin, so this lets
// NFC mode force NFC without removing an installed VDDK directory.
func DisabledVDDKLibDir() (string, error) {
	path, err := os.MkdirTemp("", "vm-inspector-vddk-disabled-")
	if err != nil {
		return "", fmt.Errorf("create temporary VDDK-disabled path: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("remove temporary VDDK-disabled path: %w", err)
	}
	return path, nil
}

func vddkLibraryAvailable(libraryDir string) bool {
	candidates := []string{
		filepath.Join(libraryDir, "lib64", "libvixDiskLib.so*"),
		filepath.Join(libraryDir, "libvixDiskLib.so*"),
	}
	for _, pattern := range candidates {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, match := range matches {
			info, err := os.Stat(match)
			if err == nil && !info.IsDir() {
				return true
			}
		}
	}
	return false
}

func nfcPluginAvailable() bool {
	for _, path := range []string{
		"/opt/nbdkit-nfc-plugin.so",
		"/usr/lib64/nbdkit/plugins/nbdkit-nfc-plugin.so",
	} {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}
