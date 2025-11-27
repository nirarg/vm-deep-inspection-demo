package types

// SnapshotDiskInfo contains VM moref, snapshot moref, and disk path for inspection
// This is used by both vm_service (to retrieve the info) and inspection (to use it)
type SnapshotDiskInfo struct {
	VMMoref       string
	SnapshotMoref string
	DiskPath      string
	BaseDiskPath  string
}

