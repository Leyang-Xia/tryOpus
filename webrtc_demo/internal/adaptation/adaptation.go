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
	Mode          RedundancyMode
	FEC           bool
	PLP           int
	DRED          int
	Reason        string
	BitrateBps    int
	BitrateTier   string
	DecisionClass string
	DREDAllowed   bool
	DREDLevelCap  string
}

type ControllerConfig struct {
	Alpha                   float64
	Cooling                 time.Duration
	PromoteConsecutive      int
	DemoteConsecutive       int
	DREDPromoteConsecutive  int
	DREDHighPromote         int
	LowBitrateDREDPromote   int
	BurstThreshold          float64
	BurstLossRatioThreshold float64
	SupportsDRED            bool
	BitrateBps              int
	LowBitrateThresholdBps  int
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
	lastBurstRate   float64
	hasBurstRate    bool
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
			if burstRate, ok := estimateBurstLossRateFromTWCC(p); ok {
				c.lastBurstRate = burstRate
				c.hasBurstRate = true
			}
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
	if c.hasBurstRate {
		snap.BurstLossRate = c.lastBurstRate
		snap.HasBurstMetric = true
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
			if s.BurstLossRate > 0 {
				snap.BurstLossRate = s.BurstLossRate
				snap.HasBurstMetric = true
			}
			snap.JitterSeconds = s.Jitter
			snap.RTTSeconds = s.RoundTripTime
			snap.HasStats = true
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

	if !foundRemoteInbound && !snap.HasBurstMetric {
		snap.HasBurstMetric = false
	}

	return snap
}

func estimateBurstLossRateFromTWCC(pkt *rtcp.TransportLayerCC) (float64, bool) {
	statuses, ok := decodeTWCCStatuses(pkt)
	if !ok || len(statuses) == 0 {
		return 0, false
	}

	totalPackets := len(statuses)
	totalLost := 0
	burstLost := 0
	currentLostRun := 0

	flushRun := func() {
		if currentLostRun >= 2 {
			burstLost += currentLostRun
		}
		currentLostRun = 0
	}

	for _, lost := range statuses {
		if lost {
			totalLost++
			currentLostRun++
			continue
		}
		flushRun()
	}
	flushRun()

	if totalLost == 0 {
		return 0, true
	}

	return float64(burstLost) / float64(totalPackets), true
}

func decodeTWCCStatuses(pkt *rtcp.TransportLayerCC) ([]bool, bool) {
	if pkt == nil || pkt.PacketStatusCount == 0 {
		return nil, false
	}

	statuses := make([]bool, 0, pkt.PacketStatusCount)
	remaining := int(pkt.PacketStatusCount)

	appendStatus := func(lost bool) {
		if remaining <= 0 {
			return
		}
		statuses = append(statuses, lost)
		remaining--
	}

	for _, rawChunk := range pkt.PacketChunks {
		if remaining <= 0 {
			break
		}
		switch chunk := rawChunk.(type) {
		case *rtcp.RunLengthChunk:
			lost := chunk.PacketStatusSymbol == rtcp.TypeTCCPacketNotReceived
			runLength := int(chunk.RunLength)
			if runLength > remaining {
				runLength = remaining
			}
			for i := 0; i < runLength; i++ {
				appendStatus(lost)
			}
		case *rtcp.StatusVectorChunk:
			for _, symbol := range chunk.SymbolList {
				appendStatus(symbol == rtcp.TypeTCCPacketNotReceived)
				if remaining <= 0 {
					break
				}
			}
		}
	}

	if len(statuses) == 0 {
		return nil, false
	}

	return statuses, true
}

func DefaultControllerConfig() ControllerConfig {
	return ControllerConfig{
		Alpha:                   0.35,
		Cooling:                 3 * time.Second,
		PromoteConsecutive:      2,
		DemoteConsecutive:       5,
		DREDPromoteConsecutive:  2,
		DREDHighPromote:         1,
		LowBitrateDREDPromote:   3,
		BurstThreshold:          0.18,
		BurstLossRatioThreshold: 0.70,
		SupportsDRED:            true,
		BitrateBps:              32000,
		LowBitrateThresholdBps:  24000,
	}
}

type decisionContext struct {
	Mode          RedundancyMode
	DecisionClass string
	BitrateTier   string
	DREDAllowed   bool
	DREDLevelCap  string
}

func bitrateTier(cfg ControllerConfig) string {
	threshold := cfg.LowBitrateThresholdBps
	if threshold <= 0 {
		threshold = 24000
	}
	if cfg.BitrateBps > 0 && cfg.BitrateBps < threshold {
		return "<24k"
	}
	return "24k+"
}

func decideLBRRMode(lossEMA float64) RedundancyMode {
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

func isBurstHeavy(lossEMA, burstEMA float64, hasBurstMetric bool, cfg ControllerConfig) bool {
	if !hasBurstMetric || lossEMA < 0.08 || burstEMA < cfg.BurstThreshold {
		return false
	}
	if lossEMA <= 0 {
		return false
	}
	return (burstEMA / lossEMA) >= cfg.BurstLossRatioThreshold
}

func shouldPreferDRED(lossEMA, burstEMA float64, hasBurstMetric bool) bool {
	if lossEMA >= 0.15 {
		return true
	}
	if !hasBurstMetric {
		return false
	}
	return lossEMA >= 0.10 && burstEMA >= 0.10
}

func shouldPromoteToDREDHigh(lossEMA, burstEMA float64, hasBurstMetric bool, cfg ControllerConfig) bool {
	if !hasBurstMetric {
		return lossEMA >= 0.22
	}
	if lossEMA >= 0.18 && burstEMA >= 0.18 {
		return true
	}
	if burstEMA >= 0.25 {
		return true
	}
	if lossEMA <= 0 {
		return false
	}
	return burstEMA >= cfg.BurstThreshold && (burstEMA/lossEMA) >= (cfg.BurstLossRatioThreshold+0.10)
}

func shouldAllowLowBitrateDRED(lossEMA, burstEMA float64, hasBurstMetric bool) bool {
	if !hasBurstMetric || lossEMA <= 0 {
		return false
	}
	if lossEMA < 0.20 || burstEMA < 0.25 {
		return false
	}
	return (burstEMA / lossEMA) >= 0.80
}

func maxDREDCap(cfg ControllerConfig, rembBps float64, hasREMB bool) RedundancyMode {
	tier := bitrateTier(cfg)
	if tier == "<24k" {
		if hasREMB && rembBps > 0 && rembBps < 16000 {
			return ModeOff
		}
		return ModeDREDMedium
	}
	if !hasREMB {
		return ModeDREDUltra
	}
	if rembBps < 16000 {
		return ModeDREDMedium
	}
	if rembBps < 24000 {
		return ModeDREDHigh
	}
	return ModeDREDUltra
}

func applyREMBCap(mode RedundancyMode, lossEMA, rembBps float64, hasREMB bool, cfg ControllerConfig) RedundancyMode {
	if !hasREMB {
		return mode
	}
	if bitrateTier(cfg) == "<24k" && rembBps < 16000 {
		if isDREDMode(mode) {
			if lossEMA >= 0.12 {
				return ModeLBRRHigh
			}
			return ModeLBRRMedium
		}
		if mode == ModeLBRRUltra {
			return ModeLBRRHigh
		}
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

func capDREDMode(mode, cap RedundancyMode) RedundancyMode {
	if !isDREDMode(mode) {
		return mode
	}
	if cap == ModeOff {
		return ModeOff
	}
	if modeRank(mode) > modeRank(cap) {
		return cap
	}
	return mode
}

func decideContext(lossEMA, burstEMA float64, hasBurstMetric bool, snap FeedbackSnapshot, cfg ControllerConfig) decisionContext {
	tier := bitrateTier(cfg)
	ctx := decisionContext{
		BitrateTier:   tier,
		DecisionClass: "lbrr-first",
		DREDAllowed:   false,
		DREDLevelCap:  string(maxDREDCap(cfg, snap.REMBBps, snap.HasREMB)),
	}

	if tier == "<24k" {
		if shouldAllowLowBitrateDRED(lossEMA, burstEMA, hasBurstMetric) {
			ctx.Mode = capDREDMode(ModeDREDMedium, maxDREDCap(cfg, snap.REMBBps, snap.HasREMB))
			ctx.DecisionClass = "burst-heavy"
			ctx.DREDAllowed = ctx.Mode != ModeOff && isDREDMode(ctx.Mode)
		} else {
			ctx.Mode = decideLBRRMode(lossEMA)
		}
		ctx.Mode = applyREMBCap(ctx.Mode, lossEMA, snap.REMBBps, snap.HasREMB, cfg)
		return ctx
	}

	if isBurstHeavy(lossEMA, burstEMA, hasBurstMetric, cfg) {
		ctx.DecisionClass = "burst-heavy"
		ctx.DREDAllowed = true
		if shouldPromoteToDREDHigh(lossEMA, burstEMA, hasBurstMetric, cfg) {
			ctx.Mode = capDREDMode(ModeDREDHigh, maxDREDCap(cfg, snap.REMBBps, snap.HasREMB))
			ctx.Mode = applyREMBCap(ctx.Mode, lossEMA, snap.REMBBps, snap.HasREMB, cfg)
			return ctx
		}
		ctx.Mode = capDREDMode(ModeDREDMedium, maxDREDCap(cfg, snap.REMBBps, snap.HasREMB))
		ctx.Mode = applyREMBCap(ctx.Mode, lossEMA, snap.REMBBps, snap.HasREMB, cfg)
		return ctx
	}

	if shouldPreferDRED(lossEMA, burstEMA, hasBurstMetric) {
		ctx.Mode = capDREDMode(ModeDREDMedium, maxDREDCap(cfg, snap.REMBBps, snap.HasREMB))
		ctx.DecisionClass = "dred-preferred"
		ctx.DREDAllowed = ctx.Mode != ModeOff && isDREDMode(ctx.Mode)
		ctx.Mode = applyREMBCap(ctx.Mode, lossEMA, snap.REMBBps, snap.HasREMB, cfg)
		return ctx
	}

	ctx.Mode = applyREMBCap(decideLBRRMode(lossEMA), lossEMA, snap.REMBBps, snap.HasREMB, cfg)
	return ctx
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

func modeDecision(mode RedundancyMode, reason string, supportsDRED bool, ctx decisionContext, bitrateBps int) RedundancyDecision {
	switch mode {
	case ModeOff:
		return RedundancyDecision{Mode: mode, FEC: false, PLP: 0, DRED: 0, Reason: reason, BitrateBps: bitrateBps, BitrateTier: ctx.BitrateTier, DecisionClass: ctx.DecisionClass, DREDAllowed: ctx.DREDAllowed, DREDLevelCap: ctx.DREDLevelCap}
	case ModeLBRRMedium:
		return RedundancyDecision{Mode: mode, FEC: true, PLP: 10, DRED: 0, Reason: reason, BitrateBps: bitrateBps, BitrateTier: ctx.BitrateTier, DecisionClass: ctx.DecisionClass, DREDAllowed: ctx.DREDAllowed, DREDLevelCap: ctx.DREDLevelCap}
	case ModeLBRRHigh:
		return RedundancyDecision{Mode: mode, FEC: true, PLP: 15, DRED: 0, Reason: reason, BitrateBps: bitrateBps, BitrateTier: ctx.BitrateTier, DecisionClass: ctx.DecisionClass, DREDAllowed: ctx.DREDAllowed, DREDLevelCap: ctx.DREDLevelCap}
	case ModeLBRRUltra:
		return RedundancyDecision{Mode: mode, FEC: true, PLP: 20, DRED: 0, Reason: reason, BitrateBps: bitrateBps, BitrateTier: ctx.BitrateTier, DecisionClass: ctx.DecisionClass, DREDAllowed: ctx.DREDAllowed, DREDLevelCap: ctx.DREDLevelCap}
	case ModeDREDMedium:
		if !supportsDRED {
			return modeDecision(ModeLBRRMedium, reason+" (dred unavailable)", supportsDRED, ctx, bitrateBps)
		}
		return RedundancyDecision{Mode: mode, FEC: false, PLP: 10, DRED: 3, Reason: reason, BitrateBps: bitrateBps, BitrateTier: ctx.BitrateTier, DecisionClass: ctx.DecisionClass, DREDAllowed: ctx.DREDAllowed, DREDLevelCap: ctx.DREDLevelCap}
	case ModeDREDHigh:
		if !supportsDRED {
			return modeDecision(ModeLBRRHigh, reason+" (dred unavailable)", supportsDRED, ctx, bitrateBps)
		}
		return RedundancyDecision{Mode: mode, FEC: false, PLP: 15, DRED: 5, Reason: reason, BitrateBps: bitrateBps, BitrateTier: ctx.BitrateTier, DecisionClass: ctx.DecisionClass, DREDAllowed: ctx.DREDAllowed, DREDLevelCap: ctx.DREDLevelCap}
	case ModeDREDUltra:
		if !supportsDRED {
			return modeDecision(ModeLBRRUltra, reason+" (dred unavailable)", supportsDRED, ctx, bitrateBps)
		}
		return RedundancyDecision{Mode: mode, FEC: false, PLP: 20, DRED: 10, Reason: reason, BitrateBps: bitrateBps, BitrateTier: ctx.BitrateTier, DecisionClass: ctx.DecisionClass, DREDAllowed: ctx.DREDAllowed, DREDLevelCap: ctx.DREDLevelCap}
	default:
		return RedundancyDecision{Mode: ModeOff, FEC: false, PLP: 0, DRED: 0, Reason: reason, BitrateBps: bitrateBps, BitrateTier: ctx.BitrateTier, DecisionClass: ctx.DecisionClass, DREDAllowed: ctx.DREDAllowed, DREDLevelCap: ctx.DREDLevelCap}
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

	ctx := decideContext(state.LossEMA, state.BurstEMA, snap.HasBurstMetric, snap, cfg)
	target := ctx.Mode

	if state.LastMode == "" {
		if ctx.BitrateTier == "<24k" && isDREDMode(target) {
			ctx.Mode = decideLBRRMode(state.LossEMA)
			ctx.DecisionClass = "lbrr-first"
			ctx.DREDAllowed = false
			target = ctx.Mode
		}
		if target == ModeDREDUltra {
			target = ModeDREDMedium
			ctx.Mode = target
		}
		state.LastMode = target
		state.LastApplyAt = snap.Timestamp
		return modeDecision(
			target,
			fmt.Sprintf("initial loss=%.3f burst=%.3f jitter=%.3f rtt=%.3f remb=%.0f",
				state.LossEMA, state.BurstEMA, state.JitterEMA, state.RTTEMA, snap.REMBBps),
			cfg.SupportsDRED,
			ctx,
			cfg.BitrateBps,
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
			ctx,
			cfg.BitrateBps,
		), false
	}

	candidateUp := modeRank(target) > modeRank(state.LastMode) || !sameFamily(target, state.LastMode)
	if candidateUp {
		state.PromoteCount++
		state.DemoteCount = 0
		requiredPromote := cfg.PromoteConsecutive
		if isDREDMode(target) && !isDREDMode(state.LastMode) && cfg.DREDPromoteConsecutive > requiredPromote {
			requiredPromote = cfg.DREDPromoteConsecutive
		}
		if target == ModeDREDHigh && state.LastMode == ModeDREDMedium && cfg.DREDHighPromote < requiredPromote {
			requiredPromote = cfg.DREDHighPromote
		}
		if ctx.BitrateTier == "<24k" && isDREDMode(target) && cfg.LowBitrateDREDPromote > requiredPromote {
			requiredPromote = cfg.LowBitrateDREDPromote
		}
		if state.PromoteCount < requiredPromote {
			return modeDecision(state.LastMode, "waiting promote hysteresis", cfg.SupportsDRED, ctx, cfg.BitrateBps), false
		}
	} else {
		state.DemoteCount++
		state.PromoteCount = 0
		if state.DemoteCount < cfg.DemoteConsecutive {
			return modeDecision(state.LastMode, "waiting demote hysteresis", cfg.SupportsDRED, ctx, cfg.BitrateBps), false
		}
	}

	if !state.LastApplyAt.IsZero() && snap.Timestamp.Sub(state.LastApplyAt) < cfg.Cooling {
		return modeDecision(state.LastMode, "cooling down", cfg.SupportsDRED, ctx, cfg.BitrateBps), false
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
		ctx,
		cfg.BitrateBps,
	), true
}

func AlmostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
