package core

import "testing"

func TestContextFitsBudget(t *testing.T) {
	if !ContextFitsBudget(13050, 700) {
		t.Fatal("expected context to fit budget")
	}
	if ContextFitsBudget(13050, 9000) {
		t.Fatal("expected context to exceed budget")
	}
}
