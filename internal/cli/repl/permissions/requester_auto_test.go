package permissions

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/tools"
)

type blockingReviewer struct {
	started chan struct{}
	release chan struct{}
}

func (r blockingReviewer) ReviewOperation(context.Context, tools.Operation) (tools.OperationReviewDecision, error) {
	close(r.started)
	<-r.release
	return tools.OperationReviewApproved, nil
}

func TestRequester_QueuedPromptHonorsCancellation(t *testing.T) {
	r := NewRequester(config.NewProjectPermissions())
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	firstDone := make(chan error, 1)
	go func() {
		_, err := r.RequestPermission(firstCtx, "read_file", "one", "one", false)
		firstDone <- err
	}()
	<-r.GetRequestChan()

	secondCtx, cancelSecond := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() {
		_, err := r.RequestPermission(secondCtx, "edit_file", "two", "two", false)
		secondDone <- err
	}()
	cancelSecond()
	if err := <-secondDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("queued request error = %v, want canceled", err)
	}
	select {
	case request := <-r.GetRequestChan():
		t.Fatalf("queued request reached UI: %#v", request)
	default:
	}
	cancelFirst()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("active request error = %v, want canceled", err)
	}
}

func TestRequester_LateResponseAfterCancellationDoesNotAllowOrGrant(t *testing.T) {
	r := NewRequester(config.NewProjectPermissions())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct {
		allowed bool
		err     error
	}, 1)
	go func() {
		allowed, err := r.RequestPermission(ctx, "read_file", "file", "file", false)
		done <- struct {
			allowed bool
			err     error
		}{allowed, err}
	}()
	<-r.GetRequestChan()
	cancel()
	result := <-done
	if result.allowed || !errors.Is(result.err, context.Canceled) {
		t.Fatalf("canceled request = (%v, %v), want false, canceled", result.allowed, result.err)
	}
	r.SendResponse(ChoiceAllowSession, "read_file")
	if r.IsSessionAllowed("read_file") {
		t.Fatal("late response created a session grant")
	}
}

func TestRequester_AutoPromptIgnoresBroadGrants(t *testing.T) {
	perms := config.NewProjectPermissions()
	perms.Allow["read_file"] = struct{}{}
	r := NewRequester(perms)
	r.SetAutoMode(true, nil)

	firstDone := make(chan bool, 1)
	go func() {
		allowed, _ := r.RequestPermission(context.Background(), "read_file", "file", "file", false)
		firstDone <- allowed
	}()
	request := <-r.GetRequestChan()
	if !request.SingleOperation {
		t.Fatal("auto fallback did not mark the prompt single-operation")
	}
	r.SendResponse(ChoiceAllowSession, "read_file")
	if !<-firstDone {
		t.Fatal("manual auto fallback was denied")
	}
	if r.IsSessionAllowed("read_file") {
		t.Fatal("auto fallback created a session grant")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	secondDone := make(chan error, 1)
	go func() {
		_, err := r.RequestPermission(ctx, "read_file", "file", "file", false)
		secondDone <- err
	}()
	<-r.GetRequestChan()
	cancel()
	if err := <-secondDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("second auto fallback error = %v, want canceled", err)
	}
}

func TestRequester_ReviewRejectsApprovalAfterModeTransition(t *testing.T) {
	reviewer := blockingReviewer{started: make(chan struct{}), release: make(chan struct{})}
	r := NewRequester(config.NewProjectPermissions())
	r.SetAutoMode(true, reviewer)
	done := make(chan tools.OperationReviewDecision, 1)
	go func() {
		decision, _ := r.ReviewOperation(context.Background(), tools.Operation{Kind: "bash"})
		done <- decision
	}()
	<-reviewer.started
	r.SetAutoMode(false, nil)
	close(reviewer.release)
	if got := <-done; got != tools.OperationReviewAskUser {
		t.Fatalf("stale review decision = %v, want ask user", got)
	}
}

func TestRequester_MismatchedResponseDoesNotResolvePrompt(t *testing.T) {
	r := NewRequester(config.NewProjectPermissions())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan bool, 1)
	go func() {
		allowed, _ := r.RequestPermission(ctx, "read_file", "file", "file", false)
		done <- allowed
	}()
	<-r.GetRequestChan()
	r.SendResponse(ChoiceAllow, "edit_file")
	select {
	case allowed := <-done:
		t.Fatalf("mismatched response resolved request: %v", allowed)
	case <-time.After(20 * time.Millisecond):
	}
	r.SendResponse(ChoiceAllow, "read_file")
	if !<-done {
		t.Fatal("matching response did not allow request")
	}
}

func TestRequester_StaleRequestIDDoesNotResolveReplacementPrompt(t *testing.T) {
	r := NewRequester(config.NewProjectPermissions())
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := r.RequestPermission(firstCtx, "read_file", "first", "first", false)
		firstDone <- err
	}()
	first := <-r.GetRequestChan()
	cancelFirst()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("first request error = %v, want canceled", err)
	}

	secondDone := make(chan bool, 1)
	go func() {
		allowed, _ := r.RequestPermission(context.Background(), "read_file", "second", "second", false)
		secondDone <- allowed
	}()
	second := <-r.GetRequestChan()
	r.SendResponseFor(first.RequestID, ChoiceAllow, "read_file")
	select {
	case allowed := <-secondDone:
		t.Fatalf("stale response resolved replacement request: %v", allowed)
	case <-time.After(20 * time.Millisecond):
	}
	r.SendResponseFor(second.RequestID, ChoiceAllow, "read_file")
	if !<-secondDone {
		t.Fatal("matching request ID did not allow replacement request")
	}
}

func TestRequester_ConcurrentStateAccess(t *testing.T) {
	r := NewAutoApproveRequester()
	var group sync.WaitGroup
	for range 20 {
		group.Add(1)
		go func() {
			defer group.Done()
			r.SetYoloMode(true)
			_, _ = r.RequestPermission(context.Background(), "read_file", "file", "file", false)
			r.SetYoloMode(false)
			r.ResetSessionPermissions()
			_ = r.IsSessionAllowed("read_file")
		}()
	}
	group.Wait()
}
