package contextreduce

import (
	"testing"

	"github.com/mochow13/keen-code/internal/llm/core"
)

func TestReduceRemovesOldestTargetsUntilBudgetFits(t *testing.T) {
	var removed []int
	targets := []target{
		{tokens: 50, remove: func() { removed = append(removed, 0) }},
		{tokens: 100, remove: func() { removed = append(removed, 1) }},
		{tokens: 100, remove: func() { removed = append(removed, 2) }},
	}
	reduction := reduce(contextWindowForInputBudget(800), 850, targets)
	if !reduction.FitsBudget || reduction.RemovedToolResults != 2 {
		t.Fatalf("unexpected reduction: %#v", reduction)
	}
	want := 850 - 50 - 100 + 2*core.EstimateContextTokenCount(RemovedToolResultPlaceholder)
	if reduction.ReducedTokenCount != want || len(removed) != 2 || removed[0] != 0 || removed[1] != 1 {
		t.Fatalf("unexpected reduction result: %#v, removed=%v", reduction, removed)
	}
}

func TestReduceRemainsOverBudgetAfterRemovingAllTargets(t *testing.T) {
	removed := 0
	reduction := reduce(contextWindowForInputBudget(800), 1000, []target{
		{tokens: 50, remove: func() { removed++ }},
	})

	if reduction.FitsBudget {
		t.Fatal("expected reduced context to remain over budget")
	}
	if reduction.RemovedToolResults != 1 {
		t.Fatalf("expected 1 removed tool result, got %d", reduction.RemovedToolResults)
	}
	if removed != 1 {
		t.Fatalf("expected target removal to run once, got %d", removed)
	}
}

func TestReduceSkipsSmallTargets(t *testing.T) {
	removed := 0
	placeholder := core.EstimateContextTokenCount(RemovedToolResultPlaceholder)
	reduction := reduce(contextWindowForInputBudget(800), 801, []target{
		{tokens: 5, remove: func() { removed++ }},
		{tokens: placeholder + 1, remove: func() { removed++ }},
	})
	if !reduction.FitsBudget || reduction.RemovedToolResults != 1 || removed != 1 {
		t.Fatalf("unexpected reduction: %#v, removed=%d", reduction, removed)
	}
}

func contextWindowForInputBudget(budget int) int {
	if budget+4096 < 81920 {
		return budget + 4096
	}
	return (20*budget + 18) / 19
}
