package session

import (
	"claude-squad/log"
	"context"
	"fmt"
	"sync"
	"time"
)

// ReviewQueuePollerConfig contains configuration for the review queue poller.
type ReviewQueuePollerConfig struct {
	PollInterval       time.Duration // How often to check sessions
	IdleThreshold      time.Duration // Duration before considering session idle and adding to queue
	InputWaitDuration  time.Duration // Time waiting for input before flagging
	StalenessThreshold time.Duration // Duration since last meaningful output before considering stale
}

// DefaultReviewQueuePollerConfig returns sensible defaults for polling.
func DefaultReviewQueuePollerConfig() ReviewQueuePollerConfig {
	return ReviewQueuePollerConfig{
		PollInterval:       2 * time.Second,  // Poll every 2 seconds for immediate detection
		IdleThreshold:      10 * time.Second, // Add to queue after 10s idle (reduced from 30s for faster detection)
		InputWaitDuration:  3 * time.Second,  // Flag if waiting for input > 3s (reduced from 5s)
		StalenessThreshold: 2 * time.Minute,  // Flag if no meaningful output for 2 minutes (reduced from 5min)
	}
}

// ReviewQueuePoller automatically monitors sessions and adds them to the review queue
// when they become idle or need attention.
type ReviewQueuePoller struct {
	queue         *ReviewQueue
	statusManager *InstanceStatusManager
	instances     []*Instance
	config        ReviewQueuePollerConfig

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// NewReviewQueuePoller creates a new poller for automatically managing the review queue.
func NewReviewQueuePoller(queue *ReviewQueue, statusManager *InstanceStatusManager) *ReviewQueuePoller {
	return NewReviewQueuePollerWithConfig(queue, statusManager, DefaultReviewQueuePollerConfig())
}

// NewReviewQueuePollerWithConfig creates a poller with custom configuration.
func NewReviewQueuePollerWithConfig(queue *ReviewQueue, statusManager *InstanceStatusManager, config ReviewQueuePollerConfig) *ReviewQueuePoller {
	return &ReviewQueuePoller{
		queue:         queue,
		statusManager: statusManager,
		instances:     make([]*Instance, 0),
		config:        config,
	}
}

// SetInstances sets the list of instances to monitor.
func (rqp *ReviewQueuePoller) SetInstances(instances []*Instance) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()
	rqp.instances = instances
}

// AddInstance adds a single instance to monitor.
func (rqp *ReviewQueuePoller) AddInstance(instance *Instance) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()
	rqp.instances = append(rqp.instances, instance)
}

// RemoveInstance removes an instance from monitoring.
func (rqp *ReviewQueuePoller) RemoveInstance(instanceTitle string) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()

	filtered := make([]*Instance, 0, len(rqp.instances))
	for _, inst := range rqp.instances {
		if inst.Title != instanceTitle {
			filtered = append(filtered, inst)
		}
	}
	rqp.instances = filtered
}

// Start begins polling for idle sessions.
func (rqp *ReviewQueuePoller) Start(ctx context.Context) {
	rqp.mu.Lock()
	if rqp.ctx != nil {
		rqp.mu.Unlock()
		log.InfoLog.Printf("ReviewQueuePoller already started")
		return
	}

	rqp.ctx, rqp.cancel = context.WithCancel(ctx)
	rqp.mu.Unlock()

	// Perform initial queue population immediately on startup
	// This ensures the queue is populated without waiting for the first poll interval
	rqp.checkSessions()

	rqp.wg.Add(1)
	go rqp.pollLoop()

	log.InfoLog.Printf("ReviewQueuePoller started (poll interval: %s)", rqp.config.PollInterval)
}

// Stop stops the poller.
func (rqp *ReviewQueuePoller) Stop() {
	rqp.mu.Lock()
	if rqp.cancel != nil {
		rqp.cancel()
	}
	rqp.mu.Unlock()

	rqp.wg.Wait()
	log.InfoLog.Printf("ReviewQueuePoller stopped")
}

// pollLoop is the main polling loop that runs in the background.
func (rqp *ReviewQueuePoller) pollLoop() {
	defer rqp.wg.Done()

	ticker := time.NewTicker(rqp.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rqp.ctx.Done():
			return
		case <-ticker.C:
			rqp.checkSessions()
		}
	}
}

// checkSessions checks all instances and updates the review queue.
func (rqp *ReviewQueuePoller) checkSessions() {
	rqp.mu.RLock()
	instances := make([]*Instance, len(rqp.instances))
	copy(instances, rqp.instances)
	rqp.mu.RUnlock()

	for _, inst := range instances {
		rqp.checkSession(inst)
	}
}

// checkSession checks a single session and adds/removes from queue as needed.
func (rqp *ReviewQueuePoller) checkSession(inst *Instance) {
	// Get comprehensive status
	statusInfo := rqp.statusManager.GetStatus(inst)

	if !statusInfo.IsControllerActive {
		// No controller active, remove from queue if present
		log.DebugLog.Printf("[ReviewQueue] Session '%s': No active controller (started=%v, paused=%v)",
			inst.Title, inst.Started(), inst.Paused())
		rqp.queue.Remove(inst.Title)
		return
	}

	// Get controller for idle detection
	controller, exists := rqp.statusManager.GetController(inst.Title)
	if !exists || controller == nil {
		log.DebugLog.Printf("[ReviewQueue] Session '%s': Controller not found in status manager", inst.Title)
		rqp.queue.Remove(inst.Title)
		return
	}

	log.DebugLog.Printf("[ReviewQueue] Session '%s': Checking idle state (controller active)", inst.Title)

	// Get idle state
	idleState, lastActivity := controller.GetIdleState()
	log.DebugLog.Printf("[ReviewQueue] Session '%s': Detected idle state=%s, lastActivity=%s",
		inst.Title, idleState.String(), formatDuration(time.Since(lastActivity)))

	// Determine if needs attention and why
	var reason AttentionReason
	var priority Priority
	var shouldAdd bool
	var context string

	switch idleState {
	case IdleStateActive:
		// Actively working, remove from queue
		log.DebugLog.Printf("[ReviewQueue] Session '%s': Active state - removing from queue", inst.Title)
		rqp.queue.Remove(inst.Title)
		return

	case IdleStateWaiting:
		// Normal idle state (e.g., INSERT mode) - don't add to queue by default
		// Only add if there are specific issues (approval, error) checked below
		log.DebugLog.Printf("[ReviewQueue] Session '%s': Waiting state - will check for specific issues", inst.Title)
		shouldAdd = false

	case IdleStateTimeout:
		// Definite timeout - been idle too long
		reason = ReasonIdleTimeout
		priority = PriorityLow
		shouldAdd = true
		idleDuration := time.Since(lastActivity)
		context = fmt.Sprintf("Timed out after %s of inactivity", formatDuration(idleDuration))
		log.DebugLog.Printf("[ReviewQueue] Session '%s': Timeout detected - idle for %s", inst.Title, formatDuration(idleDuration))

	default:
		// Unknown state, remove from queue
		log.DebugLog.Printf("[ReviewQueue] Session '%s': Unknown idle state - removing from queue", inst.Title)
		rqp.queue.Remove(inst.Title)
		return
	}

	// Check for approval needs (higher priority than idle)
	if statusInfo.ClaudeStatus == StatusNeedsApproval || statusInfo.PendingApprovals > 0 {
		reason = ReasonApprovalPending
		priority = PriorityHigh
		shouldAdd = true
		context = "Waiting for approval to proceed"
		log.DebugLog.Printf("[ReviewQueue] Session '%s': Approval needed (status=%s, pendingApprovals=%d)",
			inst.Title, statusInfo.ClaudeStatus.String(), statusInfo.PendingApprovals)
	}

	// Check for errors (highest priority)
	if statusInfo.ClaudeStatus == StatusError {
		reason = ReasonErrorState
		priority = PriorityUrgent
		shouldAdd = true
		context = "Error state detected"
		log.DebugLog.Printf("[ReviewQueue] Session '%s': Error state detected", inst.Title)
	}

	// Check for terminal staleness (no meaningful output for configured threshold)
	// This helps identify sessions that might be stuck or waiting without showing obvious idle state
	timeSinceOutput := inst.GetTimeSinceLastMeaningfulOutput()
	log.DebugLog.Printf("[ReviewQueue] Session '%s': Staleness check - %s since last meaningful output (threshold: %s)",
		inst.Title, formatDuration(timeSinceOutput), formatDuration(rqp.config.StalenessThreshold))

	if timeSinceOutput > rqp.config.StalenessThreshold {
		// Only override if we don't already have a higher-priority reason
		if !shouldAdd || priority < PriorityMedium {
			reason = ReasonIdleTimeout // Reuse idle timeout reason for staleness
			priority = PriorityLow     // Lower priority than approval/error, but should be reviewed
			shouldAdd = true
			context = fmt.Sprintf("No meaningful output for %s (may be stuck or waiting)",
				formatDuration(timeSinceOutput))

			log.DebugLog.Printf("[ReviewQueue] Session '%s': Flagged as stale - %s since last meaningful output",
				inst.Title, formatDuration(timeSinceOutput))
		} else {
			log.DebugLog.Printf("[ReviewQueue] Session '%s': Stale but already has higher priority reason (%s)",
				inst.Title, reason.String())
		}
	}

	// Add or update in queue
	if shouldAdd {
		// Check if item already exists and preserve DetectedAt if status hasn't changed
		detectedAt := time.Now()
		isUpdate := false
		if existingItem, exists := rqp.queue.Get(inst.Title); exists {
			isUpdate = true
			// Preserve original timestamp if meaningful fields haven't changed
			if existingItem.Reason == reason &&
				existingItem.Priority == priority &&
				existingItem.Context == context {
				detectedAt = existingItem.DetectedAt
				log.DebugLog.Printf("[ReviewQueue] Session '%s': Updating existing queue item (no changes, preserving timestamp)", inst.Title)
			} else {
				log.DebugLog.Printf("[ReviewQueue] Session '%s': Updating queue item (reason changed from %s to %s, priority %s to %s)",
					inst.Title, existingItem.Reason.String(), reason.String(), existingItem.Priority.String(), priority.String())
			}
		}

		item := &ReviewItem{
			SessionID:    inst.Title,
			SessionName:  inst.Title,
			Reason:       reason,
			Priority:     priority,
			DetectedAt:   detectedAt,
			Context:      context,
			// Populate session details for rich display
			Program:      inst.Program,
			Branch:       inst.Branch,
			Path:         inst.Path,
			WorkingDir:   inst.WorkingDir,
			Status:       inst.Status,
			Tags:         inst.Tags,
			Category:     inst.Category,
			DiffStats:    inst.GetDiffStats(),
			LastActivity: inst.LastMeaningfulOutput,
		}
		rqp.queue.Add(item)

		if !isUpdate {
			log.DebugLog.Printf("[ReviewQueue] Session '%s': Added to queue - %s (priority: %s, context: %s)",
				inst.Title, reason.String(), priority.String(), context)
		}
	} else {
		// Remove from queue - log only if it was actually in the queue
		if rqp.queue.Has(inst.Title) {
			log.DebugLog.Printf("[ReviewQueue] Session '%s': Removing from queue (shouldAdd=false)", inst.Title)
			rqp.queue.Remove(inst.Title)
		}
	}
}

// UpdateConfig updates the poller configuration.
func (rqp *ReviewQueuePoller) UpdateConfig(config ReviewQueuePollerConfig) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()
	rqp.config = config
	log.InfoLog.Printf("ReviewQueuePoller config updated: poll interval=%s, idle threshold=%s",
		config.PollInterval, config.IdleThreshold)
}

// GetConfig returns the current configuration.
func (rqp *ReviewQueuePoller) GetConfig() ReviewQueuePollerConfig {
	rqp.mu.RLock()
	defer rqp.mu.RUnlock()
	return rqp.config
}

// IsRunning returns true if the poller is currently running.
func (rqp *ReviewQueuePoller) IsRunning() bool {
	rqp.mu.RLock()
	defer rqp.mu.RUnlock()
	return rqp.ctx != nil && rqp.ctx.Err() == nil
}

// GetMonitoredCount returns the number of instances being monitored.
func (rqp *ReviewQueuePoller) GetMonitoredCount() int {
	rqp.mu.RLock()
	defer rqp.mu.RUnlock()
	return len(rqp.instances)
}

// CheckSession checks a single session immediately (exported for ReactiveQueueManager).
// This allows external components to trigger immediate re-evaluation without waiting for
// the next poll cycle, providing <100ms feedback on user interactions.
func (rqp *ReviewQueuePoller) CheckSession(inst *Instance) {
	rqp.checkSession(inst)
}

// FindInstance finds an instance by session ID (exported for ReactiveQueueManager).
// Returns nil if the instance is not found in the monitored list.
func (rqp *ReviewQueuePoller) FindInstance(sessionID string) *Instance {
	rqp.mu.RLock()
	defer rqp.mu.RUnlock()

	for _, inst := range rqp.instances {
		if inst.Title == sessionID {
			return inst
		}
	}
	return nil
}
