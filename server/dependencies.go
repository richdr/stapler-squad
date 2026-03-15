package server

import (
	"claude-squad/log"
	"claude-squad/server/events"
	"claude-squad/server/services"
	"claude-squad/session"
	"claude-squad/session/detection"
	"claude-squad/session/scrollback"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ServerDependencies holds all wired service components for the HTTP server.
// Use BuildDependencies to construct and wire them in the correct order.
// See the initialization order comment on NewServer for dependency constraints.
type ServerDependencies struct {
	SessionService          *services.SessionService
	Storage                 *session.Storage
	EventBus                *events.EventBus
	StatusManager           *session.InstanceStatusManager
	ReviewQueue             *session.ReviewQueue
	ReviewQueuePoller       *session.ReviewQueuePoller
	ReactiveQueueMgr        *ReactiveQueueManager
	ScrollbackManager       *scrollback.ScrollbackManager
	TmuxStreamerManager     *session.ExternalTmuxStreamerManager
	ExternalDiscovery       *session.ExternalSessionDiscovery
	ExternalApprovalMonitor *session.ExternalApprovalMonitor
}

// BuildDependencies constructs and wires all server dependencies in the correct order.
// Returns an error only for unrecoverable failures (SessionService init, Storage start).
// Non-fatal failures (individual instance start) are logged and skipped.
func BuildDependencies() (*ServerDependencies, error) {
	// Step 1: SessionService (creates Storage, EventBus, ReviewQueue internally)
	sessionService, err := services.NewSessionServiceFromConfig()
	if err != nil {
		return nil, fmt.Errorf("initialize SessionService: %w", err)
	}

	storage := sessionService.GetStorage()
	eventBus := sessionService.GetEventBus()
	reviewQueue := sessionService.GetReviewQueueInstance()

	// Steps 2-3: StatusManager and ReviewQueuePoller (before storage starts)
	statusManager := session.NewInstanceStatusManager()
	reviewQueuePoller := session.NewReviewQueuePoller(reviewQueue, statusManager, storage)

	// Wire ApprovalStore as metadata provider for enriching review queue items (Story 3, Task 3.2)
	reviewQueuePoller.SetApprovalProvider(sessionService.GetApprovalStore())

	// Steps 5-7: load, wire, start instances and controllers
	instances, err := storage.LoadInstances()
	if err != nil {
		return nil, fmt.Errorf("load instances: %w", err)
	}

	// Step 5: wire dependencies to each instance
	for _, inst := range instances {
		inst.SetReviewQueue(reviewQueue)
		inst.SetStatusManager(statusManager)
	}
	reviewQueuePoller.SetInstances(instances)

	// Step 6: start tmux sessions for loaded instances
	for _, inst := range instances {
		if !inst.Started() {
			if err := inst.Start(false); err != nil {
				log.ErrorLog.Printf("Failed to start loaded instance '%s': %v", inst.Title, err)
			} else {
				log.InfoLog.Printf("Started loaded instance '%s'", inst.Title)
			}
		}
	}

	// Persist any auto-detected worktree info (must happen after Step 6)
	if len(instances) > 0 {
		if err := storage.SaveInstances(instances); err != nil {
			log.WarningLog.Printf("Failed to persist migrated instance data: %v", err)
		} else {
			log.InfoLog.Printf("Persisted migrated instance data for %d instances", len(instances))
		}
	}

	// Step 7: start controllers (requires started instances + StatusManager)
	log.InfoLog.Printf("Attempting controller startup for %d loaded instances", len(instances))
	for _, inst := range instances {
		started := inst.Started()
		paused := inst.Paused()
		log.InfoLog.Printf("Instance '%s': Started()=%v, Paused()=%v", inst.Title, started, paused)
		if started && !paused {
			if inst.GetController() == nil {
				if err := inst.StartController(); err != nil {
					log.WarningLog.Printf("Failed to start controller for '%s': %v", inst.Title, err)
				} else {
					log.InfoLog.Printf("Started controller for '%s'", inst.Title)
				}
			} else {
				log.InfoLog.Printf("Instance '%s' already has active controller", inst.Title)
			}
		}
	}

	// Step 7.5: Startup scan and orphaned approval sync (after controllers, before ReactiveQueueManager)
	// Brief settling delay to allow controllers to initialize their terminal readers.
	time.Sleep(500 * time.Millisecond)
	scanSessionsOnStartup(instances, reviewQueue, statusManager)
	syncOrphanedApprovalsToQueue(sessionService.GetApprovalStore(), instances, reviewQueue)

	// Step 8: ReactiveQueueManager
	reactiveQueueMgr := NewReactiveQueueManager(reviewQueue, reviewQueuePoller, eventBus, statusManager, storage)
	sessionService.SetReactiveQueueManager(reactiveQueueMgr)
	log.InfoLog.Printf("ReactiveQueueManager initialized")

	// Step 9: ScrollbackManager (independent of above)
	homeDir, _ := os.UserHomeDir()
	scrollbackPath := filepath.Join(homeDir, ".claude-squad", "sessions")
	scrollbackConfig := scrollback.DefaultScrollbackConfig()
	scrollbackConfig.StoragePath = scrollbackPath
	scrollbackManager := scrollback.NewScrollbackManager(scrollbackConfig)
	log.InfoLog.Printf("Initialized ScrollbackManager: path=%s, compression=%s, maxLines=%d",
		scrollbackPath, scrollbackConfig.CompressionType, scrollbackConfig.MaxLines)

	// Step 10: TmuxStreamerManager (independent)
	tmuxStreamerManager := session.NewExternalTmuxStreamerManager()

	// Step 11: ExternalDiscovery with session-added/removed callbacks
	externalDiscovery := session.NewExternalSessionDiscovery()
	externalDiscovery.OnSessionAdded(func(instance *session.Instance) {
		if err := storage.AddInstance(instance); err != nil {
			log.ErrorLog.Printf("Failed to persist external session '%s': %v", instance.Title, err)
		} else {
			log.InfoLog.Printf("Persisted external session '%s' to storage", instance.Title)
		}
		// Wire dependencies so the external session appears in the review queue
		instance.SetReviewQueue(reviewQueue)
		instance.SetStatusManager(statusManager)
		reviewQueuePoller.AddInstance(instance)
		log.InfoLog.Printf("Added external session '%s' to review queue poller", instance.Title)
	})
	externalDiscovery.OnSessionRemoved(func(instance *session.Instance) {
		reviewQueuePoller.RemoveInstance(instance.Title)
		log.InfoLog.Printf("Removed external session '%s' from review queue poller", instance.Title)
		reviewQueue.Remove(instance.Title)
		if err := storage.DeleteInstance(instance.Title); err != nil {
			log.WarningLog.Printf("Failed to remove external session '%s' from storage: %v", instance.Title, err)
		} else {
			log.InfoLog.Printf("Removed external session '%s' from storage", instance.Title)
		}
	})

	// Step 12: ExternalApprovalMonitor — wire approval-to-review-queue bridge
	externalApprovalMonitor := session.NewExternalApprovalMonitor()
	externalApprovalMonitor.OnApproval(func(event *session.ExternalApprovalEvent) {
		if event == nil || event.Request == nil {
			return
		}
		// Resolve the instance (try tmux session name first, socket path as fallback)
		inst := externalDiscovery.GetSessionByTmux(event.SessionID)
		if inst == nil {
			inst = externalDiscovery.GetSession(event.SessionID)
		}

		context := event.Request.DetectedText
		if context == "" {
			context = "Permission request detected"
		}

		item := &session.ReviewItem{
			SessionID:   event.SessionTitle,
			SessionName: event.SessionTitle,
			Reason:      session.ReasonApprovalPending,
			Priority:    session.PriorityHigh,
			DetectedAt:  event.Request.Timestamp,
			Context:     context,
		}
		if inst != nil {
			item.Program = inst.Program
			item.Branch = inst.Branch
			item.Path = inst.Path
			item.WorkingDir = inst.WorkingDir
			item.Status = inst.Status.String()
			item.Tags = inst.Tags
			item.Category = inst.Category
			item.DiffStats = inst.GetDiffStats()
			item.LastActivity = inst.LastMeaningfulOutput
		}

		reviewQueue.Add(item)
		log.InfoLog.Printf("Added external session approval '%s' to review queue (type: %s, confidence: %.2f)",
			event.SessionTitle, event.Request.Type, event.Request.Confidence)
	})

	return &ServerDependencies{
		SessionService:          sessionService,
		Storage:                 storage,
		EventBus:                eventBus,
		StatusManager:           statusManager,
		ReviewQueue:             reviewQueue,
		ReviewQueuePoller:       reviewQueuePoller,
		ReactiveQueueMgr:        reactiveQueueMgr,
		ScrollbackManager:       scrollbackManager,
		TmuxStreamerManager:     tmuxStreamerManager,
		ExternalDiscovery:       externalDiscovery,
		ExternalApprovalMonitor: externalApprovalMonitor,
	}, nil
}

// scanSessionsOnStartup scans all running sessions for pre-existing approval prompts,
// input required states, and errors. Adds matching sessions to the review queue immediately
// so the user sees them before the regular polling cycle kicks in.
func scanSessionsOnStartup(
	instances []*session.Instance,
	queue *session.ReviewQueue,
	statusManager *session.InstanceStatusManager,
) {
	detector := detection.NewStatusDetector()
	scanned, added := 0, 0

	for _, inst := range instances {
		if !inst.Started() || inst.Paused() {
			continue
		}
		scanned++

		// Try controller-based detection first
		statusInfo := statusManager.GetStatus(inst)
		if statusInfo.IsControllerActive {
			reason, priority, context := mapDetectedStatus(statusInfo.ClaudeStatus, statusInfo.StatusContext)
			if reason != "" {
				addStartupItem(queue, inst, reason, priority, context)
				added++
				log.InfoLog.Printf("[StartupScan] Session '%s': detected %s via controller (status=%s)",
					inst.Title, reason, statusInfo.ClaudeStatus.String())
			}
			continue
		}

		// Fallback: terminal content detection
		content, err := inst.Preview()
		if err != nil {
			log.WarningLog.Printf("[StartupScan] Session '%s': failed to get terminal content: %v", inst.Title, err)
			continue
		}
		if content == "" {
			log.InfoLog.Printf("[StartupScan] Session '%s': empty terminal content, skipping", inst.Title)
			continue
		}

		detectedStatus, statusContext := detector.DetectWithContext([]byte(content))
		reason, priority, ctx := mapDetectedStatus(detectedStatus, statusContext)
		if reason != "" {
			addStartupItem(queue, inst, reason, priority, ctx)
			added++
			log.InfoLog.Printf("[StartupScan] Session '%s': detected %s via terminal (status=%s)",
				inst.Title, reason, detectedStatus.String())
		}
	}

	log.InfoLog.Printf("[StartupScan] Scanned %d sessions, added %d to review queue", scanned, added)
}

// mapDetectedStatus maps a DetectedStatus to a review queue reason, priority, and context string.
// Returns empty reason if the status does not warrant adding to the review queue.
func mapDetectedStatus(status detection.DetectedStatus, statusContext string) (session.AttentionReason, session.Priority, string) {
	switch status {
	case detection.StatusNeedsApproval:
		ctx := statusContext
		if ctx == "" {
			ctx = "Waiting for approval to proceed"
		}
		return session.ReasonApprovalPending, session.PriorityHigh, ctx
	case detection.StatusInputRequired:
		ctx := statusContext
		if ctx == "" {
			ctx = "Waiting for explicit user input"
		}
		return session.ReasonInputRequired, session.PriorityMedium, ctx
	case detection.StatusError:
		ctx := statusContext
		if ctx == "" {
			ctx = "Error state detected"
		}
		return session.ReasonErrorState, session.PriorityUrgent, ctx
	default:
		return "", 0, ""
	}
}

// addStartupItem creates a ReviewItem from an instance and adds it to the queue.
func addStartupItem(queue *session.ReviewQueue, inst *session.Instance, reason session.AttentionReason, priority session.Priority, context string) {
	item := &session.ReviewItem{
		SessionID:    inst.Title,
		SessionName:  inst.Title,
		Reason:       reason,
		Priority:     priority,
		DetectedAt:   time.Now(),
		Context:      context,
		Program:      inst.Program,
		Branch:       inst.Branch,
		Path:         inst.Path,
		WorkingDir:   inst.WorkingDir,
		Status:       inst.Status.String(),
		Tags:         inst.Tags,
		Category:     inst.Category,
		DiffStats:    inst.GetDiffStats(),
		LastActivity: inst.LastMeaningfulOutput,
	}
	queue.Add(item)
}

// syncOrphanedApprovalsToQueue adds review queue items for orphaned (persisted) approvals.
// This ensures sessions with known pending approvals appear in the queue immediately on startup,
// even before the first poll cycle detects them via terminal content scanning.
func syncOrphanedApprovalsToQueue(
	store *services.ApprovalStore,
	instances []*session.Instance,
	queue *session.ReviewQueue,
) {
	if store == nil {
		return
	}

	orphaned := store.ListAll()
	if len(orphaned) == 0 {
		return
	}

	// Build a lookup map for instances by title
	instMap := make(map[string]*session.Instance, len(instances))
	for _, inst := range instances {
		instMap[inst.Title] = inst
	}

	added := 0
	for _, approval := range orphaned {
		if !approval.Orphaned {
			continue
		}

		// Build context from approval metadata
		context := fmt.Sprintf("Permission required: %s", approval.ToolName)
		if cmd, ok := approval.ToolInput["command"].(string); ok && cmd != "" {
			if len(cmd) > 120 {
				context = cmd[:120] + "..."
			} else {
				context = cmd
			}
		}

		item := &session.ReviewItem{
			SessionID:   approval.SessionID,
			SessionName: approval.SessionID,
			Reason:      session.ReasonApprovalPending,
			Priority:    session.PriorityHigh,
			DetectedAt:  approval.CreatedAt,
			Context:     context,
			Metadata: map[string]string{
				"pending_approval_id": approval.ID,
				"tool_name":           approval.ToolName,
				"orphaned":            "true",
			},
			LastActivity: approval.CreatedAt,
		}

		// Enrich with instance data if available
		if inst, ok := instMap[approval.SessionID]; ok {
			item.Program = inst.Program
			item.Branch = inst.Branch
			item.Path = inst.Path
			item.WorkingDir = inst.WorkingDir
			item.Status = inst.Status.String()
			item.Tags = inst.Tags
			item.Category = inst.Category
			item.DiffStats = inst.GetDiffStats()
			if !inst.LastMeaningfulOutput.IsZero() {
				item.LastActivity = inst.LastMeaningfulOutput
			}
		}

		queue.Add(item)
		added++
		log.InfoLog.Printf("[ApprovalSync] Added orphaned approval to review queue: session=%s, tool=%s, approval_id=%s",
			approval.SessionID, approval.ToolName, approval.ID)
	}

	if added > 0 {
		log.InfoLog.Printf("[ApprovalSync] Synced %d orphaned approvals to review queue", added)
	}
}
