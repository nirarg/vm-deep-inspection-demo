# Build stage
FROM golang:1.24-alpine AS builder

# Install ca-certificates and git
RUN apk add --no-cache ca-certificates git

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/server

# Runtime stage with libguestfs support
FROM fedora:39

# Install libguestfs, virt-inspector, and dependencies
RUN dnf install -y \
    libguestfs \
    libguestfs-tools \
    libguestfs-tools-c \
    qemu-kvm \
    ca-certificates \
    curl \
    && dnf clean all

# Set libguestfs backend to direct (no libvirt needed)
ENV LIBGUESTFS_BACKEND=direct

# Create non-root user
RUN useradd -m -u 1001 -s /bin/bash appuser

# Create necessary directories
RUN mkdir -p /var/tmp/guestfs /app && \
    chown -R appuser:appuser /var/tmp/guestfs /app

WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/main ./vm-inspector

# Change ownership to non-root user
RUN chown appuser:appuser /app/vm-inspector && \
    chmod +x /app/vm-inspector

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Run the application
CMD ["./vm-inspector", "-config", "/etc/vm-inspector/config.yaml"]