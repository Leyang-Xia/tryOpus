package adaptation

import (
	"testing"
	"time"
)

func snapshot(ts time.Time, loss, burst float64, hasBurst bool) FeedbackSnapshot {
	return FeedbackSnapshot{
		Timestamp:      ts,
		FractionLost:   loss,
		BurstLossRate:  burst,
		HasStats:       true,
		HasBurstMetric: hasBurst,
	}
}

func TestControllerPromotesToLBRRAndDRED(t *testing.T) {
	cfg := DefaultControllerConfig()
	cfg.Cooling = 0
	var state ControllerState
	now := time.Unix(0, 0)

	decision, changed := Observe(cfg, &state, snapshot(now, 0.01, 0.0, false))
	if !changed || decision.Mode != ModeOff {
		t.Fatalf("expected initial off, got changed=%v mode=%s", changed, decision.Mode)
	}

	decision, changed = Observe(cfg, &state, snapshot(now.Add(time.Second), 0.09, 0.0, false))
	if changed {
		t.Fatalf("should wait promote hysteresis")
	}
	decision, changed = Observe(cfg, &state, snapshot(now.Add(2*time.Second), 0.20, 0.0, false))
	if changed {
		t.Fatalf("should still wait promote hysteresis after first threshold crossing")
	}
	decision, changed = Observe(cfg, &state, snapshot(now.Add(3*time.Second), 0.20, 0.0, false))
	if !changed || decision.Mode != ModeLBRRHigh {
		t.Fatalf("expected lbrr high, got changed=%v mode=%s", changed, decision.Mode)
	}

	for i := 4; i < 9; i++ {
		decision, _ = Observe(cfg, &state, snapshot(now.Add(time.Duration(i)*time.Second), 0.18, 0.20, true))
	}
	if decision.Mode != ModeDREDHigh {
		t.Fatalf("expected dred high after burst-heavy transition, got %s", decision.Mode)
	}
}

func TestControllerFallsBackWhenDREDUnavailable(t *testing.T) {
	cfg := DefaultControllerConfig()
	cfg.Cooling = 0
	cfg.SupportsDRED = false
	var state ControllerState
	now := time.Unix(0, 0)

	Observe(cfg, &state, snapshot(now, 0.10, 0.20, true))
	Observe(cfg, &state, snapshot(now.Add(time.Second), 0.20, 0.20, true))
	decision, changed := Observe(cfg, &state, snapshot(now.Add(2*time.Second), 0.20, 0.20, true))
	if !changed {
		t.Fatalf("expected mode change")
	}
	if decision.Mode != ModeLBRRHigh || !decision.FEC || decision.DRED != 0 || decision.PLP != 15 {
		t.Fatalf("unexpected fallback decision: %+v", decision)
	}
}

func TestREMBClampsUltra(t *testing.T) {
	cfg := DefaultControllerConfig()
	cfg.Cooling = 0
	var state ControllerState
	now := time.Unix(0, 0)

	s1 := snapshot(now, 0.01, 0.0, false)
	s1.REMBBps = 20000
	s1.HasREMB = true
	Observe(cfg, &state, s1)

	s2 := snapshot(now.Add(time.Second), 0.30, 0.0, false)
	s2.REMBBps = 20000
	s2.HasREMB = true
	decision, changed := Observe(cfg, &state, s2)
	if changed {
		t.Fatalf("should still wait promote hysteresis")
	}
	s3 := snapshot(now.Add(2*time.Second), 0.30, 0.0, false)
	s3.REMBBps = 20000
	s3.HasREMB = true
	decision, changed = Observe(cfg, &state, s3)
	if !changed {
		t.Fatalf("expected changed mode")
	}
	if decision.Mode != ModeLBRRHigh {
		t.Fatalf("expected ultra clamped to high, got %s", decision.Mode)
	}
}
