# VM Deep Inspection Demo

A Go service that provides **deep inspection** of VMware vSphere virtual machines, going beyond what the standard VMware API offers. Extract detailed OS information, installed packages, and filesystem data from VM snapshots without requiring agents or powered-on VMs.

## Overview

While the VMware vSphere API provides basic VM metadata (CPU, memory, OS name), this service performs **disk-level inspection** using libguestfs and VMware VDDK to extract:

- ✅ Complete package inventory (RPM/DEB packages with versions)
- ✅ Exact OS and kernel versions
- ✅ Filesystem and partition details
- ✅ Installed applications and services
- ✅ Security analysis capabilities

**Key Benefits:**
- 🔒 **Agentless** - No software required inside VMs
- 🛡️ **Safe** - Operates on snapshots, zero impact on production
- 📦 **Comprehensive** - Full disk-level visibility
- 🚀 **API-Driven** - RESTful API for easy integration

## Documentation

- **[Introduction](./docs/INTRO.md)** - Project goals, use cases, and how it works
- **[Getting Started](./docs/GETTING-STARTED.md)** - Installation and setup instructions
- **[Technical Limitations](./docs/LIMITATIONS.md)** - VDDK and KVM requirements
