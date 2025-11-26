package inspection

import (
	"encoding/xml"
	"fmt"

	apitypes "github.com/nirarg/vm-deep-inspection-demo/pkg/types"
)

// ParseInspectionXML parses virt-inspector XML output
func ParseInspectionXML(xmlData []byte) (*apitypes.InspectionData, error) {
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
