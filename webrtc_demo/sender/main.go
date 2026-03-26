package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"opus_lab/webrtc_demo/internal/adaptation"
	"opus_lab/webrtc_demo/internal/opusx"
	"opus_lab/webrtc_demo/internal/rtc"
	"opus_lab/webrtc_demo/internal/signal"
	"opus_lab/webrtc_demo/internal/wav"
)

type adaptationSample struct {
	Timestamp     string  `json:"timestamp"`
	Loss          float64 `json:"loss"`
	Burst         float64 `json:"burst"`
	Jitter        float64 `json:"jitter"`
	RTT           float64 `json:"rtt"`
	REMBBps       float64 `json:"remb_bps"`
	Mode          string  `json:"mode"`
	FEC           bool    `json:"fec"`
	PLP           int     `json:"plp"`
	DRED          int     `json:"dred"`
	Changed       bool    `json:"changed"`
	HasBurst      bool    `json:"has_burst_metric"`
	HasTransport  bool    `json:"has_transport_cc"`
	Reason        string  `json:"reason"`
	BitrateBps    int     `json:"bitrate_bps"`
	BitrateTier   string  `json:"bitrate_tier"`
	DecisionClass string  `json:"decision_class"`
	DREDAllowed   bool    `json:"dred_allowed"`
	DREDLevelCap  string  `json:"dred_level_cap"`
}

type adaptationTrace struct {
	FeedbackIntervalSeconds float64            `json:"feedback_interval_seconds"`
	Samples                 []adaptationSample `json:"samples"`
}

type senderStats struct {
	PacketsSent          int               `json:"packets_sent"`
	BytesSent            int               `json:"bytes_sent"`
	AvgPayloadBytes      float64           `json:"avg_payload_bytes"`
	InputFrames          int               `json:"input_frames"`
	FrameDurationMS      int               `json:"frame_duration_ms"`
	ConfiguredBitrate    int               `json:"configured_bitrate_bps"`
	EffectivePayloadKbps float64           `json:"effective_payload_kbps"`
	PayloadHistogram     []payloadBucket   `json:"payload_histogram,omitempty"`
	DREDDebug            *dredDebugSummary `json:"dred_debug,omitempty"`
}

type payloadBucket struct {
	PayloadBytes int `json:"payload_bytes"`
	Count        int `json:"count"`
}

type dredDebugSummary struct {
	DurationConfigured       int `json:"duration_configured"`
	RequestedChunksCap       int `json:"requested_chunks_cap"`
	TargetChunksCapUnderRate int `json:"target_chunks_cap_under_rate"`
	Q0                       int `json:"q0"`
	DQ                       int `json:"dQ"`
	QMax                     int `json:"qmax"`
	EstimatedTargetBitrate   int `json:"estimated_target_bitrate_bps"`
	EstimatedAppliedBitrate  int `json:"estimated_applied_bitrate_bps"`
	FramesObserved           int `json:"frames_observed"`
	DistinctPayloadSizes     int `json:"distinct_payload_sizes"`
	MostCommonPayloadBytes   int `json:"most_common_payload_bytes"`
	MostCommonPayloadCount   int `json:"most_common_payload_count"`
}

var (
	dredBitsTable = []float64{73.2, 68.1, 62.5, 57.0, 51.5, 45.7, 39.9, 32.4, 26.4, 20.4, 16.3, 13.0, 9.3, 8.2, 7.2, 6.4}
	dredDQTable   = []int{0, 2, 3, 4, 6, 8, 12, 16}
)

func ecILog(v int) int {
	n := 0
	for v > 0 {
		n++
		v >>= 1
	}
	return n
}

func computeQuantizer(q0, dQ, qMax, i int) int {
	quant := q0 + (dredDQTable[dQ]*i+8)/16
	if quant > qMax {
		return qMax
	}
	return quant
}

func bitrateToBits(bitrate, sampleRate, frameSize int) int {
	return bitrate * 6 / (6 * sampleRate / frameSize)
}

func bitsToBitrate(bits, sampleRate, frameSize int) int {
	return bits * (6 * sampleRate / frameSize) / 6
}

func estimateDREDBitrate(q0, dQ, qMax, duration, targetBits int) (int, int, int) {
	requestedChunks := minInt((duration+5)/4, 26)
	bits := 8.0*(3.0+2.0) + 50.0 + dredBitsTable[q0]
	targetChunks := 0
	for i := 0; i < requestedChunks; i++ {
		q := computeQuantizer(q0, dQ, qMax, i)
		bits += dredBitsTable[q]
		if int(bits) < targetBits {
			targetChunks = i + 1
		}
	}
	return int(bits + 0.5), targetChunks, requestedChunks
}

func buildDREDDebugSummary(duration, bitrate, packetLoss, sampleRate, frameSize int, useFEC bool, payloadHistogram map[int]int, framesObserved int) *dredDebugSummary {
	if duration <= 0 {
		return nil
	}
	var dredFrac float64
	bitrateOffset := 12000
	if useFEC {
		dredFrac = minFloat(0.7, 3.0*float64(packetLoss)/100.0)
		bitrateOffset = 20000
	} else if packetLoss > 5 {
		dredFrac = minFloat(0.8, 0.55+float64(packetLoss)/100.0)
	} else {
		dredFrac = 12.0 * float64(packetLoss) / 100.0
	}
	dredFrac = dredFrac / (dredFrac + (1.0-dredFrac)*(float64(frameSize)*50.0)/float64(sampleRate))
	q0 := minInt(15, maxInt(4, 51-3*ecILog(maxInt(1, bitrate-bitrateOffset))))
	dQ := 5
	if bitrate-bitrateOffset > 36000 {
		dQ = 3
	}
	qMax := 15
	targetBitrate := maxInt(0, int(dredFrac*float64(bitrate-bitrateOffset)))
	targetBits := bitrateToBits(targetBitrate, sampleRate, frameSize)
	maxDREDBits, targetChunks, requestedChunks := estimateDREDBitrate(q0, dQ, qMax, duration, targetBits)
	appliedBitrate := minInt(targetBitrate, bitsToBitrate(maxDREDBits, sampleRate, frameSize))
	if targetChunks < 2 {
		appliedBitrate = 0
	}
	mostCommonPayloadBytes, mostCommonPayloadCount := 0, 0
	for size, count := range payloadHistogram {
		if count > mostCommonPayloadCount {
			mostCommonPayloadBytes = size
			mostCommonPayloadCount = count
		}
	}
	return &dredDebugSummary{
		DurationConfigured:       duration,
		RequestedChunksCap:       requestedChunks,
		TargetChunksCapUnderRate: targetChunks,
		Q0:                       q0,
		DQ:                       dQ,
		QMax:                     qMax,
		EstimatedTargetBitrate:   targetBitrate,
		EstimatedAppliedBitrate:  appliedBitrate,
		FramesObserved:           framesObserved,
		DistinctPayloadSizes:     len(payloadHistogram),
		MostCommonPayloadBytes:   mostCommonPayloadBytes,
		MostCommonPayloadCount:   mostCommonPayloadCount,
	}
}

func main() {
	var (
		signalURL          string
		sessionID          string
		inputWAV           string
		frameMS            int
		bitrate            int
		packetLoss         int
		enableFEC          bool
		enableVBR          bool
		complexity         int
		signalHint         string
		dredDuration       int
		dnnBlobPath        string
		connectWait        time.Duration
		signalWait         time.Duration
		adaptiveRedundancy bool
		feedbackInterval   time.Duration
		adaptWindow        time.Duration
		adaptLog           bool
		adaptationJSON     string
		senderStatsJSON    string
	)

	flag.StringVar(&signalURL, "signal", "http://127.0.0.1:8090", "signaling server base URL")
	flag.StringVar(&sessionID, "session", "", "session id (must match receiver)")
	flag.StringVar(&inputWAV, "input", "", "input WAV path (PCM16 mono 48k)")
	flag.IntVar(&frameMS, "frame-ms", 20, "frame duration in milliseconds")
	flag.IntVar(&bitrate, "bitrate", 32000, "opus bitrate in bps")
	flag.IntVar(&packetLoss, "plp", 10, "expected packet loss percentage for opus encoder")
	flag.BoolVar(&enableFEC, "fec", true, "enable opus in-band FEC")
	flag.BoolVar(&enableVBR, "vbr", true, "enable opus VBR")
	flag.IntVar(&complexity, "complexity", 9, "opus complexity (0-10)")
	flag.StringVar(&signalHint, "signal-hint", "auto", "opus signal hint: auto|voice|music")
	flag.IntVar(&dredDuration, "dred", 0, "opus dred duration in 10ms units, 0 to disable")
	flag.StringVar(&dnnBlobPath, "weights", "../weights_blob.bin", "path to DNN blob file for DRED")
	flag.DurationVar(&connectWait, "connect-timeout", 10*time.Second, "peer connection timeout")
	flag.DurationVar(&signalWait, "signal-timeout", 20*time.Second, "answer wait timeout")
	flag.BoolVar(&adaptiveRedundancy, "adaptive-redundancy", true, "adapt fec/dred redundancy from RTC feedback")
	flag.DurationVar(&feedbackInterval, "feedback-interval", time.Second, "feedback polling interval")
	flag.DurationVar(&adaptWindow, "adapt-window", 5*time.Second, "smoothing window duration")
	flag.BoolVar(&adaptLog, "adapt-log", true, "log adaptive redundancy feedback and switches")
	flag.StringVar(&adaptationJSON, "adaptation-json", "", "optional path to write adaptive control trace json")
	flag.StringVar(&senderStatsJSON, "sender-stats-json", "", "optional path to write sender payload stats json")
	flag.Parse()

	if sessionID == "" {
		log.Fatal("--session is required")
	}
	if inputWAV == "" {
		log.Fatal("--input is required")
	}
	if frameMS <= 0 {
		log.Fatal("--frame-ms must be > 0")
	}
	if feedbackInterval <= 0 {
		log.Fatal("--feedback-interval must be > 0")
	}
	if adaptWindow <= 0 {
		log.Fatal("--adapt-window must be > 0")
	}
	if dredDuration > 0 && packetLoss == 0 {
		packetLoss = 10
	}
	if dnnBlobPath != "" {
		if absPath, err := filepath.Abs(dnnBlobPath); err == nil {
			dnnBlobPath = absPath
		}
	}
	if adaptationJSON != "" {
		if absPath, err := filepath.Abs(adaptationJSON); err == nil {
			adaptationJSON = absPath
		}
	}
	if senderStatsJSON != "" {
		if absPath, err := filepath.Abs(senderStatsJSON); err == nil {
			senderStatsJSON = absPath
		}
	}

	audio, err := wav.ReadPCM16Mono(inputWAV)
	if err != nil {
		log.Fatalf("read input wav failed: %v", err)
	}
	if audio.SampleRate != 48000 {
		log.Fatalf("unsupported sample rate=%d, sender currently requires 48000Hz", audio.SampleRate)
	}
	log.Printf("sender using opus: %s", opusx.Version())

	client := signal.NewClient(signalURL)
	if _, err := client.CreateSession(sessionID); err != nil {
		log.Fatalf("create/get session failed: %v", err)
	}

	pc, err := rtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		log.Fatalf("init peer connection failed: %v", err)
	}
	defer func() {
		_ = pc.Close()
	}()

	connectedCh := make(chan struct{}, 1)
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("sender peer state: %s", state.String())
		if state == webrtc.PeerConnectionStateConnected {
			select {
			case connectedCh <- struct{}{}:
			default:
			}
		}
	})

	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
			ClockRate: 48000,
			Channels:  2,
		},
		"audio",
		"sender",
	)
	if err != nil {
		log.Fatalf("create local track failed: %v", err)
	}

	rtpSender, err := pc.AddTrack(track)
	if err != nil {
		log.Fatalf("add local track failed: %v", err)
	}

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		log.Fatalf("create offer failed: %v", err)
	}
	gatherDone := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		log.Fatalf("set local description failed: %v", err)
	}
	<-gatherDone

	localOffer := pc.LocalDescription()
	if err := client.PublishOffer(sessionID, signal.SDP{
		Type: localOffer.Type.String(),
		SDP:  localOffer.SDP,
	}); err != nil {
		log.Fatalf("publish offer failed: %v", err)
	}
	log.Printf("offer published, waiting answer...")

	answer, err := client.WaitAnswer(sessionID, signalWait)
	if err != nil {
		log.Fatalf("wait answer failed: %v", err)
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.NewSDPType(answer.Type),
		SDP:  answer.SDP,
	}); err != nil {
		log.Fatalf("set remote description failed: %v", err)
	}

	select {
	case <-connectedCh:
	case <-time.After(connectWait):
		log.Fatalf("peer connect timeout after %s", connectWait)
	}

	encoder, err := opusx.NewEncoder(48000, 2, opusx.AppVoIP)
	if err != nil {
		log.Fatalf("create opus encoder failed: %v", err)
	}
	defer encoder.Close()
	var encMu sync.Mutex

	if err := encoder.SetBitrate(bitrate); err != nil {
		log.Fatalf("set encoder bitrate failed: %v", err)
	}
	if err := encoder.SetComplexity(complexity); err != nil {
		log.Fatalf("set encoder complexity failed: %v", err)
	}
	switch signalHint {
	case "auto":
		if err := encoder.SetSignal(opusx.SignalAuto); err != nil {
			log.Fatalf("set signal hint failed: %v", err)
		}
	case "voice":
		if err := encoder.SetSignal(opusx.SignalVoice); err != nil {
			log.Fatalf("set signal hint failed: %v", err)
		}
	case "music":
		if err := encoder.SetSignal(opusx.SignalMusic); err != nil {
			log.Fatalf("set signal hint failed: %v", err)
		}
	default:
		log.Fatalf("unsupported --signal-hint=%s, expected auto|voice|music", signalHint)
	}
	if err := encoder.SetInBandFEC(enableFEC); err != nil {
		log.Fatalf("set in-band fec failed: %v", err)
	}
	if err := encoder.SetVBR(enableVBR); err != nil {
		log.Fatalf("set vbr failed: %v", err)
	}
	if err := encoder.SetPacketLossPerc(packetLoss); err != nil {
		log.Fatalf("set packet loss percentage failed: %v", err)
	}
	var (
		dredBlob     []byte
		supportsDRED = true
	)
	if adaptiveRedundancy || dredDuration > 0 {
		dredBlob, err = opusx.LoadDNNBlob(dnnBlobPath)
		if err != nil {
			log.Printf("warning: load dnn blob failed (%v), continue without adaptive DRED", err)
			supportsDRED = false
		}
		if len(dredBlob) > 0 {
			if err := encoder.SetDNNBlob(dredBlob); err != nil {
				log.Printf("warning: set encoder dnn blob failed (%v), continue without adaptive DRED", err)
				supportsDRED = false
			}
		}
		if supportsDRED {
			if err := encoder.SetDREDDuration(0); err != nil {
				if opusx.IsRequestNotImplemented(err) {
					log.Printf("warning: encoder DRED unsupported, adaptive loop will stay on LBRR-only: %v", err)
					supportsDRED = false
				} else {
					log.Fatalf("probe dred support failed: %v", err)
				}
			}
		}
	}
	if dredDuration > 0 {
		if err := encoder.SetDREDDuration(dredDuration); err != nil {
			log.Fatalf("set dred duration failed: %v", err)
		}
		if len(dredBlob) == 0 {
			dredBlob, err = opusx.LoadDNNBlob(dnnBlobPath)
			if err != nil {
				log.Fatalf("load dnn blob failed: %v", err)
			}
		}
		if err := encoder.SetDNNBlob(dredBlob); err != nil {
			log.Printf("warning: set encoder dnn blob failed (%v), continue", err)
		}
		log.Printf("sender DRED enabled: duration=%d, blob=%s", dredDuration, dnnBlobPath)
	}

	if adaptiveRedundancy {
		collector := adaptation.NewCollector()
		controllerCfg := adaptation.DefaultControllerConfig()
		controllerCfg.SupportsDRED = supportsDRED
		controllerCfg.BitrateBps = bitrate
		controllerCfg.Cooling = 3 * time.Second
		controllerCfg.PromoteConsecutive = maxInt(2, int((2*time.Second)/feedbackInterval))
		controllerCfg.DemoteConsecutive = maxInt(5, int((adaptWindow)/feedbackInterval))
		controllerCfg.DREDPromoteConsecutive = maxInt(2, controllerCfg.PromoteConsecutive)
		var (
			controllerState adaptation.ControllerState
			prevPacketsLost int32
			traceMu         sync.Mutex
			traceSamples    []adaptationSample
		)

		go func() {
			for {
				pkts, _, rtcpErr := rtpSender.ReadRTCP()
				if rtcpErr != nil {
					if adaptLog {
						log.Printf("adaptive rtcp reader stopped: %v", rtcpErr)
					}
					return
				}
				collector.ConsumeRTCP(pkts)
				if adaptLog {
					for _, pkt := range pkts {
						switch p := pkt.(type) {
						case *rtcp.ReceiverEstimatedMaximumBitrate:
							log.Printf("adaptive rtcp remb=%.0fbps", p.Bitrate)
						case *rtcp.TransportLayerCC:
							log.Printf("adaptive rtcp twcc packets=%d fb=%d", p.PacketStatusCount, p.FbPktCount)
						}
					}
				}
			}
		}()

		go func() {
			ticker := time.NewTicker(feedbackInterval)
			defer ticker.Stop()
			for range ticker.C {
				snap := collector.Snapshot(pc.GetStats(), &prevPacketsLost)
				decision, changed := adaptation.Observe(controllerCfg, &controllerState, snap)
				if adaptLog {
					log.Printf("adaptive snapshot bitrate=%d tier=%s class=%s loss=%.3f burst=%.3f jitter=%.3f rtt=%.3f remb=%.0f mode=%s reason=%s",
						decision.BitrateBps, decision.BitrateTier, decision.DecisionClass,
						snap.FractionLost, snap.BurstLossRate, snap.JitterSeconds, snap.RTTSeconds, snap.REMBBps, decision.Mode, decision.Reason)
				}
				traceMu.Lock()
				traceSamples = append(traceSamples, adaptationSample{
					Timestamp:     snap.Timestamp.Format(time.RFC3339Nano),
					Loss:          snap.FractionLost,
					Burst:         snap.BurstLossRate,
					Jitter:        snap.JitterSeconds,
					RTT:           snap.RTTSeconds,
					REMBBps:       snap.REMBBps,
					Mode:          string(decision.Mode),
					FEC:           decision.FEC,
					PLP:           decision.PLP,
					DRED:          decision.DRED,
					Changed:       changed,
					HasBurst:      snap.HasBurstMetric,
					HasTransport:  snap.HasTransportCC,
					Reason:        decision.Reason,
					BitrateBps:    decision.BitrateBps,
					BitrateTier:   decision.BitrateTier,
					DecisionClass: decision.DecisionClass,
					DREDAllowed:   decision.DREDAllowed,
					DREDLevelCap:  decision.DREDLevelCap,
				})
				traceMu.Unlock()
				if !changed {
					continue
				}

				encMu.Lock()
				if err := encoder.SetInBandFEC(decision.FEC); err != nil {
					encMu.Unlock()
					log.Printf("adaptive set fec failed: %v", err)
					continue
				}
				if err := encoder.SetPacketLossPerc(decision.PLP); err != nil {
					encMu.Unlock()
					log.Printf("adaptive set plp failed: %v", err)
					continue
				}
				if controllerCfg.SupportsDRED {
					if err := encoder.SetDREDDuration(decision.DRED); err != nil {
						encMu.Unlock()
						if opusx.IsRequestNotImplemented(err) {
							controllerCfg.SupportsDRED = false
							log.Printf("adaptive disabled dred after unsupported ctl: %v", err)
							continue
						}
						log.Printf("adaptive set dred failed: %v", err)
						continue
					}
				}
				encMu.Unlock()
				log.Printf("adaptive applied bitrate=%d tier=%s class=%s mode=%s fec=%v plp=%d dred=%d dred_allowed=%v dred_cap=%s reason=%s",
					decision.BitrateBps, decision.BitrateTier, decision.DecisionClass,
					decision.Mode, decision.FEC, decision.PLP, decision.DRED, decision.DREDAllowed, decision.DREDLevelCap, decision.Reason)
			}
		}()

		defer func() {
			if adaptationJSON == "" {
				return
			}
			traceMu.Lock()
			trace := adaptationTrace{
				FeedbackIntervalSeconds: feedbackInterval.Seconds(),
				Samples:                 append([]adaptationSample(nil), traceSamples...),
			}
			traceMu.Unlock()
			if err := os.MkdirAll(filepath.Dir(adaptationJSON), 0o755); err != nil {
				log.Printf("write adaptation trace mkdir failed: %v", err)
				return
			}
			data, err := json.MarshalIndent(trace, "", "  ")
			if err != nil {
				log.Printf("marshal adaptation trace failed: %v", err)
				return
			}
			if err := os.WriteFile(adaptationJSON, data, 0o644); err != nil {
				log.Printf("write adaptation trace failed: %v", err)
				return
			}
			log.Printf("adaptive trace saved: %s", adaptationJSON)
		}()
	} else {
		go func() {
			for {
				if _, _, readErr := rtpSender.ReadRTCP(); readErr != nil {
					return
				}
			}
		}()
	}

	samplesPerFrame := 48000 * frameMS / 1000
	if samplesPerFrame <= 0 {
		log.Fatalf("invalid samples per frame from frame-ms=%d", frameMS)
	}

	frameCount := len(audio.Samples) / samplesPerFrame
	if frameCount == 0 {
		log.Fatal("input wav is too short")
	}
	log.Printf("sending %d frames (%dms each) to session=%s", frameCount, frameMS, sessionID)

	packetBuf := make([]byte, 4000)
	stereoPCM := make([]int16, samplesPerFrame*2)
	frameDuration := time.Duration(frameMS) * time.Millisecond
	stats := senderStats{
		FrameDurationMS:   frameMS,
		ConfiguredBitrate: bitrate,
	}
	payloadHistogram := map[int]int{}
	for i := 0; i < frameCount; i++ {
		start := i * samplesPerFrame
		end := start + samplesPerFrame
		monoFrame := audio.Samples[start:end]
		for j, sample := range monoFrame {
			stereoPCM[j*2] = sample
			stereoPCM[j*2+1] = sample
		}

		encMu.Lock()
		n, encodeErr := encoder.Encode(stereoPCM, samplesPerFrame, packetBuf)
		encMu.Unlock()
		if encodeErr != nil {
			log.Fatalf("encode opus frame=%d failed: %v", i, encodeErr)
		}
		if writeErr := track.WriteSample(media.Sample{
			Data:     packetBuf[:n],
			Duration: frameDuration,
		}); writeErr != nil {
			log.Fatalf("write sample frame=%d failed: %v", i, writeErr)
		}
		stats.PacketsSent++
		stats.BytesSent += n
		payloadHistogram[n]++
		time.Sleep(frameDuration)
	}
	stats.InputFrames = frameCount
	if stats.PacketsSent > 0 {
		stats.AvgPayloadBytes = float64(stats.BytesSent) / float64(stats.PacketsSent)
	}
	totalSeconds := float64(frameCount*frameMS) / 1000.0
	if totalSeconds > 0 {
		stats.EffectivePayloadKbps = float64(stats.BytesSent*8) / totalSeconds / 1000.0
	}
	if len(payloadHistogram) > 0 {
		keys := make([]int, 0, len(payloadHistogram))
		for size := range payloadHistogram {
			keys = append(keys, size)
		}
		sort.Ints(keys)
		stats.PayloadHistogram = make([]payloadBucket, 0, len(keys))
		for _, size := range keys {
			stats.PayloadHistogram = append(stats.PayloadHistogram, payloadBucket{
				PayloadBytes: size,
				Count:        payloadHistogram[size],
			})
		}
	}
	stats.DREDDebug = buildDREDDebugSummary(dredDuration, bitrate, packetLoss, 48000, samplesPerFrame, enableFEC, payloadHistogram, frameCount)

	log.Printf("send complete: %d frames, input=%s", frameCount, inputWAV)
	if senderStatsJSON != "" {
		if err := os.MkdirAll(filepath.Dir(senderStatsJSON), 0o755); err != nil {
			log.Printf("write sender stats mkdir failed: %v", err)
		} else if data, err := json.MarshalIndent(stats, "", "  "); err != nil {
			log.Printf("marshal sender stats failed: %v", err)
		} else if err := os.WriteFile(senderStatsJSON, data, 0o644); err != nil {
			log.Printf("write sender stats failed: %v", err)
		} else {
			log.Printf("sender stats saved: %s", senderStatsJSON)
		}
	}
	time.Sleep(500 * time.Millisecond)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
