# Technical Limitations and Requirements

This document details the technical requirements, limitations, and constraints of the VM Deep Inspection Demo service.

## VMware VDDK Requirements

### What is VDDK?

**VMware Virtual Disk Development Kit (VDDK)** is a proprietary VMware library that provides low-level access to virtual machine disks. It's required for deep inspection capabilities.

### Licensing Constraints

#### ✅ What's Allowed
- **Free download** - No cost from VMware
- **Development use** - Testing and proof-of-concept
- **Internal deployment** - Use within your organization
- **Production use** - Running in enterprise environments

#### ❌ What's NOT Allowed
- **Public redistribution** - Cannot share VDDK files publicly
- **Bundling in open-source projects** - Cannot include in public GitHub releases
- **Automated downloads** - Must manually download from VMware
- **License bypass** - Each user must accept VMware EULA

### Version Compatibility

VDDK versions must match vSphere environment:

| vSphere Version | Compatible VDDK | Download Link |
|-----------------|-----------------|---------------|
| vSphere 8.x | VDDK 8.0.x | [vddk-8.0](https://developer.vmware.com/web/sdk/8.0/vddk) |
| vSphere 7.x | VDDK 7.0.x or 8.0.x | [vddk-7.0](https://developer.vmware.com/web/sdk/7.0/vddk) |
| vSphere 6.7 | VDDK 6.7.x or 7.0.x | [vddk-6.7](https://developer.vmware.com/web/sdk/6.7/vddk) |

### VDDK Dependencies

**Library Requirements:**
```bash
# VDDK requires these system libraries
libssl.so.1.1      # OpenSSL 1.1
libcrypto.so.1.1   # Crypto library
libstdc++.so.6     # C++ standard library
libgcc_s.so.1      # GCC runtime
```

**Included in Fedora base image:**
```bash
dnf install -y openssl-libs libstdc++ libgcc
```

**Size Impact:**
- VDDK download: ~50 MB compressed
- VDDK extracted: ~150 MB
- Container image increase: ~100 MB

### Manual Download Process

**Cannot be automated** - Must be performed manually:

1. Visit VMware Developer Portal
2. Create or log in to VMware account
3. Navigate to VDDK download page
4. Read and accept EULA
5. Download appropriate version
6. Extract to build directory

**Time required:** 15-30 minutes (first time)

## KVM and Virtualization Requirements

### Why KVM is Required

**libguestfs** (used by virt-inspector) requires access to virtualization capabilities to safely mount and inspect VM disk images.

### KVM Dependency

```
libguestfs → qemu-kvm → /dev/kvm → Linux KVM module
```

Without KVM, libguestfs runs in **direct mode** which:
- ✅ Works but has limitations
- ⚠️ Less secure (no isolation)
- ⚠️ Potential filesystem corruption if interrupted
- ❌ Some features may not work

### Deployment Scenarios

#### Scenario 1: Linux Physical Host
```
✅ Best Performance
- Native KVM support
- Full hardware virtualization
- No overhead

Requirements:
- Linux kernel with KVM module
- CPU with virtualization extensions (Intel VT-x or AMD-V)
- /dev/kvm device accessible
```

Check KVM availability:
```bash
# Check if KVM module is loaded
lsmod | grep kvm

# Check CPU virtualization support
egrep -c '(vmx|svm)' /proc/cpuinfo
# Output > 0 means supported

# Verify /dev/kvm exists
ls -la /dev/kvm
```

#### Scenario 2: Linux VM with Nested Virtualization
```
⚠️ Requires Nested Virtualization Enabled

VMware ESXi:
1. Power off the VM
2. Edit VM settings
3. CPU → Hardware virtualization → Expose to guest OS ✓
4. Power on the VM

Verify:
cat /sys/module/kvm_intel/parameters/nested
# Output: Y (enabled)
```

#### Scenario 3: Container on Physical Linux Host
```
✅ Recommended Approach

Docker/Podman run requirements:
docker run --privileged --device /dev/kvm:/dev/kvm ...
podman run --privileged --device /dev/kvm:/dev/kvm ...

The container inherits KVM from host.
```

#### Scenario 4: macOS or Windows Host
```
❌ KVM Not Available

Options:
1. Run Linux VM with nested virtualization
2. Deploy to Linux server/cloud
3. Use remote container registry + Linux deployment
```

### Privileged Container Requirement

libguestfs with KVM requires **privileged mode**:

```bash
# Required flags
docker run --privileged --device /dev/kvm:/dev/kvm ...
```

**Security implications:**
- Container has elevated privileges
- Can access host devices
- Should only run trusted code
- Deploy in controlled environment

**Mitigation strategies:**
1. Run in isolated network segment
2. Use dedicated VM/host for inspection
3. Apply strict firewall rules
4. Audit container images regularly
5. Limit network exposure

### Alternative: Direct Backend (No KVM)

Can run without KVM using direct backend:

```dockerfile
ENV LIBGUESTFS_BACKEND=direct
```

**Limitations:**
- ⚠️ Less safe (no process isolation)
- ⚠️ Risk of filesystem corruption
- ⚠️ Must be run as root or with sudo
- ⚠️ Cannot handle some disk formats
- ⚠️ Slower performance

**When to use:**
- Testing/development only
- Cannot enable KVM in environment
- Understand and accept risks
