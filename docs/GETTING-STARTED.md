# Getting Started

This guide walks you through setting up and running the VM Deep Inspection Demo service.

## Prerequisites

### Required

- **Go 1.21+** - For building the service
- **VMware vSphere Environment** - vCenter Server with ESXi hosts
- **vSphere Credentials** - Service account with appropriate permissions
- **Container Runtime** - Docker or Podman (for containerized deployment)

### Required (for inspection capabilities)

- **VMware VDDK 8.0.3** - Required for deep disk inspection
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


## VDDK Setup (Required for Inspection)

VDDK is required for all inspection operations. Follow these steps to set it up.

### 1. Download VMware VDDK

1. Visit [VMware VDDK Download Page](https://developer.vmware.com/web/sdk/8.0/vddk)
2. Create a free VMware Developer account (if needed)
3. Download **VDDK 8.0.3 for Linux** (x86_64)
4. Accept the license agreement

### 2. Extract VDDK

```bash
# Extract the downloaded archive
tar -xzf VMware-vix-disklib-8.0.3-*.tar.gz

# Move to project directory
mv vmware-vix-disklib-distrib /path/to/vm-deep-inspection-demo/
```

Your directory structure should look like:
```
vm-deep-inspection-demo/
├── vmware-vix-disklib-distrib/
│   ├── lib64/
│   ├── bin64/
│   └── ...
├── Dockerfile.vddk
├── cmd/
├── internal/
└── ...
```

### 3. Build Container with VDDK

```bash
# Build image with VDDK support
make docker-build

# Run container with VDDK
make docker-run
```

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

## Container Deployment (VDDK Required)

### Using Podman (Recommended)

```bash
# Build the container image (requires VDDK)
make docker-build CONTAINER_RUNTIME=podman

# Run the container
make docker-run CONTAINER_RUNTIME=podman
```

### Using Docker

```bash
# Build the container image (requires VDDK)
make docker-build CONTAINER_RUNTIME=docker

# Run the container
make docker-run CONTAINER_RUNTIME=docker
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
