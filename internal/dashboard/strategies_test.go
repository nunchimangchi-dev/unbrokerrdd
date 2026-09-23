package dashboard

import (
	"strings"
	"testing"
)

func TestStrategies_AreNumberedOneThroughSix(t *testing.T) {
	seen := map[int]bool{}
	for _, s := range Strategies() {
		if seen[s.N] {
			t.Errorf("duplicate strategy number %d", s.N)
		}
		seen[s.N] = true
		if s.Name == "" || s.Mechanism == "" {
			t.Errorf("strategy %d is missing a name or mechanism", s.N)
		}
	}
	for n := 1; n <= 6; n++ {
		if !seen[n] {
			t.Errorf("strategy %d is not described", n)
		}
	}
}

func TestStrategies_ObsoleteMustJustifyItself(t *testing.T) {
	// "Obsolete" exists so the next person does not rebuild a dead end and
	// rediscover the same wall. A bare label does not do that job — it has to
	// carry the reason and the measurement behind it, or deleting the strategy
	// outright would have been just as informative.
	for _, s := range Strategies() {
		if s.State != StrategyObsolete {
			continue
		}
		if len(s.Note) < 80 {
			t.Errorf("strategy %d is marked obsolete with a %d-char note; say what changed in the world",
				s.N, len(s.Note))
		}
		if s.Evidence == "" {
			t.Errorf("strategy %d is marked obsolete with no evidence; the claim must be re-checkable", s.N)
		}
		if !strings.Contains(s.Evidence, "2026-") {
			t.Errorf("strategy %d evidence has no date: %q", s.N, s.Evidence)
		}
	}
}

func TestStrategies_StatesAreKnown(t *testing.T) {
	valid := map[StrategyState]bool{
		StrategyBuilt: true, StrategyPartial: true,
		StrategyPlanned: true, StrategyObsolete: true,
	}
	for _, s := range Strategies() {
		if !valid[s.State] {
			t.Errorf("strategy %d has unknown state %q", s.N, s.State)
		}
	}
}

func TestStrategyByN(t *testing.T) {
	s, ok := StrategyByN(5)
	if !ok {
		t.Fatal("strategy 5 not found")
	}
	if s.State != StrategyObsolete {
		t.Errorf("strategy 5 state = %q, want obsolete", s.State)
	}
	if _, ok := StrategyByN(99); ok {
		t.Error("StrategyByN(99) should not resolve")
	}
}
