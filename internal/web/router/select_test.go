package router

import (
	"testing"

	"github.com/foxzi/sendry/internal/web/config"
)

func newTestRouter(strategy string, weights map[string]int) *EmailRouter {
	return NewEmailRouter(RouterConfig{
		MultiSend: &config.MultiSendConfig{Strategy: strategy, Weights: weights},
	})
}

func TestSelectServerWeighted(t *testing.T) {
	servers := []ServerInfo{{Name: "a", Weight: 3}, {Name: "b", Weight: 1}, {Name: "c", Weight: 0}}

	for _, strategy := range []string{config.StrategyWeighted, config.StrategyWeightedRoundRobin} {
		r := newTestRouter(strategy, nil)
		counts := map[string]int{}
		for i := 0; i < 8; i++ {
			counts[r.selectServer("example.com", servers).Name]++
		}
		if counts["a"] != 6 || counts["b"] != 2 || counts["c"] != 0 {
			t.Errorf("%s: unexpected distribution %v", strategy, counts)
		}
	}
}

func TestSelectServerWeightedAllZero(t *testing.T) {
	servers := []ServerInfo{{Name: "a", Weight: 0}, {Name: "b", Weight: 0}}
	r := newTestRouter(config.StrategyWeighted, nil)

	// Must not panic (division by zero) and must still pick servers
	counts := map[string]int{}
	for i := 0; i < 4; i++ {
		counts[r.selectServer("example.com", servers).Name]++
	}
	if counts["a"] != 2 || counts["b"] != 2 {
		t.Errorf("unexpected distribution %v", counts)
	}
}

func TestSelectServerDomainAffinityFallsBackToRoundRobin(t *testing.T) {
	servers := []ServerInfo{{Name: "a", Weight: 1}, {Name: "b", Weight: 1}}
	r := newTestRouter(config.StrategyDomainAffinity, nil)

	first := r.selectServer("example.com", servers).Name
	second := r.selectServer("example.com", servers).Name
	if first == second {
		t.Errorf("expected round robin, got %s twice", first)
	}
}
