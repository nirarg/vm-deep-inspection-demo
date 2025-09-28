package types

// VMInspectionRequest represents a request to inspect a VM snapshot
type VMInspectionRequest struct {
	SnapshotName string `json:"snapshot_name" binding:"required" example:"backup-snapshot"`
}

// CloneRequest represents a request to create a clone from snapshot
type CloneRequest struct {
	SnapshotName string `json:"snapshot_name" binding:"required" example:"backup-snapshot"`
	CloneName    string `json:"clone_name,omitempty" example:"my-clone"`
}

// CloneResponse represents the response from clone creation
type CloneResponse struct {
	CloneName    string `json:"clone_name" example:"vm-clone-123"`
	VMName       string `json:"vm_name" example:"web-server-01"`
	SnapshotName string `json:"snapshot_name" example:"backup-snapshot"`
	Status       string `json:"status" example:"completed"`
	Message      string `json:"message" example:"Clone created successfully"`
}

// VMInspectionResponse represents the response from VM inspection
type VMInspectionResponse struct {
	VMName       string          `json:"vm_name" example:"web-server-01"`
	SnapshotName string          `json:"snapshot_name" example:"backup-snapshot"`
	Status       string          `json:"status" example:"completed"`
	Message      string          `json:"message" example:"Inspection completed successfully"`
	Data         *InspectionData `json:"data,omitempty"`
}

// InspectionData contains the parsed inspection results
type InspectionData struct {
	OperatingSystem *OSInfo          `json:"operating_system,omitempty"`
	Applications    []Application    `json:"applications,omitempty"`
	Filesystems     []Filesystem     `json:"filesystems,omitempty"`
	Mountpoints     []Mountpoint     `json:"mountpoints,omitempty"`
	Drives          []Drive          `json:"drives,omitempty"`
}

// OSInfo contains operating system information
type OSInfo struct {
	Name              string `json:"name" example:"linux"`
	Distro            string `json:"distro" example:"ubuntu"`
	Version           string `json:"version" example:"22.04"`
	Architecture      string `json:"architecture" example:"x86_64"`
	Hostname          string `json:"hostname,omitempty" example:"web-server-01"`
	Product           string `json:"product,omitempty" example:"Ubuntu 22.04 LTS"`
	Root              string `json:"root,omitempty" example:"/dev/sda1"`
	PackageFormat     string `json:"package_format,omitempty" example:"rpm"`
	PackageManagement string `json:"package_management,omitempty" example:"dnf"`
	OSInfo            string `json:"osinfo,omitempty" example:"centos9"`
}

// Application represents an installed application/package
type Application struct {
	Name        string `json:"name" example:"nginx"`
	Version     string `json:"version,omitempty" example:"1.18.0"`
	Epoch       int    `json:"epoch,omitempty"`
	Release     string `json:"release,omitempty"`
	Arch        string `json:"arch,omitempty" example:"amd64"`
	URL         string `json:"url,omitempty" example:"https://nginx.org"`
	Summary     string `json:"summary,omitempty" example:"High performance web server"`
	Description string `json:"description,omitempty" example:"nginx is a web server with a strong focus on high concurrency"`
}

// Filesystem represents a filesystem on the VM
type Filesystem struct {
	Device string `json:"device" example:"/dev/sda1"`
	Type   string `json:"type" example:"ext4"`
	UUID   string `json:"uuid,omitempty" example:"ac818163-71e7-4b23-a1b7-e94196b9dada"`
	Size   int64  `json:"size,omitempty"`
	Used   int64  `json:"used,omitempty"`
}

// Drive represents a drive/disk on the VM
type Drive struct {
	Name string `json:"name" example:"/dev/sda"`
	Size int64  `json:"size,omitempty"`
	Type string `json:"type,omitempty" example:"disk"`
}

// Mountpoint represents a mounted filesystem
type Mountpoint struct {
	Device     string `json:"device" example:"/dev/sda1"`
	MountPoint string `json:"mount_point" example:"/boot"`
}
