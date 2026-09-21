package repl

import (
	"strings"
	"testing"

	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
)

func TestAutoPermissionHasOnlySingleOperationChoices(t *testing.T) {
	req := &replpermissions.Request{
		ToolName: "write_file", Path: "main.go", Status: replpermissions.StatusPending,
		SingleOperation: true,
	}
	sh := &StreamHandler{}
	sh.HandlePermissionRequest(req)
	if sh.GetPendingChoice() != replpermissions.ChoiceAllow {
		t.Fatal("first choice must allow one operation")
	}
	sh.MovePendingCursor(1)
	if sh.GetPendingChoice() != replpermissions.ChoiceDeny {
		t.Fatal("auto prompt must not offer a session grant")
	}
	sh.MovePendingCursor(100)
	if sh.GetPendingChoice() != replpermissions.ChoiceAskWhatToDo {
		t.Fatal("last choice must request direction")
	}
	card := strings.Join(renderPermissionCard(&streamSegment{permissionReq: req}, 80), "\n")
	if strings.Contains(card, "Allow for this session") || strings.Contains(card, "Dangerous Command") {
		t.Fatal("auto prompt has a session grant or an incorrect danger label")
	}
}
