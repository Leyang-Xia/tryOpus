package adaptation

import (
	"testing"
	"time"

	"github.com/pion/rtcp"
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

	for i := 4; i < 11; i++ {
		decision, _ = Observe(cfg, &state, snapshot(now.Add(time.Duration(i)*time.Second), 0.22, 0.30, true))
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

	decision, changed := Observe(cfg, &state, snapshot(now, 0.20, 0.25, true))
	if !changed {
		t.Fatalf("expected initial mode change")
	}
	if decision.Mode != ModeLBRRHigh || !decision.FEC || decision.DRED != 0 || decision.PLP != 15 {
		t.Fatalf("unexpected fallback decision: %+v", decision)
	}
}

func TestControllerPrefersDRED5OnStartupForHeavyLoss(t *testing.T) {
	cfg := DefaultControllerConfig()
	cfg.Cooling = 0
	var state ControllerState
	now := time.Unix(0, 0)

	decision, changed := Observe(cfg, &state, snapshot(now, 0.20, 0.20, true))
	if !changed {
		t.Fatalf("expected initial decision")
	}
	if decision.Mode != ModeDREDHigh {
		t.Fatalf("expected startup to prefer dred high, got %s", decision.Mode)
	}
}

func TestControllerPromotesToDRED5OnStartupForHeavyBurst(t *testing.T) {
	cfg := DefaultControllerConfig()
	cfg.Cooling = 0
	var state ControllerState
	now := time.Unix(0, 0)

	decision, changed := Observe(cfg, &state, snapshot(now, 0.20, 0.28, true))
	if !changed {
		t.Fatalf("expected initial decision")
	}
	if decision.Mode != ModeDREDHigh {
		t.Fatalf("expected heavy burst startup to go directly to dred high, got %s", decision.Mode)
	}
}

func TestControllerAvoidsDREDOnUniformLikeBurstRatio(t *testing.T) {
	cfg := DefaultControllerConfig()
	cfg.Cooling = 0
	var state ControllerState
	now := time.Unix(0, 0)

	Observe(cfg, &state, snapshot(now, 0.12, 0.04, true))
	for i := 1; i < 5; i++ {
		decision, _ := Observe(cfg, &state, snapshot(now.Add(time.Duration(i)*time.Second), 0.12, 0.06, true))
		if isDREDMode(decision.Mode) {
			t.Fatalf("expected uniform-like loss to stay on lbrr, got %s", decision.Mode)
		}
	}
}

func TestControllerPrefersDRED3ForHighUniformLoss(t *testing.T) {
	cfg := DefaultControllerConfig()
	cfg.Cooling = 0
	var state ControllerState
	now := time.Unix(0, 0)

	decision, changed := Observe(cfg, &state, snapshot(now, 0.18, 0.0, false))
	if !changed || decision.Mode != ModeDREDMedium {
		t.Fatalf("expected high uniform loss to prefer dred medium, got changed=%v mode=%s", changed, decision.Mode)
	}
}

func TestREMBClampsUltra(t *testing.T) {
	cfg := DefaultControllerConfig()
	cfg.Cooling = 0
	var state ControllerState
	now := time.Unix(0, 0)

	s1 := snapshot(now, 0.30, 0.40, true)
	s1.REMBBps = 15000
	s1.HasREMB = true
	decision, changed := Observe(cfg, &state, s1)
	if !changed {
		t.Fatalf("expected changed mode")
	}
	if decision.Mode != ModeDREDMedium {
		t.Fatalf("expected low remb to clamp to dred medium, got %s", decision.Mode)
	}
}

func TestLowBitrateHighUniformLossStaysOnLBRR(t *testing.T) {
	cfg := DefaultControllerConfig()
	cfg.Cooling = 0
	cfg.BitrateBps = 16000
	var state ControllerState
	now := time.Unix(0, 0)

	decision, changed := Observe(cfg, &state, snapshot(now, 0.22, 0.0, false))
	if !changed || decision.Mode != ModeLBRRUltra {
		t.Fatalf("expected low bitrate startup to stay on LBRR ultra, got changed=%v mode=%s", changed, decision.Mode)
	}
}

func TestLowBitrateBurstNeedsThreeSamplesAndCapsAtDRED3(t *testing.T) {
	cfg := DefaultControllerConfig()
	cfg.Cooling = 0
	cfg.BitrateBps = 16000
	var state ControllerState
	now := time.Unix(0, 0)

	decision, _ := Observe(cfg, &state, snapshot(now, 0.22, 0.0, false))
	if decision.Mode != ModeLBRRUltra {
		t.Fatalf("expected initial lbrr ultra, got %s", decision.Mode)
	}
	for i := 1; i <= 2; i++ {
		decision, _ = Observe(cfg, &state, snapshot(now.Add(time.Duration(i)*time.Second), 0.25, 0.90, true))
		if isDREDMode(decision.Mode) {
			t.Fatalf("expected low bitrate dred to require more promote windows, got %s at step=%d", decision.Mode, i)
		}
	}
	decision, changed := Observe(cfg, &state, snapshot(now.Add(3*time.Second), 0.25, 0.90, true))
	if !changed || decision.Mode != ModeDREDMedium {
		t.Fatalf("expected low bitrate burst to cap at dred medium after hysteresis, got changed=%v mode=%s", changed, decision.Mode)
	}
}

func TestDRED3PromotesQuicklyToDRED5ForPersistentHeavyBurst(t *testing.T) {
	cfg := DefaultControllerConfig()
	cfg.Cooling = 0
	var state ControllerState
	now := time.Unix(0, 0)

	decision, changed := Observe(cfg, &state, snapshot(now, 0.14, 0.12, true))
	if !changed || decision.Mode != ModeDREDMedium {
		t.Fatalf("expected startup to enter dred medium, got changed=%v mode=%s", changed, decision.Mode)
	}

	decision, changed = Observe(cfg, &state, snapshot(now.Add(time.Second), 0.22, 0.30, true))
	if !changed || decision.Mode != ModeDREDHigh {
		t.Fatalf("expected persistent heavy burst to promote quickly to dred high, got changed=%v mode=%s", changed, decision.Mode)
	}
}

func TestLowBitrateREMBDisablesDRED(t *testing.T) {
	cfg := DefaultControllerConfig()
	cfg.Cooling = 0
	cfg.BitrateBps = 16000
	var state ControllerState
	now := time.Unix(0, 0)

	first := snapshot(now, 0.25, 0.30, true)
	first.REMBBps = 15000
	first.HasREMB = true
	decision, changed := Observe(cfg, &state, first)
	if !changed {
		t.Fatalf("expected changed mode")
	}
	if isDREDMode(decision.Mode) {
		t.Fatalf("expected low bitrate + low remb to stay out of dred, got %s", decision.Mode)
	}
}

func TestEstimateBurstLossRateFromTWCC(t *testing.T) {
	pkt := &rtcp.TransportLayerCC{
		PacketStatusCount: 10,
		PacketChunks: []rtcp.PacketStatusChunk{
			&rtcp.RunLengthChunk{PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta, RunLength: 2},
			&rtcp.RunLengthChunk{PacketStatusSymbol: rtcp.TypeTCCPacketNotReceived, RunLength: 3},
			&rtcp.RunLengthChunk{PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta, RunLength: 2},
			&rtcp.RunLengthChunk{PacketStatusSymbol: rtcp.TypeTCCPacketNotReceived, RunLength: 1},
			&rtcp.RunLengthChunk{PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta, RunLength: 2},
		},
	}

	burstRate, ok := estimateBurstLossRateFromTWCC(pkt)
	if !ok {
		t.Fatalf("expected valid burst rate")
	}
	if !AlmostEqual(burstRate, 0.3) {
		t.Fatalf("expected burst rate 0.3, got %.6f", burstRate)
	}
}

func TestCollectorUsesTWCCBurstFallback(t *testing.T) {
	collector := NewCollector()
	collector.ConsumeRTCP([]rtcp.Packet{
		&rtcp.TransportLayerCC{
			PacketStatusCount: 8,
			PacketChunks: []rtcp.PacketStatusChunk{
				&rtcp.RunLengthChunk{PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta, RunLength: 2},
				&rtcp.RunLengthChunk{PacketStatusSymbol: rtcp.TypeTCCPacketNotReceived, RunLength: 2},
				&rtcp.RunLengthChunk{PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta, RunLength: 4},
			},
		},
	})

	snap := collector.Snapshot(nil, nil)
	if !snap.HasTransportCC {
		t.Fatalf("expected transport cc marker")
	}
	if !snap.HasBurstMetric {
		t.Fatalf("expected burst metric from twcc fallback")
	}
	if !AlmostEqual(snap.BurstLossRate, 0.25) {
		t.Fatalf("expected burst rate 0.25, got %.6f", snap.BurstLossRate)
	}
}
