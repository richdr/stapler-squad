package services

import (
	"bufio"
	"claude-squad/config"
	sessionv1 "claude-squad/gen/proto/go/session/v1"
	"claude-squad/log"
	"claude-squad/server/adapters"
	"claude-squad/server/events"
	"claude-squad/session"
	"connectrpc.com/connect"
	"context"
	"fmt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// ReactiveQueueManager is an interface to avoid circular dependencies.
// The actual implementation is in server/review_queue_manager.go
type ReactiveQueueManager interface {
	AddStreamClient(ctx context.Context, filters interface{}) (<-chan *sessionv1.ReviewQueueEvent, string)
	RemoveStreamClient(clientID string)
}

// SessionService implements the SessionServiceHandler interface for ConnectRPC.
type SessionService struct {
	storage            *session.Storage
	eventBus           *events.EventBus
	reviewQueue        *session.ReviewQueue
	statusManager      *session.InstanceStatusManager
	reviewQueuePoller  *session.ReviewQueuePoller
	reactiveQueueMgr   ReactiveQueueManager
}

// NewSessionService creates a new SessionService with the given storage and event bus.
func NewSessionService(storage *session.Storage, eventBus *events.EventBus) *SessionService {
	reviewQueue := session.NewReviewQueue()

	// Wire up review queue to loaded instances
	instances, err := storage.LoadInstances()
	if err == nil {
		for _, inst := range instances {
			inst.SetReviewQueue(reviewQueue)
		}
	}

	return &SessionService{
		storage:     storage,
		eventBus:    eventBus,
		reviewQueue: reviewQueue,
	}
}

// NewSessionServiceFromConfig creates a SessionService using the default config and state.
func NewSessionServiceFromConfig() (*SessionService, error) {
	state := config.LoadState()
	storage, err := session.NewStorage(state)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}
	eventBus := events.NewEventBus(100) // Buffer 100 events per subscriber
	return NewSessionService(storage, eventBus), nil
}

// GetStorage returns the storage instance for direct access (e.g., WebSocket handlers).
func (s *SessionService) GetStorage() *session.Storage {
	return s.storage
}

// GetEventBus returns the event bus instance for wiring up reactive components.
func (s *SessionService) GetEventBus() *events.EventBus {
	return s.eventBus
}

// GetReviewQueueInstance returns the review queue instance for wiring up reactive components.
func (s *SessionService) GetReviewQueueInstance() *session.ReviewQueue {
	return s.reviewQueue
}

// SetReactiveQueueManager sets the ReactiveQueueManager (dependency injection).
// This must be called before WatchReviewQueue is used.
func (s *SessionService) SetReactiveQueueManager(mgr ReactiveQueueManager) {
	s.reactiveQueueMgr = mgr
}

// ListSessions returns all sessions with optional filtering.
func (s *SessionService) ListSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.ListSessionsRequest],
) (*connect.Response[sessionv1.ListSessionsResponse], error) {
	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Convert instances to proto messages
	sessions := make([]*sessionv1.Session, 0, len(instances))
	for _, inst := range instances {
		// Apply optional status filter
		if req.Msg.Status != nil && *req.Msg.Status != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
			protoStatus := adapters.InstanceToProto(inst).Status
			if protoStatus != *req.Msg.Status {
				continue
			}
		}

		// Apply optional category filter
		if req.Msg.Category != nil && *req.Msg.Category != "" && inst.Category != *req.Msg.Category {
			continue
		}

		sessions = append(sessions, adapters.InstanceToProto(inst))
	}

	return connect.NewResponse(&sessionv1.ListSessionsResponse{
		Sessions: sessions,
	}), nil
}

// GetSession retrieves a specific session by ID (Title).
func (s *SessionService) GetSession(
	ctx context.Context,
	req *connect.Request[sessionv1.GetSessionRequest],
) (*connect.Response[sessionv1.GetSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find instance by ID (using Title as ID)
	for _, inst := range instances {
		if inst.Title == req.Msg.Id {
			return connect.NewResponse(&sessionv1.GetSessionResponse{
				Session: adapters.InstanceToProto(inst),
			}), nil
		}
	}

	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
}

// CreateSession initializes a new AI agent session with tmux and git worktree.
func (s *SessionService) CreateSession(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateSessionRequest],
) (*connect.Response[sessionv1.CreateSessionResponse], error) {
	// Validate required fields
	if req.Msg.Title == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("title is required"))
	}
	if req.Msg.Path == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path is required"))
	}

	// Check if session with this title already exists
	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}
	for _, inst := range instances {
		if inst.Title == req.Msg.Title {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("session with title '%s' already exists", req.Msg.Title))
		}
	}

	// Set default program if not specified
	program := req.Msg.Program
	if program == "" {
		cfg := config.LoadConfig()
		program = cfg.DefaultProgram
	}

	// Determine session type based on ExistingWorktree field
	sessionType := session.SessionTypeDirectory
	if req.Msg.ExistingWorktree != "" {
		sessionType = session.SessionTypeExistingWorktree
	} else if req.Msg.Branch != "" {
		// If branch is specified, create a new worktree
		sessionType = session.SessionTypeNewWorktree
	}

	// Create instance using NewInstance constructor
	instance, err := session.NewInstance(session.InstanceOptions{
		Title:            req.Msg.Title,
		Path:             req.Msg.Path,
		WorkingDir:       req.Msg.WorkingDir,
		Program:          program,
		AutoYes:          req.Msg.AutoYes,
		Prompt:           req.Msg.Prompt,
		ExistingWorktree: req.Msg.ExistingWorktree,
		Category:         req.Msg.Category,
		SessionType:      sessionType,
		TmuxPrefix:       "", // Use default from config
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("failed to create instance: %w", err))
	}

	// Start the session (initializes tmux + git worktree)
	// Use Start(true) to indicate this is a first-time setup
	if err := instance.Start(true); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to start session: %w", err))
	}

	// Save instance to storage
	// Note: Storage uses SaveInstances (plural) which saves all instances
	// We need to load, append, and save all instances
	if err := s.storage.SaveInstances(append(instances, instance)); err != nil {
		// Cleanup on save failure
		if destroyErr := instance.Destroy(); destroyErr != nil {
			// Log cleanup error but return original save error
			log.ErrorLog.Printf("Failed to cleanup after save error: %v", destroyErr)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
	}

	// Publish SessionCreated event to all watchers
	s.eventBus.Publish(events.NewSessionCreatedEvent(instance))

	return connect.NewResponse(&sessionv1.CreateSessionResponse{
		Session: adapters.InstanceToProto(instance),
	}), nil
}

// UpdateSession modifies session properties (pause/resume, category, title).
func (s *SessionService) UpdateSession(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateSessionRequest],
) (*connect.Response[sessionv1.UpdateSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find the instance to update
	var instance *session.Instance
	var instanceIndex int
	for i, inst := range instances {
		if inst.Title == req.Msg.Id {
			instance = inst
			instanceIndex = i
			break
		}
	}

	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Track which fields are being updated for event publishing
	var updatedFields []string
	var oldStatus session.Status

	// Handle status change (pause/resume)
	if req.Msg.Status != nil && *req.Msg.Status != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
		targetStatus := adapters.ProtoToStatus(*req.Msg.Status)
		oldStatus = instance.Status

		if targetStatus == session.Paused && instance.Status != session.Paused {
			if err := instance.Pause(); err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to pause session: %w", err))
			}
			updatedFields = append(updatedFields, "status")
		} else if targetStatus != session.Paused && instance.Status == session.Paused {
			// Resume from paused state
			if err := instance.Resume(); err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resume session: %w", err))
			}
			updatedFields = append(updatedFields, "status")
		}
	}

	// Handle category update
	if req.Msg.Category != nil {
		instance.Category = *req.Msg.Category
		updatedFields = append(updatedFields, "category")
	}

	// Handle title update
	if req.Msg.Title != nil && *req.Msg.Title != "" && *req.Msg.Title != instance.Title {
		// Check if new title already exists
		for _, inst := range instances {
			if inst.Title == *req.Msg.Title {
				return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("session with title '%s' already exists", *req.Msg.Title))
			}
		}
		instance.Title = *req.Msg.Title
		updatedFields = append(updatedFields, "title")
	}

	// Update the instance in the list and save
	instances[instanceIndex] = instance
	if err := s.storage.SaveInstances(instances); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
	}

	// Publish events based on what was updated
	if len(updatedFields) > 0 {
		// Check if status changed specifically
		if oldStatus != instance.Status && oldStatus != 0 {
			s.eventBus.Publish(events.NewSessionStatusChangedEvent(instance, oldStatus, instance.Status))
		}
		// Also publish general update event
		s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, updatedFields))
	}

	return connect.NewResponse(&sessionv1.UpdateSessionResponse{
		Session: adapters.InstanceToProto(instance),
	}), nil
}

// DeleteSession stops and removes a session, cleaning up resources.
func (s *SessionService) DeleteSession(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteSessionRequest],
) (*connect.Response[sessionv1.DeleteSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find the instance to delete
	var instance *session.Instance
	for _, inst := range instances {
		if inst.Title == req.Msg.Id {
			instance = inst
			break
		}
	}

	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Destroy the session (cleanup tmux + git worktree)
	if err := instance.Destroy(); err != nil {
		// Log error but continue with deletion from storage
		log.WarningLog.Printf("Failed to cleanup session resources: %v", err)
	}

	// Delete from storage
	if err := s.storage.DeleteInstance(req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete instance from storage: %w", err))
	}

	// Publish SessionDeleted event to all watchers
	s.eventBus.Publish(events.NewSessionDeletedEvent(req.Msg.Id))

	return connect.NewResponse(&sessionv1.DeleteSessionResponse{
		Success: true,
		Message: fmt.Sprintf("Session '%s' deleted successfully", req.Msg.Id),
	}), nil
}

// WatchSessions streams real-time session events (created/updated/deleted).
// Sends initial snapshot of all sessions, then subscribes to real-time updates.
func (s *SessionService) WatchSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.WatchSessionsRequest],
	stream *connect.ServerStream[sessionv1.SessionEvent],
) error {
	// Send initial snapshot of all sessions
	instances, err := s.storage.LoadInstances()
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Apply optional filters from request
	for _, inst := range instances {
		// Filter by category if specified
		if req.Msg.CategoryFilter != nil && *req.Msg.CategoryFilter != "" {
			if inst.Category != *req.Msg.CategoryFilter {
				continue
			}
		}

		// Filter by status if specified
		if req.Msg.StatusFilter != nil && *req.Msg.StatusFilter != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
			if adapters.StatusToProto(inst.Status) != *req.Msg.StatusFilter {
				continue
			}
		}

		// Send as SessionCreated event for initial snapshot
		event := createInitialSnapshotEvent(inst)
		if err := stream.Send(event); err != nil {
			return fmt.Errorf("failed to send initial snapshot: %w", err)
		}
	}

	// Subscribe to real-time events from event bus
	eventCh, subID := s.eventBus.Subscribe(ctx)
	defer s.eventBus.Unsubscribe(subID)

	// Stream events until client disconnects or context is canceled
	for {
		select {
		case <-ctx.Done():
			// Client disconnected or context canceled
			return nil
		case event, ok := <-eventCh:
			if !ok {
				// Event channel closed (should not happen with proper cleanup)
				return nil
			}

			// Apply filters to real-time events
			if req.Msg.CategoryFilter != nil && *req.Msg.CategoryFilter != "" {
				if event.Session != nil && event.Session.Category != *req.Msg.CategoryFilter {
					continue
				}
			}

			if req.Msg.StatusFilter != nil && *req.Msg.StatusFilter != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
				if event.Session != nil && adapters.StatusToProto(event.Session.Status) != *req.Msg.StatusFilter {
					continue
				}
			}

			// Convert internal event to protobuf and send
			protoEvent := convertEventToProto(event)
			if err := stream.Send(protoEvent); err != nil {
				return fmt.Errorf("failed to send event: %w", err)
			}
		}
	}
}

// StreamTerminal provides bidirectional streaming for terminal I/O with delta compression.
// Implements bidirectional streaming where:
// - Client sends: terminal input and resize events
// - Server sends: terminal deltas (compressed output) or raw output (fallback)
func (s *SessionService) StreamTerminal(
	ctx context.Context,
	stream *connect.BidiStream[sessionv1.TerminalData, sessionv1.TerminalData],
) error {
	// Get the first message to determine which session to attach to
	initialMsg, err := stream.Receive()
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to receive initial message: %w", err))
	}

	if initialMsg == nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("no initial message received"))
	}

	if initialMsg.SessionId == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}

	// Load the session instance
	instances, err := s.storage.LoadInstances()
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	var instance *session.Instance
	for _, inst := range instances {
		if inst.Title == initialMsg.SessionId {
			instance = inst
			break
		}
	}

	if instance == nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", initialMsg.SessionId))
	}

	// Verify session is started and not paused
	if !instance.Started() {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session not started"))
	}

	if instance.Paused() {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session is paused"))
	}

	// Get PTY for reading terminal output
	ptyFile, err := instance.GetPTYReader()
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get PTY reader: %w", err))
	}

	// Create context for managing goroutines
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Channel for errors from goroutines
	errCh := make(chan error, 2)

	// Initialize terminal state for delta compression (default 80x25)
	// Will be resized when client sends first resize message
	terminalState := session.NewTerminalState(25, 80)
	var previousState *session.TerminalState

	// Goroutine 1: Read from PTY and send deltas to client (terminal output)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("panic in output goroutine: %v", r)
			}
		}()

		buf := make([]byte, 1024) // 1KB chunks as per task requirements
		for {
			select {
			case <-streamCtx.Done():
				return
			default:
				n, readErr := ptyFile.Read(buf)
				if n > 0 {
					// Process PTY output through terminal state
					if processErr := terminalState.ProcessOutput(buf[:n]); processErr != nil {
						log.WarningLog.Printf("Failed to process terminal output: %v", processErr)
						// Fallback to raw output on parse errors
						outputMsg := &sessionv1.TerminalData{
							SessionId: initialMsg.SessionId,
							Data: &sessionv1.TerminalData_Output{
								Output: &sessionv1.TerminalOutput{
									Data: buf[:n],
								},
							},
						}
						if sendErr := stream.Send(outputMsg); sendErr != nil {
							errCh <- fmt.Errorf("failed to send output: %w", sendErr)
							return
						}
						continue
					}

					// Generate delta from previous state to current state
					deltaMsg := terminalState.GenerateDelta(previousState)
					deltaMsg.SessionId = initialMsg.SessionId

					// Send delta to client
					if sendErr := stream.Send(deltaMsg); sendErr != nil {
						errCh <- fmt.Errorf("failed to send delta: %w", sendErr)
						return
					}

					// Clone current state for next delta generation
					previousState = terminalState.Clone()
				}

				if readErr != nil {
					// EOF or other read error
					if readErr.Error() != "EOF" {
						errCh <- fmt.Errorf("PTY read error: %w", readErr)
					}
					return
				}
			}
		}
	}()

	// Goroutine 2: Receive from client and forward to PTY (terminal input + resize)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("panic in input goroutine: %v", r)
			}
		}()

		for {
			select {
			case <-streamCtx.Done():
				return
			default:
				msg, receiveErr := stream.Receive()
				if receiveErr != nil {
					// Check if this is a normal EOF (client closed connection)
					// ConnectRPC returns io.EOF or various "stream ended" errors
					errStr := receiveErr.Error()
					if receiveErr == context.Canceled ||
						receiveErr == context.DeadlineExceeded ||
						errStr == "EOF" ||
						errStr == "stream ended" ||
						strings.Contains(errStr, "stream closed") ||
						strings.Contains(errStr, "connection closed") {
						// Client closed gracefully, exit without error
						return
					}
					// Other errors should be reported
					errCh <- fmt.Errorf("stream receive error: %w", receiveErr)
					return
				}

				if msg == nil {
					// Stream ended cleanly
					return
				}

				switch data := msg.Data.(type) {
				case *sessionv1.TerminalData_Input:
					// Forward input to PTY
					if _, writeErr := instance.WriteToPTY(data.Input.Data); writeErr != nil {
						// Send error back to client
						errorMsg := &sessionv1.TerminalData{
							SessionId: msg.SessionId,
							Data: &sessionv1.TerminalData_Error{
								Error: &sessionv1.TerminalError{
									Message: fmt.Sprintf("Failed to write to PTY: %v", writeErr),
									Code:    "WRITE_ERROR",
								},
							},
						}
						_ = stream.Send(errorMsg) // Best effort
						errCh <- writeErr
						return
					}

					// Publish user interaction event for immediate review queue reactivity
					s.eventBus.Publish(events.NewUserInteractionEvent(
						msg.SessionId,
						"terminal_input",
						"", // No additional context needed
					))

				case *sessionv1.TerminalData_Resize:
					// Handle terminal resize - update both PTY and terminal state
					cols := int(data.Resize.Cols)
					rows := int(data.Resize.Rows)

					if resizeErr := instance.ResizePTY(cols, rows); resizeErr != nil {
						// Send error back to client
						errorMsg := &sessionv1.TerminalData{
							SessionId: msg.SessionId,
							Data: &sessionv1.TerminalData_Error{
								Error: &sessionv1.TerminalError{
									Message: fmt.Sprintf("Failed to resize terminal: %v", resizeErr),
									Code:    "RESIZE_ERROR",
								},
							},
						}
						_ = stream.Send(errorMsg) // Best effort
						// Don't return on resize errors, they're not fatal
					} else {
						// Also resize terminal state to match
						terminalState.Resize(rows, cols)
						log.InfoLog.Printf("Resized terminal state to %dx%d for session %s", cols, rows, msg.SessionId)
					}

				case *sessionv1.TerminalData_Error:
					// Client sent an error, log it
					log.ErrorLog.Printf("Client error: %s (%s)", data.Error.Message, data.Error.Code)
				}
			}
		}
	}()

	// Wait for either context cancellation or error
	select {
	case <-streamCtx.Done():
		log.InfoLog.Printf("StreamTerminal: context done for session %s", initialMsg.SessionId)
		return nil // Clean shutdown
	case err := <-errCh:
		log.ErrorLog.Printf("StreamTerminal: error for session %s: %v", initialMsg.SessionId, err)
		return connect.NewError(connect.CodeInternal, err)
	}
}

// GetSessionDiff retrieves the current git diff for a session.
func (s *SessionService) GetSessionDiff(
	ctx context.Context,
	req *connect.Request[sessionv1.GetSessionDiffRequest],
) (*connect.Response[sessionv1.GetSessionDiffResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find instance by ID (using Title as ID)
	var instance *session.Instance
	for _, inst := range instances {
		if inst.Title == req.Msg.Id {
			instance = inst
			break
		}
	}

	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Get diff stats from the instance
	diffStats := instance.GetDiffStats()
	if diffStats == nil {
		// Return empty diff stats if none available
		return connect.NewResponse(&sessionv1.GetSessionDiffResponse{
			DiffStats: &sessionv1.DiffStats{
				Added:   0,
				Removed: 0,
				Content: "",
			},
		}), nil
	}

	// Convert to proto message
	protoDiffStats := &sessionv1.DiffStats{
		Added:   int32(diffStats.Added),
		Removed: int32(diffStats.Removed),
		Content: diffStats.Content,
	}

	return connect.NewResponse(&sessionv1.GetSessionDiffResponse{
		DiffStats: protoDiffStats,
	}), nil
}

// GetReviewQueue returns sessions needing user attention with priority ordering.
// It dynamically builds the queue from session statuses and actively checks for approval dialogs and idle sessions.
func (s *SessionService) GetReviewQueue(
	ctx context.Context,
	req *connect.Request[sessionv1.GetReviewQueueRequest],
) (*connect.Response[sessionv1.GetReviewQueueResponse], error) {
	// Build review queue dynamically from session statuses
	queue := session.NewReviewQueue()

	// Load all instances
	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Configuration for idle detection
	const idleThreshold = 15 * time.Minute // Sessions idle for more than 15 minutes

	// Check each session for approval dialogs, waiting states, and idle status
	for _, inst := range instances {
		// Skip paused or unstarted sessions
		if !inst.Started() || inst.Paused() {
			continue
		}

		// Actively check for approval dialogs by calling HasUpdated()
		// This triggers the detection logic that sets NeedsApproval status
		_, hasPrompt := inst.HasUpdated()

		// If session has a prompt and AutoYes is disabled, set status to NeedsApproval
		if hasPrompt && !inst.AutoYes {
			inst.SetStatus(session.NeedsApproval)
		} else if inst.Status == session.NeedsApproval && !hasPrompt {
			// Clear NeedsApproval status if no prompt is detected
			inst.SetStatus(session.Ready)
		}

		// Check for idle sessions (no updates for extended period)
		idleDuration := time.Since(inst.UpdatedAt)
		isIdle := idleDuration > idleThreshold

		// Sessions in Ready or Running state that haven't been updated recently are likely waiting for input
		// This catches Claude Code sessions in INSERT mode (Running status) and other idle sessions
		isWaitingForInput := (inst.Status == session.Ready || inst.Status == session.Running) && idleDuration > 5*time.Second

		// Check if session was dismissed (acknowledged after last update)
		isDismissed := !inst.LastAcknowledged.IsZero() && inst.LastAcknowledged.After(inst.UpdatedAt)

		// Add sessions with NeedsApproval status to queue
		if inst.Status == session.NeedsApproval {
			// Create review item for this session
			item := &session.ReviewItem{
				SessionID:   inst.Title,
				SessionName: inst.Title,
				Reason:      session.ReasonApprovalPending,
				Priority:    session.PriorityHigh,
				DetectedAt:  inst.UpdatedAt, // Use actual last update time
				Context:     "Waiting for user approval",
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

			// Apply filters if specified
			if req.Msg.PriorityFilter != nil {
				targetPriority := adapters.ProtoToPriority(*req.Msg.PriorityFilter)
				if item.Priority != targetPriority {
					continue
				}
			}

			if req.Msg.ReasonFilter != nil {
				targetReason := adapters.ProtoToAttentionReason(*req.Msg.ReasonFilter)
				if item.Reason != targetReason {
					continue
				}
			}

			// Add to queue
			queue.Add(item)
		} else if isWaitingForInput {
			// Add sessions waiting for input to review queue
			item := &session.ReviewItem{
				SessionID:   inst.Title,
				SessionName: inst.Title,
				Reason:      session.ReasonInputRequired,
				Priority:    session.PriorityMedium,
				DetectedAt:  inst.UpdatedAt, // Use last update time, not current time
				Context:     "Waiting for user input",
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

			// Apply filters if specified
			if req.Msg.PriorityFilter != nil {
				targetPriority := adapters.ProtoToPriority(*req.Msg.PriorityFilter)
				if item.Priority != targetPriority {
					continue
				}
			}

			if req.Msg.ReasonFilter != nil {
				targetReason := adapters.ProtoToAttentionReason(*req.Msg.ReasonFilter)
				if item.Reason != targetReason {
					continue
				}
			}

			// Add to queue
			queue.Add(item)
		} else if isIdle {
			// Add idle sessions to review queue
			item := &session.ReviewItem{
				SessionID:   inst.Title,
				SessionName: inst.Title,
				Reason:      session.ReasonIdleTimeout,
				Priority:    session.PriorityLow,
				DetectedAt:  inst.UpdatedAt, // Use last actual update time
				Context:     fmt.Sprintf("No activity for %s", formatDuration(idleDuration)),
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

			// Apply filters if specified
			if req.Msg.PriorityFilter != nil {
				targetPriority := adapters.ProtoToPriority(*req.Msg.PriorityFilter)
				if item.Priority != targetPriority {
					continue
				}
			}

			if req.Msg.ReasonFilter != nil {
				targetReason := adapters.ProtoToAttentionReason(*req.Msg.ReasonFilter)
				if item.Reason != targetReason {
					continue
				}
			}

			// Add to queue
			queue.Add(item)
		} else if isDismissed {
			// Add dismissed sessions to review queue with very low priority
			// Show time since last changes to help user decide if worth revisiting
			timeSinceChange := time.Since(inst.UpdatedAt)
			item := &session.ReviewItem{
				SessionID:   inst.Title,
				SessionName: inst.Title,
				Reason:      session.ReasonIdleTimeout, // Use idle timeout as reason for dismissed sessions
				Priority:    session.PriorityLow,       // Very low priority since user already acknowledged
				DetectedAt:  inst.UpdatedAt,            // Use last actual update time
				Context:     fmt.Sprintf("Dismissed - last changed %s ago", formatDuration(timeSinceChange)),
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

			// Apply filters if specified
			if req.Msg.PriorityFilter != nil {
				targetPriority := adapters.ProtoToPriority(*req.Msg.PriorityFilter)
				if item.Priority != targetPriority {
					continue
				}
			}

			if req.Msg.ReasonFilter != nil {
				targetReason := adapters.ProtoToAttentionReason(*req.Msg.ReasonFilter)
				if item.Reason != targetReason {
					continue
				}
			}

			// Add to queue
			queue.Add(item)
		}
	}

	// Save updated statuses back to storage
	if err := s.storage.SaveInstances(instances); err != nil {
		log.WarningLog.Printf("Failed to save updated session statuses: %v", err)
		// Don't fail the request if save fails - return current queue state
	}

	// Convert to proto using adapters
	protoQueue := adapters.ReviewQueueToProto(queue)

	return connect.NewResponse(&sessionv1.GetReviewQueueResponse{
		ReviewQueue: protoQueue,
	}), nil
}

// AcknowledgeSession marks a session as acknowledged in the review queue.
// The session won't reappear in the queue until it receives an update.
func (s *SessionService) AcknowledgeSession(
	ctx context.Context,
	req *connect.Request[sessionv1.AcknowledgeSessionRequest],
) (*connect.Response[sessionv1.AcknowledgeSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find the instance to acknowledge
	var instance *session.Instance
	var instanceIndex int
	for i, inst := range instances {
		if inst.Title == req.Msg.Id {
			instance = inst
			instanceIndex = i
			break
		}
	}

	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Set the acknowledgment timestamp to now
	instance.LastAcknowledged = time.Now()

	// Update the instance in the list and save
	instances[instanceIndex] = instance
	if err := s.storage.SaveInstances(instances); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
	}

	// Publish event for immediate reactivity
	s.eventBus.Publish(events.NewSessionAcknowledgedEvent(
		instance.Title,
		"user_acknowledged",
	))

	return connect.NewResponse(&sessionv1.AcknowledgeSessionResponse{
		Success: true,
		Message: fmt.Sprintf("Session '%s' acknowledged and removed from review queue", req.Msg.Id),
	}), nil
}

// GetLogs retrieves application logs with optional filtering and search.
func (s *SessionService) GetLogs(
	ctx context.Context,
	req *connect.Request[sessionv1.GetLogsRequest],
) (*connect.Response[sessionv1.GetLogsResponse], error) {
	// Get log file path from config
	cfg := log.ConfigToLogConfig(config.LoadConfig())
	logFilePath, err := log.GetLogFilePath(cfg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get log file path: %w", err))
	}

	// Read log file
	file, err := os.Open(logFilePath)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to open log file: %w", err))
	}
	defer file.Close()

	// Parse logs with filters
	entries, err := parseLogs(file, req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse logs: %w", err))
	}

	return connect.NewResponse(&sessionv1.GetLogsResponse{
		Entries: entries,
	}), nil
}

// parseLogs reads log file and applies filters to return matching entries
func parseLogs(reader io.Reader, req *sessionv1.GetLogsRequest) ([]*sessionv1.LogEntry, error) {
	// Log line format: [instance] LEVEL:date time file:line: message
	// Example: [pid-12345-timestamp] INFO:2025/10/17 14:23:45 app.go:123: Starting session
	logLineRegex := regexp.MustCompile(`^\[([^\]]+)\]\s+(\w+):(\d{4}/\d{2}/\d{2})\s+(\d{2}:\d{2}:\d{2})\s+([^:]+:\d+):\s+(.*)$`)

	var entries []*sessionv1.LogEntry
	scanner := bufio.NewScanner(reader)

	// Default limit if not specified
	limit := 100
	if req.Limit != nil && *req.Limit > 0 {
		limit = int(*req.Limit)
	}

	// Parse filters
	var searchQuery string
	if req.SearchQuery != nil {
		searchQuery = strings.ToLower(*req.SearchQuery)
	}

	var levelFilter string
	if req.Level != nil {
		levelFilter = strings.ToUpper(*req.Level)
	}

	var startTime, endTime *time.Time
	if req.StartTime != nil {
		t := req.StartTime.AsTime()
		startTime = &t
	}
	if req.EndTime != nil {
		t := req.EndTime.AsTime()
		endTime = &t
	}

	for scanner.Scan() {
		line := scanner.Text()

		// Try to parse the log line
		matches := logLineRegex.FindStringSubmatch(line)
		if matches == nil || len(matches) < 7 {
			// Skip lines that don't match expected format
			continue
		}

		// Extract fields from regex match
		// matches[1] = instance (ignored for API)
		level := matches[2]
		dateStr := matches[3]
		timeStr := matches[4]
		source := matches[5]
		message := matches[6]

		// Parse timestamp
		timestampStr := fmt.Sprintf("%s %s", dateStr, timeStr)
		timestamp, err := time.Parse("2006/01/02 15:04:05", timestampStr)
		if err != nil {
			// Skip entries with invalid timestamps
			continue
		}

		// Apply level filter
		if levelFilter != "" && level != levelFilter {
			continue
		}

		// Apply time range filters
		if startTime != nil && timestamp.Before(*startTime) {
			continue
		}
		if endTime != nil && timestamp.After(*endTime) {
			continue
		}

		// Apply search query filter (case-insensitive, searches message and source)
		if searchQuery != "" {
			messageAndSource := strings.ToLower(message + " " + source)
			if !strings.Contains(messageAndSource, searchQuery) {
				continue
			}
		}

		// Create log entry
		entry := &sessionv1.LogEntry{
			Timestamp: timestamppb.New(timestamp),
			Level:     level,
			Message:   message,
			Source:    &source,
		}

		entries = append(entries, entry)

		// Apply limit
		if len(entries) >= limit {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading log file: %w", err)
	}

	return entries, nil
}

// WatchReviewQueueFilters contains filters for review queue event streaming.
type WatchReviewQueueFilters struct {
	PriorityFilter    []session.Priority
	ReasonFilter      []session.AttentionReason
	SessionIDs        []string
	IncludeStatistics bool
	InitialSnapshot   bool
}

// Implement FilterProvider interface for type-safe conversion
func (f *WatchReviewQueueFilters) GetPriorityFilter() []session.Priority {
	return f.PriorityFilter
}

func (f *WatchReviewQueueFilters) GetReasonFilter() []session.AttentionReason {
	return f.ReasonFilter
}

func (f *WatchReviewQueueFilters) GetSessionIDs() []string {
	return f.SessionIDs
}

func (f *WatchReviewQueueFilters) GetIncludeStatistics() bool {
	return f.IncludeStatistics
}

func (f *WatchReviewQueueFilters) GetInitialSnapshot() bool {
	return f.InitialSnapshot
}

// WatchReviewQueue streams real-time review queue events.
func (s *SessionService) WatchReviewQueue(
	ctx context.Context,
	req *connect.Request[sessionv1.WatchReviewQueueRequest],
	stream *connect.ServerStream[sessionv1.ReviewQueueEvent],
) error {
	if s.reactiveQueueMgr == nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("reactive queue manager not initialized"))
	}

	// Convert proto filters to internal type
	filters := &WatchReviewQueueFilters{
		PriorityFilter:    convertProtoPriorities(req.Msg.PriorityFilter),
		ReasonFilter:      convertProtoReasons(req.Msg.ReasonFilter),
		SessionIDs:        req.Msg.SessionIds,
		IncludeStatistics: req.Msg.IncludeStatistics,
		InitialSnapshot:   req.Msg.InitialSnapshot,
	}

	// Subscribe to queue events
	eventCh, clientID := s.reactiveQueueMgr.AddStreamClient(ctx, filters)
	defer s.reactiveQueueMgr.RemoveStreamClient(clientID)

	// Stream events
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-eventCh:
			if !ok {
				return nil // Channel closed
			}
			if err := stream.Send(event); err != nil {
				return err
			}
		}
	}
}

// convertProtoPriorities converts proto Priority values to internal session.Priority
func convertProtoPriorities(protoPriorities []sessionv1.Priority) []session.Priority {
	result := make([]session.Priority, 0, len(protoPriorities))
	for _, p := range protoPriorities {
		switch p {
		case sessionv1.Priority_PRIORITY_URGENT:
			result = append(result, session.PriorityUrgent)
		case sessionv1.Priority_PRIORITY_HIGH:
			result = append(result, session.PriorityHigh)
		case sessionv1.Priority_PRIORITY_MEDIUM:
			result = append(result, session.PriorityMedium)
		case sessionv1.Priority_PRIORITY_LOW:
			result = append(result, session.PriorityLow)
		}
	}
	return result
}

// convertProtoReasons converts proto AttentionReason values to internal session.AttentionReason
func convertProtoReasons(protoReasons []sessionv1.AttentionReason) []session.AttentionReason {
	result := make([]session.AttentionReason, 0, len(protoReasons))
	for _, r := range protoReasons {
		switch r {
		case sessionv1.AttentionReason_ATTENTION_REASON_APPROVAL_PENDING:
			result = append(result, session.ReasonApprovalPending)
		case sessionv1.AttentionReason_ATTENTION_REASON_INPUT_REQUIRED:
			result = append(result, session.ReasonInputRequired)
		case sessionv1.AttentionReason_ATTENTION_REASON_ERROR_STATE:
			result = append(result, session.ReasonErrorState)
		case sessionv1.AttentionReason_ATTENTION_REASON_IDLE_TIMEOUT:
			result = append(result, session.ReasonIdleTimeout)
		case sessionv1.AttentionReason_ATTENTION_REASON_TASK_COMPLETE:
			result = append(result, session.ReasonTaskComplete)
		}
	}
	return result
}

// formatDuration formats a time.Duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		if minutes == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd%dh", days, hours)
}
