package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/nirarg/vm-deep-inspection-demo/internal/inspection"
	"github.com/nirarg/vm-deep-inspection-demo/pkg/types"
	"github.com/sirupsen/logrus"
)

// V2VHandler exposes the asynchronous V2V inspection workflow.
type V2VHandler struct {
	service *inspection.V2VService
	logger  *logrus.Logger
}

func NewV2VHandler(service *inspection.V2VService, logger *logrus.Logger) *V2VHandler {
	return &V2VHandler{service: service, logger: logger}
}

type startV2VRequest struct {
	VMNames []string `json:"vm_names" binding:"required"`
}

// Start godoc
// @Summary Start asynchronous virt-v2v inspection
// @Description Queue V2V-only inspection jobs through the selected VDDK or NFC backend. Each job creates and removes a temporary vSphere snapshot.
// @Tags inspector
// @Accept json
// @Produce json
// @Param request body startV2VRequest true "VM names to inspect"
// @Success 202 {object} map[string]interface{} "Jobs accepted"
// @Failure 400 {object} types.ErrorResponse "Invalid request"
// @Failure 409 {object} types.ErrorResponse "A VM already has an active job or the queue is full"
// @Failure 503 {object} types.ErrorResponse "Selected inspection backend is unavailable"
// @Router /api/v1/inspector/v2v [post]
func (h *V2VHandler) Start(c *gin.Context) {
	var request startV2VRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "Invalid request body", Code: "INVALID_REQUEST", Details: err.Error()})
		return
	}
	statuses, err := h.service.Start(c.Request.Context(), request.VMNames)
	if err != nil {
		switch {
		case errors.Is(err, inspection.ErrV2VNoVMs):
			c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: err.Error(), Code: "VMS_REQUIRED"})
		case errors.Is(err, inspection.ErrV2VAlreadyRunning), errors.Is(err, inspection.ErrV2VQueueFull):
			c.JSON(http.StatusConflict, types.ErrorResponse{Error: err.Error(), Code: "V2V_JOB_CONFLICT"})
		case errors.Is(err, inspection.ErrV2VBackendUnavailable):
			c.JSON(http.StatusServiceUnavailable, types.ErrorResponse{Error: err.Error(), Code: "INSPECTION_BACKEND_UNAVAILABLE", Details: "Check INSPECTION_BACKEND, VDDK_LIB_DIR, and that the NFC plugin is installed when NFC is selected."})
		default:
			h.logger.WithError(err).Error("failed to start V2V inspection")
			c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "Failed to start V2V inspection", Code: "V2V_START_FAILED", Details: err.Error()})
		}
		return
	}
	c.JSON(http.StatusAccepted, v2vStatusPayload(h.service, statuses))
}

// Status godoc
// @Summary Get V2V inspection status
// @Description Return the latest persisted V2V job state for each VM.
// @Tags inspector
// @Produce json
// @Success 200 {object} map[string]interface{} "V2V status"
// @Failure 500 {object} types.ErrorResponse "Status query failed"
// @Router /api/v1/inspector/v2v/status [get]
func (h *V2VHandler) Status(c *gin.Context) {
	statuses, err := h.service.Statuses(c.Request.Context())
	if err != nil {
		h.logger.WithError(err).Error("failed to read V2V inspection statuses")
		c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "Failed to read V2V inspection statuses", Code: "V2V_STATUS_FAILED", Details: err.Error()})
		return
	}
	c.JSON(http.StatusOK, v2vStatusPayload(h.service, statuses))
}

// Cancel godoc
// @Summary Cancel V2V inspection for one VM
// @Tags inspector
// @Produce json
// @Param vmName path string true "VM name"
// @Success 202 {object} map[string]interface{} "Cancellation requested"
// @Failure 404 {object} types.ErrorResponse "No active job"
// @Router /api/v1/inspector/v2v/{vmName} [delete]
func (h *V2VHandler) Cancel(c *gin.Context) {
	vmName := strings.TrimSpace(c.Param("vmName"))
	if vmName == "" {
		c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "VM name is required", Code: "VM_NAME_REQUIRED"})
		return
	}
	if err := h.service.Cancel(vmName); err != nil {
		c.JSON(http.StatusNotFound, types.ErrorResponse{Error: err.Error(), Code: "V2V_JOB_NOT_FOUND"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"vm_name": vmName, "status": "cancellation_requested"})
}

// CancelAll godoc
// @Summary Cancel all queued and running V2V inspections
// @Tags inspector
// @Produce json
// @Success 202 {object} map[string]interface{} "Cancellation requested"
// @Router /api/v1/inspector/v2v [delete]
func (h *V2VHandler) CancelAll(c *gin.Context) {
	count := h.service.CancelAll()
	c.JSON(http.StatusAccepted, gin.H{"cancelled_jobs": count, "status": "cancellation_requested"})
}

func v2vStatusPayload(service *inspection.V2VService, statuses []inspection.V2VStatus) gin.H {
	backend := service.BackendStatus()
	return gin.H{
		"inspection_backend_mode":      backend.Mode,
		"inspection_backend":           backend.Selected,
		"inspection_backend_available": backend.Available(),
		"vddk_available":               backend.VDDKAvailable,
		"nfc_available":                backend.NFCAvailable,
		"jobs":                         statuses,
	}
}
