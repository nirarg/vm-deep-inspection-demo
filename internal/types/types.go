package types

// SnapshotDiskInfo contains VM moref, snapshot moref, disk path, and compute resource path for inspection
// This is used by both vm_service (to retrieve the info) and inspection (to use it)
type SnapshotDiskInfo struct {
	VMMoref       string
	SnapshotMoref string
	DiskPath      string
	BaseDiskPath  string
	ComputeResourcePath string // Path to compute resource (host/cluster) for vpx:// URL (e.g., "/Datacenter/Cluster/host.example.com")
}


