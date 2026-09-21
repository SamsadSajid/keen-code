package tools

import "context"

// Operation describes one validated tool effect for an approval review.
type Operation struct {
	Kind    string
	Path    string
	Command string
	Content string
	Exists  bool
	Bytes   int
}

type OperationReviewDecision uint8

const (
	OperationReviewNotRequired OperationReviewDecision = iota
	OperationReviewApproved
	OperationReviewAskUser
)

// OperationReviewer decides whether auto mode can run one operation.
type OperationReviewer interface {
	ReviewOperation(context.Context, Operation) (OperationReviewDecision, error)
}

func reviewOperation(ctx context.Context, requester PermissionRequester, operation Operation) (OperationReviewDecision, error) {
	reviewer, ok := requester.(OperationReviewer)
	if !ok {
		return OperationReviewNotRequired, nil
	}
	return reviewer.ReviewOperation(ctx, operation)
}

type ManualPermissionRequester interface {
	RequestManualPermission(context.Context, string, string, string, bool) (bool, error)
}

func requestManualPermission(ctx context.Context, requester PermissionRequester, toolName, path, resolvedPath string, dangerous bool) (bool, error) {
	if manual, ok := requester.(ManualPermissionRequester); ok {
		return manual.RequestManualPermission(ctx, toolName, path, resolvedPath, dangerous)
	}
	return requester.RequestPermission(ctx, toolName, path, resolvedPath, dangerous)
}

func autoModeEnabled(requester PermissionRequester) bool {
	state, ok := requester.(interface{ AutoModeEnabled() bool })
	return ok && state.AutoModeEnabled()
}

type PermissionRequester interface {
	RequestPermission(ctx context.Context, toolName, path, resolvedPath string, isDangerous bool) (bool, error)
}
