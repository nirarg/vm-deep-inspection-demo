# Getting Started

This guide walks you through setting up and running the VM Deep Inspection Demo service.

## Prerequisites

### Required

- **Go 1.21+** - For building the service
- **VMware vSphere Environment** - vCenter Server with ESXi hosts
- **vSphere Credentials** - Service account with appropriate permissions
- **Container Runtime** - Docker or Podman (for containerized deployment)

### Required (for inspection capabilities)

- **An inspection backend:** VMware VDDK 8.0.3 for the `vddk` backend, or the nbdkit NFC plugin for the `nfc` backend
- **KVM support** - For running libguestfs (Linux host or VM with nested virtualization)

## Quick Start

### 1. Clone the Repository

```bash
git clone https://github.com/nirarg/vm-deep-inspection-demo.git
cd vm-deep-inspection-demo
```

### 2. Configure the Service

Copy the example configuration:

```bash
cp config.example.yaml config.yaml
```

Edit `config.yaml` with your vCenter details:

```yaml
vmware:
  vcenter_url: "https://your-vcenter.example.com/sdk"
  username: "your-service-account"
  password: "your-password"
  insecure_skip_verify: false  # Set to true for self-signed certs

server:
  host: "0.0.0.0"
  port: 8080

logging:
  level: "info"
  format: "json"
  output: "stdout"
```

### 3. Build and Run Locally

```bash
# Install dependencies
make deps

# Generate Swagger documentation
make swagger

# Build the binary
make build

# Run the service
make run-config
```

The service will start on `http://localhost:8080`.

### 4. Verify Installation

Check the health endpoint:

```bash
curl http://localhost:8080/health
```

Expected response:
```json
{
  "status": "healthy",
  "timestamp": "2024-11-12T10:30:00Z",
  "service": "vm-deep-inspection-demo",
  "version": "1.0.0"
}
```

View the Swagger UI:
```
http://localhost:8080/swagger/index.html
```


## Inspection backend selection

The `INSPECTION_BACKEND` environment variable selects the transport used by regular `virt-inspector`, combined `virt-v2v-inspector`, asynchronous V2V-only inspection, and validation checks.

| Value | Behavior |
| --- | --- |
| `auto` (default) | Use VDDK when its library is available; otherwise use NFC when its nbdkit plugin is available. |
| `vddk` | Require VDDK. |
| `nfc` | Force NFC, even when VDDK is installed. |

NFC mode does not require removing VDDK. It selects NFC for all inspection calls and bypasses Detective's persistent result cache, whose keys do not distinguish backends. The demo logs the selected backend at startup; the async V2V status response also reports `inspection_backend_mode`, `inspection_backend`, `vddk_available`, and `nfc_available`.

For a local run, install the NFC plugin at `/usr/lib64/nbdkit/plugins/nbdkit-nfc-plugin.so` or `/opt/nbdkit-nfc-plugin.so`, then verify that nbdkit can load it:

```bash
nbdkit nfc --dump-plugin
```

Run the service with NFC forced:

```bash
INSPECTION_BACKEND=nfc make run-config
```

Omit the variable, or set it to `auto`, to use automatic selection. If both backends are available, `auto` prefers VDDK.

## VDDK Setup (Optional: VDDK Backend)

Follow these steps when you want to use the VDDK backend.

### 1. Download VMware VDDK

1. Visit [VMware VDDK Download Page](https://developer.vmware.com/web/sdk/8.0/vddk)
2. Create a free VMware Developer account (if needed)
3. Download **VDDK 8.0.3 for Linux** (x86_64)
4. Accept the license agreement

### 2. Extract VDDK

```bash
# Extract the downloaded archive
tar -xzf VMware-vix-disklib-8.0.3-*.tar.gz

# Install at the default path used by the service and container
sudo mv vmware-vix-disklib-distrib /opt/vmware-vix-disklib
```

The VDDK libraries should be available at `/opt/vmware-vix-disklib/lib64/`. Set `VDDK_LIB_DIR` if you install them elsewhere. NFC mode does not use this directory.

### 3. Build and run the container

The image includes the NFC plugin as well as the nbdkit VDDK plugin. The default plugin image is `localhost/go-nfc:poc`; make it available to your container runtime before building, or pass another image and its expected SHA256 through `NFC_PLUGIN_IMAGE` and `NFC_PLUGIN_SHA256`.

```bash
# Build the image
make docker-build

# Force NFC for both virt-inspector and V2V inspection
make docker-run INSPECTION_BACKEND=nfc
```

To use VDDK, make sure `/opt/vmware-vix-disklib` contains the VDDK libraries and run `make docker-run INSPECTION_BACKEND=vddk`. The default `auto` mode prefers VDDK when present and falls back to NFC. The container may keep mounting VDDK while running NFC; the selected mode controls the inspection transport.

### 4. Verify VDDK Installation

```bash
# Open shell in container
make docker-shell

# Check VDDK libraries
ls -la /opt/vmware-vix-disklib/lib64/

# Test nbdkit with VDDK plugin
nbdkit vddk --version

# Test virt-inspector
virt-inspector --version

# Or use the test command
make docker-test-vddk

# Exit the container
exit
```

## Container Deployment (VDDK or NFC)

### Using Podman (Recommended)

```bash
# Build the container image (requires the configured NFC plugin image)
make docker-build CONTAINER_RUNTIME=podman

# Run all inspections through NFC, even if VDDK is mounted
make docker-run CONTAINER_RUNTIME=podman INSPECTION_BACKEND=nfc
```

### Using Docker

```bash
# Build the container image (requires the configured NFC plugin image)
make docker-build CONTAINER_RUNTIME=docker

# Run all inspections through NFC, even if VDDK is mounted
make docker-run CONTAINER_RUNTIME=docker INSPECTION_BACKEND=nfc
```

### Verify Container is Running

```bash
# Check container status
podman ps
# or
docker ps

# View logs
make docker-logs

# Test the API
curl http://localhost:8080/health
```

## Testing the Service

### List VMs

```bash
curl http://localhost:8080/api/v1/vms | jq
```

### List VMs - only with name contains specific string

```bash
export CONTAINS_STR=your-unique-string
curl http://localhost:8080/api/v1/vms?name_contains=$CONTAINS_STR | jq
```

### Get Specific VM

```bash
export VM_NAME=your-vm-name
curl http://localhost:8080/api/v1/vms/$VM_NAME | jq
```

### Create Snapshot

```bash
export VM_NAME=your-vm-name
export SNAPSHOT_NAME=new-snapshot-name
curl -X POST "http://localhost:8080/api/v1/vms/snapshot?name=$VM_NAME" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"$SNAPSHOT_NAME\",
    \"description\": \"Test snapshot for inspection\",
    \"memory\": false,
    \"quiesce\": true
  }" | jq
```

### Inspect Snapshot

```bash
curl -X POST "http://localhost:8080/api/v1/vms/inspect-snapshot?vm=your-vm-name&snapshot=test-snapshot" | jq
```

Expected response:
```json
{
  "vm_name": "your-vm-name",
  "snapshot_name": "test-snapshot",
  "status": "success",
  "message": "Inspection completed successfully",
  "data": {
    "operating_system": {
      "name": "linux",
      "distro": "centos",
      "version": "9",
      "architecture": "x86_64",
      "hostname": "test-vm",
      "product": "CentOS Stream 9"
    },
    "applications": [
      {
        "name": "httpd",
        "version": "2.4.57",
        "arch": "x86_64"
      }
    ],
    "filesystems": [
      {
        "device": "/dev/sda1",
        "type": "xfs"
      }
    ]
  }
}
```

## Configuration Reference

### VMware Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `vcenter_url` | vCenter Server SDK URL | Required |
| `username` | vSphere username | Required |
| `password` | vSphere password | Required |
| `insecure_skip_verify` | Skip TLS verification | `false` |
| `connection_timeout` | Connection timeout | `30s` |
| `request_timeout` | Request timeout | `60s` |
| `retry_attempts` | Number of retries | `3` |
| `retry_delay` | Delay between retries | `5s` |

### Server Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `host` | Server bind address | `0.0.0.0` |
| `port` | Server port | `8080` |
| `read_timeout` | HTTP read timeout | `10s` |
| `write_timeout` | HTTP write timeout | `10s` |
| `idle_timeout` | HTTP idle timeout | `60s` |
| `enable_cors` | Enable CORS headers | `true` |

### Logging Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `level` | Log level (debug, info, warn, error) | `info` |
| `format` | Log format (json, text) | `json` |
| `output` | Output destination (stdout, stderr, file) | `stdout` |
| `file_path` | Log file path (when output=file) | - |

## vSphere Permissions

Your service account needs these permissions:

### Required for Basic Operations
- `VirtualMachine.Snapshot.Create` - Create snapshots
- `VirtualMachine.Snapshot.Remove` - Remove snapshots
- `VirtualMachine.Provisioning.Clone` - Clone VMs
- `VirtualMachine.Inventory.Delete` - Delete clones
- `VirtualMachine.Config.Settings` - Read VM configuration

### Required for VDDK Inspection
- `Datastore.Browse` - Browse datastore files
- `VirtualMachine.Config.DiskLease` - Access VM disks via VDDK
- `VirtualMachine.Provisioning.DiskRandomRead` - Read disk blocks

### Setting Permissions in vCenter

1. **Create Custom Role**:
   - vCenter → Administration → Roles
   - Create New Role: "VM Deep Inspection"
   - Assign the permissions listed above

2. **Assign Role to Service Account**:
   - Navigate to your datacenter or cluster
   - Right-click → Add Permission
   - Select your service account
   - Assign the "VM Deep Inspection" role
   - Check "Propagate to children"

### Regenerate Swagger Docs

```bash
# Install swag tool
make install-swag

# Generate docs
make swagger
```

## Troubleshooting

### "Failed to connect to vCenter"

**Check**:
- vCenter URL is correct (include `/sdk`)
- Credentials are valid
- Network connectivity to vCenter
- Firewall allows HTTPS (port 443)

**Test connection**:
```bash
curl -k https://your-vcenter.example.com/sdk
```

### "Permission denied" errors

**Check**:
- Service account has required permissions
- Permissions are propagated to child objects
- User is not locked or disabled

### "VDDK libraries not found"

**Check**:
- VDDK is extracted to `vmware-vix-disklib-distrib/`
- Built with `make docker-build` (which now uses Dockerfile.vddk)
- `LD_LIBRARY_PATH` is set correctly in container

**Verify**:
```bash
make docker-shell
ls -la /opt/vmware-vix-disklib/lib64/
```

### "virt-inspector failed"

**Check**:
- Container has `--privileged` flag
- KVM is available (for libguestfs)
- VM disk is not corrupted
- Supported guest OS (RHEL, CentOS, Ubuntu, etc.)

**Test**:
```bash
make docker-test-vddk
```

### Port 8080 already in use

**Solution**:
```bash
# Stop existing container
make docker-stop

# Or change port in config.yaml
server:
  port: 8081
```

## Stopping the Service

### Local Binary

Press `Ctrl+C` in the terminal running the service.

### Container

```bash
make docker-stop
```

Or manually:
```bash
podman stop vm-inspector
podman rm vm-inspector
```

## Uninstallation

### Remove Binary

```bash
make clean
```

### Remove Container Images

```bash
podman rmi vm-deep-inspection-demo:latest
podman rmi vm-deep-inspection-demo:latest-vddk
```

### Remove Configuration

```bash
rm config.yaml
```

### Asynchronous virt-v2v inspection

The agent-style V2V workflow creates a temporary snapshot for each VM, runs the V2V-only pass through `vm-migration-detective`, persists the result, and removes the snapshot. It uses the selected inspection backend: `auto` prefers VDDK and falls back to NFC, while `INSPECTION_BACKEND=nfc` forces NFC for V2V, regular virt-inspector, and validation checks. The existing `go.mod` replacement points to `../vm-migration-detective`; use a checkout containing the `RunVirtV2v` API from the V2V inspector branch.

Start one or more inspections:

```bash
curl -X POST "http://localhost:8080/api/v1/inspector/v2v" \
  -H "Content-Type: application/json" \
  -d '{"vm_names":["your-vm-name"]}' | jq
```

Poll the latest per-VM state and result:

```bash
curl "http://localhost:8080/api/v1/inspector/v2v/status" | jq
```

Cancel one VM or all queued/running V2V inspections:

```bash
curl -X DELETE "http://localhost:8080/api/v1/inspector/v2v/your-vm-name" | jq
curl -X DELETE "http://localhost:8080/api/v1/inspector/v2v" | jq
```
