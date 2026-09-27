package inspection

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kubev2v/vm-migration-detective/pkg/checks"
	"github.com/kubev2v/vm-migration-detective/pkg/vmdetect"
	"github.com/sirupsen/logrus"
)

const (
	V2VStateQueued    = "queued"
	V2VStateRunning   = "running"
	V2VStateCompleted = "completed"
	V2VStateFailed    = "failed"
	V2VStateCancelled = "cancelled"

	v2vInspectionTimeout      = 30 * time.Minute
	v2vSnapshotCleanupTimeout = 2 * time.Minute
	v2vQueueSize              = 1024
)

var (
	ErrV2VNoVMs              = errors.New("at least one VM name is required")
	ErrV2VBackendUnavailable = errors.New("no selected inspection backend is available")
	ErrV2VVDDKUnavailable    = ErrV2VBackendUnavailable // Deprecated: use ErrV2VBackendUnavailable.
	ErrV2VAlreadyRunning     = errors.New("V2V inspection is already queued or running for a VM")
	ErrV2VQueueFull          = errors.New("V2V inspection queue is full")
	ErrV2VServiceClosed      = errors.New("V2V inspection service is shutting down")
	ErrV2VNotRunning         = errors.New("no queued or running V2V inspection for VM")
)

// V2VStatus is the latest asynchronous V2V inspection status for a VM.
type V2VStatus struct {
	VMName       string           `json:"vm_name"`
	State        string           `json:"state"`
	Details      string           `json:"details,omitempty"`
	SnapshotName string           `json:"snapshot_name,omitempty"`
	SnapshotID   string           `json:"snapshot_id,omitempty"`
	Result       *vmdetect.OSInfo `json:"result,omitempty"`
	Concerns     []checks.Concern `json:"concerns,omitempty"`
	Error        string           `json:"error,omitempty"`
	StartedAt    *time.Time       `json:"started_at,omitempty"`
	CompletedAt  *time.Time       `json:"completed_at,omitempty"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

// V2VRepository persists current job status so clients can poll it after requests return.
type V2VRepository interface {
	SaveV2VStatus(context.Context, V2VStatus) error
	ListV2VStatuses(context.Context) ([]V2VStatus, error)
}

// V2VSnapshotOperator creates and removes temporary snapshots for V2V inspection.
type V2VSnapshotOperator interface {
	CreateV2VInspectionSnapshot(context.Context, string, string) (vmMoref, snapshotMoref string, err error)
	RemoveV2VInspectionSnapshot(context.Context, string, string) error
}

// V2VDetector is the Detective API used for the V2V-only pass.
type V2VDetector interface {
	Detect(vmdetect.DetectParams, ...checks.CheckType) (*vmdetect.DetectResult, error)
}

type v2vJob struct {
	status V2VStatus
	ctx    context.Context
	cancel context.CancelFunc
}

// V2VService runs V2V inspections sequentially, matching the agent's single-worker flow.
type V2VService struct {
	vm            V2VSnapshotOperator
	detector      V2VDetector
	repository    V2VRepository
	backendStatus func() BackendStatus
	logger        *logrus.Logger
	jobs          chan v2vJob
	workerDone    chan struct{}
	shutdown      chan struct{}

	mu              sync.Mutex
	active          map[string]context.CancelFunc
	closed          bool
	snapshotCounter atomic.Uint64
}

func NewV2VService(vm V2VSnapshotOperator, detector V2VDetector, repository V2VRepository, backendStatus func() BackendStatus, logger *logrus.Logger) *V2VService {
	if logger == nil {
		logger = logrus.New()
	}
	service := &V2VService{
		vm:            vm,
		detector:      detector,
		repository:    repository,
		backendStatus: backendStatus,
		logger:        logger,
		jobs:          make(chan v2vJob, v2vQueueSize),
		workerDone:    make(chan struct{}),
		shutdown:      make(chan struct{}),
		active:        make(map[string]context.CancelFunc),
	}
	go service.runWorker()
	return service
}

func (s *V2VService) BackendStatus() BackendStatus {
	if s.backendStatus == nil {
		return BackendStatus{Mode: BackendAuto}
	}
	return s.backendStatus()
}

func (s *V2VService) BackendAvailable() bool {
	return s.BackendStatus().Available()
}

// VDDKAvailable reports physical VDDK availability for compatibility with existing callers.
func (s *V2VService) VDDKAvailable() bool {
	return s.BackendStatus().VDDKAvailable
}

// Start queues one V2V job per VM name. The request context is checked before accepting,
// then jobs run independently of the HTTP connection and can be cancelled explicitly.
func (s *V2VService) Start(ctx context.Context, vmNames []string) ([]V2VStatus, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(vmNames) == 0 {
		return nil, ErrV2VNoVMs
	}
	if !s.BackendAvailable() {
		return nil, ErrV2VBackendUnavailable
	}

	normalized := make([]string, 0, len(vmNames))
	seen := make(map[string]struct{}, len(vmNames))
	for _, vmName := range vmNames {
		vmName = strings.TrimSpace(vmName)
		if vmName == "" {
			continue
		}
		if _, exists := seen[vmName]; exists {
			continue
		}
		seen[vmName] = struct{}{}
		normalized = append(normalized, vmName)
	}
	if len(normalized) == 0 {
		return nil, ErrV2VNoVMs
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrV2VServiceClosed
	}
	if len(s.jobs)+len(normalized) > cap(s.jobs) {
		return nil, ErrV2VQueueFull
	}
	for _, vmName := range normalized {
		if _, exists := s.active[vmName]; exists {
			return nil, fmt.Errorf("%w: %s", ErrV2VAlreadyRunning, vmName)
		}
	}

	statuses := make([]V2VStatus, 0, len(normalized))
	jobs := make([]v2vJob, 0, len(normalized))
	for _, vmName := range normalized {
		jobCtx, cancel := context.WithTimeout(context.Background(), v2vInspectionTimeout)
		status := V2VStatus{
			VMName:       vmName,
			State:        V2VStateQueued,
			Details:      "waiting for the V2V inspection worker",
			SnapshotName: s.newSnapshotName(),
			UpdatedAt:    time.Now().UTC(),
		}
		if err := s.repository.SaveV2VStatus(context.Background(), status); err != nil {
			cancel()
			for _, job := range jobs {
				job.cancel()
			}
			for _, persisted := range statuses {
				completed := time.Now().UTC()
				persisted.State = V2VStateFailed
				persisted.Details = "failed to queue V2V inspection"
				persisted.Error = err.Error()
				persisted.CompletedAt = &completed
				persisted.UpdatedAt = completed
				if saveErr := s.repository.SaveV2VStatus(context.Background(), persisted); saveErr != nil {
					s.logger.WithError(saveErr).WithField("vm_name", persisted.VMName).Error("failed to persist V2V queue failure")
				}
			}
			return nil, fmt.Errorf("save queued V2V status for %q: %w", vmName, err)
		}
		jobs = append(jobs, v2vJob{status: status, ctx: jobCtx, cancel: cancel})
		statuses = append(statuses, status)
	}
	for i, job := range jobs {
		s.active[job.status.VMName] = job.cancel
		s.jobs <- job
		statuses[i] = job.status
	}
	return statuses, nil
}

func (s *V2VService) Statuses(ctx context.Context) ([]V2VStatus, error) {
	return s.repository.ListV2VStatuses(ctx)
}

func (s *V2VService) Cancel(vmName string) error {
	s.mu.Lock()
	cancel, exists := s.active[vmName]
	s.mu.Unlock()
	if !exists {
		return ErrV2VNotRunning
	}
	cancel()
	return nil
}

func (s *V2VService) CancelAll() int {
	s.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.active))
	for _, cancel := range s.active {
		cancels = append(cancels, cancel)
	}
	s.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	return len(cancels)
}

// Shutdown cancels active jobs, drains queued jobs into cancelled statuses, then waits for
// the worker to finish snapshot cleanup before the database is closed.
func (s *V2VService) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		for _, cancel := range s.active {
			cancel()
		}
		close(s.shutdown)
	}
	s.mu.Unlock()
	select {
	case <-s.workerDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *V2VService) newSnapshotName() string {
	counter := s.snapshotCounter.Add(1)
	return fmt.Sprintf("v2v-inspection-%d-%d", time.Now().UTC().UnixNano(), counter)
}

func (s *V2VService) runWorker() {
	defer close(s.workerDone)
	for {
		select {
		case job := <-s.jobs:
			s.runJob(job)
		case <-s.shutdown:
			for {
				select {
				case job := <-s.jobs:
					job.cancel()
					s.finishJob(&job, "", nil, nil, context.Canceled)
				default:
					return
				}
			}
		}
	}
}

func (s *V2VService) runJob(job v2vJob) {
	status := job.status
	if err := job.ctx.Err(); err != nil {
		s.finishJob(&job, "", &status, nil, err)
		return
	}

	status.State = V2VStateRunning
	started := time.Now().UTC()
	status.StartedAt = &started
	status.Details = "creating temporary inspection snapshot"
	s.saveStatus(&status)

	if !s.BackendAvailable() {
		s.finishJob(&job, "", &status, nil, ErrV2VBackendUnavailable)
		return
	}

	// Cleanup by the generated name even if snapshot creation returns an error: vCenter may
	// have completed the task while the client lost its response.
	vmMoref, snapshotMoref, err := s.vm.CreateV2VInspectionSnapshot(job.ctx, status.VMName, status.SnapshotName)
	if err != nil {
		s.finishJob(&job, status.SnapshotName, &status, nil, err)
		return
	}
	status.SnapshotID = snapshotMoref
	status.Details = "running virt-v2v-inspector through vm-migration-detective"
	s.saveStatus(&status)

	result, detectErr := s.detector.Detect(vmdetect.DetectParams{
		Ctx:           job.ctx,
		VMMoref:       vmMoref,
		SnapshotMoref: snapshotMoref,
		RunVirtV2v:    true,
	})
	if detectErr == nil && result == nil {
		detectErr = errors.New("detector returned no V2V result")
	}
	if detectErr == nil {
		status.Result = result.V2VOSInfo
		status.Concerns = result.AllConcerns
		if !result.Passed {
			detectErr = concernError(result.AllConcerns)
		}
	}
	s.finishJob(&job, status.SnapshotName, &status, result, detectErr)
}

func (s *V2VService) finishJob(job *v2vJob, snapshotName string, status *V2VStatus, result *vmdetect.DetectResult, jobErr error) {
	if status == nil {
		status = &job.status
	}
	if snapshotName != "" {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), v2vSnapshotCleanupTimeout)
		if err := s.vm.RemoveV2VInspectionSnapshot(cleanupCtx, status.VMName, snapshotName); err != nil {
			s.logger.WithError(err).WithFields(logrus.Fields{"vm_name": status.VMName, "snapshot_name": snapshotName}).Warn("failed to remove temporary V2V snapshot")
		}
		cancel()
	}

	completed := time.Now().UTC()
	status.CompletedAt = &completed
	status.UpdatedAt = completed
	status.Details = "V2V inspection finished"
	switch {
	case errors.Is(jobErr, context.Canceled), errors.Is(jobErr, context.DeadlineExceeded) && job.ctx.Err() != nil:
		status.State = V2VStateCancelled
		status.Error = jobErr.Error()
	case jobErr != nil:
		status.State = V2VStateFailed
		status.Error = jobErr.Error()
	default:
		status.State = V2VStateCompleted
		status.Details = "V2V inspection completed"
	}
	if result != nil {
		status.Result = result.V2VOSInfo
		status.Concerns = result.AllConcerns
	}
	s.saveStatus(status)

	s.mu.Lock()
	delete(s.active, status.VMName)
	s.mu.Unlock()
	job.cancel()
}

func (s *V2VService) saveStatus(status *V2VStatus) {
	status.UpdatedAt = time.Now().UTC()
	if err := s.repository.SaveV2VStatus(context.Background(), *status); err != nil {
		s.logger.WithError(err).WithField("vm_name", status.VMName).Error("failed to persist V2V inspection status")
	}
}

func concernError(concerns []checks.Concern) error {
	if len(concerns) > 0 && concerns[0].Message != "" {
		return errors.New(concerns[0].Message)
	}
	return errors.New("virt-v2v-inspector reported a migration concern")
}
