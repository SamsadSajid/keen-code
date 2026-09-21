package permissions

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/tools"
)

type Status string

const (
	StatusPending            Status = "pending"
	StatusAllowed            Status = "allowed"
	StatusAllowedSession     Status = "allowed_session"
	StatusDenied             Status = "denied"
	StatusAutoAllowedSession Status = "auto_allowed_session"
	StatusRedirected         Status = "redirected"
)

var requestCounter uint64

type Request struct {
	RequestID       string
	ToolName        string
	Path            string
	ResolvedPath    string
	IsDangerous     bool
	SingleOperation bool
	Preview         string
	PreviewKind     string
	AutoApproved    bool
	Status          Status
	ResponseChan    chan bool
	done            <-chan struct{}
	allowSession    bool
}

type Requester struct {
	stateMu             sync.RWMutex
	sessionMu           sync.RWMutex
	pendingMu           sync.RWMutex
	promptSem           chan struct{}
	requestChan         chan *Request
	pending             *Request
	sessionAllowedTools map[string]bool
	autoApprove         bool
	yoloMode            bool
	autoMode            bool
	reviewer            tools.OperationReviewer
	autoGeneration      uint64
	projectPerms        *config.ProjectPermissions
}

func NewRequester(projectPerms *config.ProjectPermissions) *Requester {
	r := &Requester{
		requestChan:         make(chan *Request, 1),
		promptSem:           make(chan struct{}, 1),
		sessionAllowedTools: make(map[string]bool),
		projectPerms:        projectPerms,
	}
	r.promptSem <- struct{}{}
	return r
}

func NewAutoApproveRequester() *Requester {
	r := NewRequester(nil)
	r.autoApprove = true
	return r
}

func (r *Requester) SetYoloMode(enabled bool) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	r.yoloMode = enabled
}

func (r *Requester) SetAutoMode(enabled bool, reviewer tools.OperationReviewer) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	r.autoMode = enabled
	r.reviewer = reviewer
	r.autoGeneration++
}

func (r *Requester) AutoModeEnabled() bool {
	r.stateMu.RLock()
	defer r.stateMu.RUnlock()
	return r.autoMode
}

func (r *Requester) ReviewOperation(ctx context.Context, operation tools.Operation) (tools.OperationReviewDecision, error) {
	r.stateMu.RLock()
	autoMode, reviewer, generation := r.autoMode, r.reviewer, r.autoGeneration
	r.stateMu.RUnlock()
	if !autoMode {
		return tools.OperationReviewNotRequired, nil
	}
	if reviewer == nil {
		return tools.OperationReviewAskUser, nil
	}
	decision, err := reviewer.ReviewOperation(ctx, operation)
	r.stateMu.RLock()
	current := r.autoMode && r.autoGeneration == generation
	r.stateMu.RUnlock()
	if !current {
		return tools.OperationReviewAskUser, nil
	}
	return decision, err
}

func (r *Requester) RequestPermission(ctx context.Context, toolName, path, resolvedPath string, isDangerous bool) (bool, error) {
	return r.withPrompt(ctx, func() (bool, error) {
		return r.requestPermission(ctx, toolName, path, resolvedPath, isDangerous, false)
	})
}

// RequestManualPermission skips stored broad grants after auto review.
func (r *Requester) RequestManualPermission(ctx context.Context, toolName, path, resolvedPath string, isDangerous bool) (bool, error) {
	return r.withPrompt(ctx, func() (bool, error) {
		return r.requestPermission(ctx, toolName, path, resolvedPath, isDangerous, true)
	})
}

func (r *Requester) requestPermission(ctx context.Context, toolName, path, resolvedPath string, isDangerous, forcePrompt bool) (bool, error) {
	r.stateMu.RLock()
	yoloMode, autoMode := r.yoloMode, r.autoMode
	r.stateMu.RUnlock()
	if yoloMode {
		return true, nil
	}
	if r.autoApprove {
		return true, nil
	}

	forcePrompt = forcePrompt || autoMode
	if !forcePrompt && r.projectPerms != nil && r.projectPerms.Allow.Contains(toolName) {
		return true, nil
	}

	r.sessionMu.RLock()
	sessionAllowed := r.sessionAllowedTools[toolName]
	r.sessionMu.RUnlock()
	if !forcePrompt && !isDangerous && sessionAllowed {
		return true, nil
	}

	id := atomic.AddUint64(&requestCounter, 1)
	req := &Request{
		RequestID:       fmt.Sprintf("%d", id),
		ToolName:        toolName,
		Path:            path,
		ResolvedPath:    resolvedPath,
		IsDangerous:     isDangerous,
		SingleOperation: forcePrompt,
		Status:          StatusPending,
		ResponseChan:    make(chan bool, 1),
		done:            ctx.Done(),
		allowSession:    !forcePrompt,
	}

	r.pendingMu.Lock()
	r.pending = req
	r.pendingMu.Unlock()

	select {
	case r.requestChan <- req:
		select {
		case response := <-req.ResponseChan:
			r.clearPending(req)
			if err := ctx.Err(); err != nil {
				return false, err
			}
			return response, nil
		case <-ctx.Done():
			r.clearPending(req)
			return false, ctx.Err()
		}
	case <-ctx.Done():
		r.clearPending(req)
		return false, ctx.Err()
	}
}

func (r *Requester) withPrompt(ctx context.Context, request func() (bool, error)) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-r.promptSem:
	}
	defer func() { r.promptSem <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return request()
}

func (r *Requester) clearPending(request *Request) {
	r.pendingMu.Lock()
	defer r.pendingMu.Unlock()
	if r.pending == request {
		r.pending = nil
	}
}

func (r *Requester) GetRequestChan() <-chan *Request {
	return r.requestChan
}

func (r *Requester) SendResponse(choice Choice, toolName string) {
	r.pendingMu.RLock()
	pending := r.pending
	requestID := ""
	if pending != nil {
		requestID = pending.RequestID
	}
	r.pendingMu.RUnlock()
	r.SendResponseFor(requestID, choice, toolName)
}

// SendResponseFor resolves one matching pending request.
func (r *Requester) SendResponseFor(requestID string, choice Choice, toolName string) {
	r.pendingMu.RLock()
	pending := r.pending
	if pending == nil || pending.RequestID != requestID || pending.ToolName != toolName {
		r.pendingMu.RUnlock()
		return
	}
	select {
	case <-pending.done:
		r.pendingMu.RUnlock()
		return
	default:
	}
	allowed := choice == ChoiceAllow || choice == ChoiceAllowSession
	select {
	case pending.ResponseChan <- allowed:
	default:
		r.pendingMu.RUnlock()
		return
	}
	r.pendingMu.RUnlock()

	if choice == ChoiceAllowSession && !pending.IsDangerous && pending.allowSession {
		select {
		case <-pending.done:
			return
		default:
		}
		r.sessionMu.Lock()
		r.sessionAllowedTools[toolName] = true
		r.sessionMu.Unlock()
	}
}

func (r *Requester) HasPendingRequest() bool {
	r.pendingMu.RLock()
	defer r.pendingMu.RUnlock()
	return r.pending != nil
}

func (r *Requester) IsSessionAllowed(toolName string) bool {
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()
	return r.sessionAllowedTools[toolName]
}

func (r *Requester) ResetSessionPermissions() {
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()
	r.sessionAllowedTools = make(map[string]bool)
}
