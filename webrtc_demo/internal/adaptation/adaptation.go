package adaptation

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

type FeedbackSnapshot struct {
	Timestamp        time.Time
	FractionLost     float64
	PacketsLostDelta int
	BurstLossRate    float64
	JitterSeconds    float64
	RTTSeconds       float64
	REMBBps          float64
	HasStats         bool
	HasBurstMetric   bool
	HasRR            bool
	HasREMB          bool
	HasTransportCC   bool
}

type RedundancyMode string

const (
	ModeOff        RedundancyMode = "off"
	ModeLBRRMedium RedundancyMode = "lbrr_medium"
	ModeLBRRHigh   RedundancyMode = "lbrr_high"
	ModeLBRRUltra  RedundancyMode = "lbrr_ultra"
	ModeDREDMedium RedundancyMode = "dred_medium"
	ModeDREDHigh   RedundancyMode = "dred_high"
	ModeDREDUltra  RedundancyMode = "dred_ultra"
)

type RedundancyDecision struct {
	Mode   RedundancyMode
	FEC    bool
	PLP    int
	DRED   int
	Reason string
}

type ControllerConfig struct {
	Alpha              float64
	Cooling            time.Duration
	PromoteConsecutive int
	DemoteConsecutive  int
	SupportsDRED       bool
}

type ControllerState struct {
	LossEMA      float64
	BurstEMA     float64
	JitterEMA    float64
	RTTEMA       float64
	LastMode     RedundancyMode
	LastApplyAt  time.Time
	PromoteCount int
	DemoteCount  int
}

type Collector struct {
	mu              sync.RWMutex
	lastRR          *FeedbackSnapshot
	lastREMBBps     float64
	hasREMB         bool
	lastTransportCC time.Time
}

func NewCollector() *Collector {
	return &Collector{}
}

func (c *Collector) ConsumeRTCP(pkts []rtcp.Packet) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, pkt := range pkts {
		switch p := pkt.(type) {
		case *rtcp.ReceiverReport:
			if len(p.Reports) == 0 {
				continue
			}
			rep := p.Reports[0]
			c.lastRR = &FeedbackSnapshot{
				FractionLost:  float64(rep.FractionLost) / 256.0,
				JitterSeconds: float64(rep.Jitter) / 48000.0,
				HasRR:         true,
			}
		case *rtcp.ReceiverEstimatedMaximumBitrate:
			c.lastREMBBps = float64(p.Bitrate)
			c.hasREMB = true
		case *rtcp.TransportLayerCC:
			c.lastTransportCC = time.Now()
		}
	}
}

func (c *Collector) Snapshot(report webrtc.StatsReport, prevPacketsLost *int32) FeedbackSnapshot {
	snap := FeedbackSnapshot{Timestamp: time.Now()}

	c.mu.RLock()
	if c.lastRR != nil {
		snap.FractionLost = c.lastRR.FractionLost
		snap.JitterSeconds = c.lastRR.JitterSeconds
		snap.HasRR = true
	}
	if c.hasREMB {
		snap.REMBBps = c.lastREMBBps
		snap.HasREMB = true
	}
	if !c.lastTransportCC.IsZero() && time.Since(c.lastTransportCC) < 10*time.Second {
		snap.HasTransportCC = true
	}
	c.mu.RUnlock()

	var (
		foundRemoteInbound bool
		foundCandidateRTT  bool
	)
	for _, stat := range report {
		switch s := stat.(type) {
		case webrtc.RemoteInboundRTPStreamStats:
			if s.Kind != "audio" {
				continue
			}
			snap.FractionLost = s.FractionLost
			snap.BurstLossRate = s.BurstLossRate
			snap.JitterSeconds = s.Jitter
			snap.RTTSeconds = s.RoundTripTime
			snap.HasStats = true
			snap.HasBurstMetric = true
			if prevPacketsLost != nil {
				delta := int(s.PacketsLost - *prevPacketsLost)
				if delta < 0 {
					delta = 0
				}
				snap.PacketsLostDelta = delta
				*prevPacketsLost = s.PacketsLost
			}
			foundRemoteInbound = true
		case webrtc.ICECandidatePairStats:
			if foundCandidateRTT || !s.Nominated {
				continue
			}
			if snap.RTTSeconds <= 0 {
				snap.RTTSeconds = s.CurrentRoundTripTime
			}
			foundCandidateRTT = true
		}
	}

	if !foundRemoteInbound {
		snap.HasBurstMetric = false
	}

	return snap
}

func DefaultControllerConfig() ControllerConfig {
	return ControllerConfig{
		Alpha:              0.35,
		Cooling:            3 * time.Second,
		PromoteConsecutive: 2,
		DemoteConsecutive:  5,
		SupportsDRED:       true,
	}
}

func decideMode(lossEMA, burstEMA float64, hasBurstMetric bool) RedundancyMode {
	if hasBurstMetric && lossEMA >= 0.08 && burstEMA >= 0.15 {
		switch {
		case lossEMA >= 0.18:
			return ModeDREDUltra
		case lossEMA >= 0.12:
			return ModeDREDHigh
		default:
			return ModeDREDMedium
		}
	}

	switch {
	case lossEMA >= 0.18:
		return ModeLBRRUltra
	case lossEMA >= 0.12:
		return ModeLBRRHigh
	case lossEMA >= 0.05:
		return ModeLBRRMedium
	default:
		return ModeOff
	}
}

func modeRank(mode RedundancyMode) int {
	switch mode {
	case ModeOff:
		return 0
	case ModeLBRRMedium, ModeDREDMedium:
		return 1
	case ModeLBRRHigh, ModeDREDHigh:
		return 2
	case ModeLBRRUltra, ModeDREDUltra:
		return 3
	default:
		return 0
	}
}

func sameFamily(a, b RedundancyMode) bool {
	return (isDREDMode(a) && isDREDMode(b)) || (!isDREDMode(a) && !isDREDMode(b))
}

func isDREDMode(mode RedundancyMode) bool {
	return mode == ModeDREDMedium || mode == ModeDREDHigh || mode == ModeDREDUltra
}

func applyREMBCap(mode RedundancyMode, rembBps float64, hasREMB bool) RedundancyMode {
	if !hasREMB {
		return mode
	}
	if rembBps < 16000 {
		switch {
		case isDREDMode(mode):
			return ModeDREDMedium
		case mode == ModeOff:
			return ModeOff
		default:
			return ModeLBRRMedium
		}
	}
	if rembBps < 24000 {
		if mode == ModeLBRRUltra {
			return ModeLBRRHigh
		}
		if mode == ModeDREDUltra {
			return ModeDREDHigh
		}
	}
	return mode
}

func modeDecision(mode RedundancyMode, reason string, supportsDRED bool) RedundancyDecision {
	switch mode {
	case ModeOff:
		return RedundancyDecision{Mode: mode, FEC: false, PLP: 0, DRED: 0, Reason: reason}
	case ModeLBRRMedium:
		return RedundancyDecision{Mode: mode, FEC: true, PLP: 10, DRED: 0, Reason: reason}
	case ModeLBRRHigh:
		return RedundancyDecision{Mode: mode, FEC: true, PLP: 15, DRED: 0, Reason: reason}
	case ModeLBRRUltra:
		return RedundancyDecision{Mode: mode, FEC: true, PLP: 20, DRED: 0, Reason: reason}
	case ModeDREDMedium:
		if !supportsDRED {
			return modeDecision(ModeLBRRMedium, reason+" (dred unavailable)", supportsDRED)
		}
		return RedundancyDecision{Mode: mode, FEC: false, PLP: 0, DRED: 3, Reason: reason}
	case ModeDREDHigh:
		if !supportsDRED {
			return modeDecision(ModeLBRRHigh, reason+" (dred unavailable)", supportsDRED)
		}
		return RedundancyDecision{Mode: mode, FEC: false, PLP: 0, DRED: 5, Reason: reason}
	case ModeDREDUltra:
		if !supportsDRED {
			return modeDecision(ModeLBRRUltra, reason+" (dred unavailable)", supportsDRED)
		}
		return RedundancyDecision{Mode: mode, FEC: false, PLP: 0, DRED: 10, Reason: reason}
	default:
		return RedundancyDecision{Mode: ModeOff, FEC: false, PLP: 0, DRED: 0, Reason: reason}
	}
}

func Observe(cfg ControllerConfig, state *ControllerState, snap FeedbackSnapshot) (RedundancyDecision, bool) {
	alpha := cfg.Alpha
	if alpha <= 0 || alpha > 1 {
		alpha = 0.35
	}

	if state.LastMode == "" {
		state.LossEMA = snap.FractionLost
		state.BurstEMA = snap.BurstLossRate
		state.JitterEMA = snap.JitterSeconds
		state.RTTEMA = snap.RTTSeconds
	} else {
		state.LossEMA = alpha*snap.FractionLost + (1-alpha)*state.LossEMA
		if snap.HasBurstMetric {
			state.BurstEMA = alpha*snap.BurstLossRate + (1-alpha)*state.BurstEMA
		}
		if snap.JitterSeconds > 0 {
			state.JitterEMA = alpha*snap.JitterSeconds + (1-alpha)*state.JitterEMA
		}
		if snap.RTTSeconds > 0 {
			state.RTTEMA = alpha*snap.RTTSeconds + (1-alpha)*state.RTTEMA
		}
	}

	target := decideMode(state.LossEMA, state.BurstEMA, snap.HasBurstMetric)
	target = applyREMBCap(target, snap.REMBBps, snap.HasREMB)

	if state.LastMode == "" {
		state.LastMode = target
		state.LastApplyAt = snap.Timestamp
		return modeDecision(
			target,
			fmt.Sprintf("initial loss=%.3f burst=%.3f jitter=%.3f rtt=%.3f remb=%.0f",
				state.LossEMA, state.BurstEMA, state.JitterEMA, state.RTTEMA, snap.REMBBps),
			cfg.SupportsDRED,
		), true
	}

	if target == state.LastMode {
		state.PromoteCount = 0
		state.DemoteCount = 0
		return modeDecision(
			state.LastMode,
			fmt.Sprintf("stable loss=%.3f burst=%.3f jitter=%.3f rtt=%.3f remb=%.0f",
				state.LossEMA, state.BurstEMA, state.JitterEMA, state.RTTEMA, snap.REMBBps),
			cfg.SupportsDRED,
		), false
	}

	candidateUp := modeRank(target) > modeRank(state.LastMode) || !sameFamily(target, state.LastMode)
	if candidateUp {
		state.PromoteCount++
		state.DemoteCount = 0
		if state.PromoteCount < cfg.PromoteConsecutive {
			return modeDecision(state.LastMode, "waiting promote hysteresis", cfg.SupportsDRED), false
		}
	} else {
		state.DemoteCount++
		state.PromoteCount = 0
		if state.DemoteCount < cfg.DemoteConsecutive {
			return modeDecision(state.LastMode, "waiting demote hysteresis", cfg.SupportsDRED), false
		}
	}

	if !state.LastApplyAt.IsZero() && snap.Timestamp.Sub(state.LastApplyAt) < cfg.Cooling {
		return modeDecision(state.LastMode, "cooling down", cfg.SupportsDRED), false
	}

	state.LastMode = target
	state.LastApplyAt = snap.Timestamp
	state.PromoteCount = 0
	state.DemoteCount = 0

	return modeDecision(
		target,
		fmt.Sprintf("switch loss=%.3f burst=%.3f jitter=%.3f rtt=%.3f remb=%.0f",
			state.LossEMA, state.BurstEMA, state.JitterEMA, state.RTTEMA, snap.REMBBps),
		cfg.SupportsDRED,
	), true
}

func AlmostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
