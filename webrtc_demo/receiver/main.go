package main

import (
	"encoding/json"
	"flag"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"

	"opus_lab/webrtc_demo/internal/opusx"
	"opus_lab/webrtc_demo/internal/rtc"
	"opus_lab/webrtc_demo/internal/signal"
	"opus_lab/webrtc_demo/internal/wav"
)

type decodeStats struct {
	PacketsReceived   int     `json:"packets_received"`
	PacketsLost       int     `json:"packets_lost"`
	PacketsSimDropped int     `json:"packets_sim_dropped"`
	PacketsGapLost    int     `json:"packets_gap_lost"`
	Bytes             int     `json:"bytes"`
	DecodedFrames     int     `json:"decoded_frames"`
	RecoveredLBRR     int     `json:"recovered_lbrr"`
	RecoveredDRED     int     `json:"recovered_dred"`
	PLCFrames         int     `json:"plc_frames"`
	DecodeErrors      int     `json:"decode_errors"`
	OutputSamples     int     `json:"output_samples"`
	RecoveryRate      float64 `json:"recovery_rate"`
}

type lossSimulator struct {
	rng        *rand.Rand
	uniform    float64
	useGE      bool
	pGoodToBad float64
	pBadToGood float64
	lossInBad  float64
	inBad      bool
}

func newLossSimulator(seed int64, uniform float64, useGE bool, pGoodToBad float64, pBadToGood float64, lossInBad float64) *lossSimulator {
	return &lossSimulator{
		rng:        rand.New(rand.NewSource(seed)),
		uniform:    uniform,
		useGE:      useGE,
		pGoodToBad: pGoodToBad,
		pBadToGood: pBadToGood,
		lossInBad:  lossInBad,
	}
}

func (s *lossSimulator) shouldDrop() bool {
	if s.useGE {
		if s.inBad {
			if s.rng.Float64() < s.pBadToGood {
				s.inBad = false
			}
		} else if s.rng.Float64() < s.pGoodToBad {
			s.inBad = true
		}
		if s.inBad {
			return s.rng.Float64() < s.lossInBad
		}
		return false
	}
	if s.uniform <= 0 {
		return false
	}
	return s.rng.Float64() < s.uniform
}

func seqGap(prev uint16, curr uint16) int {
	diff := int(curr - prev)
	if diff <= 0 {
		diff += 1 << 16
	}
	if diff <= 1 || diff > 3000 {
		return 0
	}
	return diff - 1
}

func main() {
	var (
		signalURL     string
		sessionID     string
		outputWAV     string
		statsJSON     string
		duration      time.Duration
		signalWait    time.Duration
		frameMS       int
		useLBRR       bool
		useDRED       bool
		dnnBlobPath   string
		simLoss       float64
		simGE         bool
		simGEP2B      float64
		simGEB2G      float64
		simGELossBad  float64
		simDelayMS    float64
		simJitterMS   float64
		simSeed       int64
		detectGapLoss bool
	)

	flag.StringVar(&signalURL, "signal", "http://127.0.0.1:8090", "signaling server base URL")
	flag.StringVar(&sessionID, "session", "", "session id (must match sender)")
	flag.StringVar(&outputWAV, "output", "received.wav", "output WAV path")
	flag.StringVar(&statsJSON, "stats-json", "", "optional output stats json path")
	flag.DurationVar(&duration, "duration", 10*time.Second, "receive duration after connected")
	flag.DurationVar(&signalWait, "signal-timeout", 20*time.Second, "offer wait timeout")
	flag.IntVar(&frameMS, "frame-ms", 20, "frame duration in milliseconds")
	flag.BoolVar(&useLBRR, "use-lbrr", true, "use in-band FEC(lbrr) recovery")
	flag.BoolVar(&useDRED, "use-dred", true, "use DRED recovery")
	flag.StringVar(&dnnBlobPath, "weights", "../weights_blob.bin", "path to DNN blob file for DRED")
	flag.Float64Var(&simLoss, "sim-loss", 0, "uniform simulated packet loss rate [0,1]")
	flag.BoolVar(&simGE, "sim-ge", false, "enable Gilbert-Elliott simulated packet loss")
	flag.Float64Var(&simGEP2B, "sim-ge-p2b", 0.05, "GE: good to bad transition probability")
	flag.Float64Var(&simGEB2G, "sim-ge-b2g", 0.30, "GE: bad to good transition probability")
	flag.Float64Var(&simGELossBad, "sim-ge-bloss", 0.80, "GE: bad state loss probability")
	flag.Float64Var(&simDelayMS, "sim-delay-ms", 0, "simulated base delay before decode")
	flag.Float64Var(&simJitterMS, "sim-jitter-ms", 0, "simulated delay jitter stddev before decode")
	flag.Int64Var(&simSeed, "sim-seed", 42, "simulator random seed")
	flag.BoolVar(&detectGapLoss, "detect-gap-loss", true, "infer packet loss from RTP sequence gaps")
	flag.Parse()

	if sessionID == "" {
		log.Fatal("--session is required")
	}
	if frameMS <= 0 {
		log.Fatal("--frame-ms must be > 0")
	}
	if dnnBlobPath != "" {
		if absPath, err := filepath.Abs(dnnBlobPath); err == nil {
			dnnBlobPath = absPath
		}
	}

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
	log.Printf("receiver using opus: %s", opusx.Version())

	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		log.Fatalf("add recvonly transceiver failed: %v", err)
	}

	var (
		statsMu sync.Mutex
		stats   decodeStats

		samplesMu sync.Mutex
		samples   []int16
		doneCh    = make(chan struct{}, 1)
	)

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("receiver peer state: %s", state.String())
	})

	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		log.Printf("receiver got remote track codec=%s", track.Codec().MimeType)
		decoder, decErr := opusx.NewDecoder(48000, 2)
		if decErr != nil {
			log.Printf("create opus decoder failed: %v", decErr)
			select {
			case doneCh <- struct{}{}:
			default:
			}
			return
		}
		defer decoder.Close()

		var blob []byte
		if useDRED {
			blob, decErr = opusx.LoadDNNBlob(dnnBlobPath)
			if decErr != nil {
				log.Printf("warning: load dnn blob failed (%v), continue without external blob", decErr)
			}
		}
		if len(blob) > 0 {
			if err := decoder.SetDNNBlob(blob); err != nil {
				log.Printf("warning: set decoder dnn blob failed (%v), continue", err)
			}
		}
		if useDRED {
			if err := decoder.EnableDRED(blob); err != nil {
				log.Printf("enable DRED failed, fallback without DRED: %v", err)
				useDRED = false
			}
		}

		sim := newLossSimulator(simSeed, simLoss, simGE, simGEP2B, simGEB2G, simGELossBad)
		frameSize := 48000 * frameMS / 1000

		pcmBuf := make([]int16, frameSize*2)
		pendingLost := make([]int, 0, 8)
		pktIndex := 0
		hasPrevSeq := false
		var prevSeq uint16

		recoverLostFrame := func(payload []byte, currentIndex int, lostIndex int, isLastLost bool) {
			recovered := false
			if useLBRR && isLastLost {
				n, err := decoder.Decode(payload, frameSize, true, pcmBuf)
				if err == nil && n > 0 {
					statsMu.Lock()
					stats.RecoveredLBRR++
					stats.DecodedFrames++
					statsMu.Unlock()
					out := stereoToMono(pcmBuf, n)
					samplesMu.Lock()
					samples = append(samples, out...)
					samplesMu.Unlock()
					recovered = true
				}
			}

			if !recovered && useDRED {
				maxNeed := (currentIndex - lostIndex) * frameSize
				if maxNeed > 48000 {
					maxNeed = 48000
				}
				parsed, err := decoder.ParseDRED(payload, maxNeed)
				if err == nil && parsed > 0 {
					offset := (currentIndex - lostIndex) * frameSize
					n, derr := decoder.DecodeDRED(offset, frameSize, pcmBuf)
					if derr == nil && n > 0 {
						statsMu.Lock()
						stats.RecoveredDRED++
						stats.DecodedFrames++
						statsMu.Unlock()
						out := stereoToMono(pcmBuf, n)
						samplesMu.Lock()
						samples = append(samples, out...)
						samplesMu.Unlock()
						recovered = true
					}
				}
			}

			if !recovered {
				n, err := decoder.DecodePLC(frameSize, pcmBuf)
				if err != nil || n <= 0 {
					statsMu.Lock()
					stats.DecodeErrors++
					statsMu.Unlock()
					return
				}
				statsMu.Lock()
				stats.PLCFrames++
				stats.DecodedFrames++
				statsMu.Unlock()
				out := stereoToMono(pcmBuf, n)
				samplesMu.Lock()
				samples = append(samples, out...)
				samplesMu.Unlock()
			}
		}

		applyDecodeDelay := func() {
			delay := simDelayMS
			if simJitterMS > 0 {
				delay += sim.rng.NormFloat64() * simJitterMS
			}
			if delay > 0 {
				time.Sleep(time.Duration(delay * float64(time.Millisecond)))
			}
		}

		for {
			pkt, _, readErr := track.ReadRTP()
			if readErr != nil {
				break
			}

			applyDecodeDelay()

			if detectGapLoss && hasPrevSeq {
				gap := seqGap(prevSeq, pkt.SequenceNumber)
				if gap > 0 {
					for g := 0; g < gap; g++ {
						pendingLost = append(pendingLost, pktIndex)
						pktIndex++
					}
					statsMu.Lock()
					stats.PacketsLost += gap
					stats.PacketsGapLost += gap
					statsMu.Unlock()
				}
			}
			prevSeq = pkt.SequenceNumber
			hasPrevSeq = true

			if sim.shouldDrop() {
				pendingLost = append(pendingLost, pktIndex)
				pktIndex++
				statsMu.Lock()
				stats.PacketsLost++
				stats.PacketsSimDropped++
				statsMu.Unlock()
				continue
			}

			statsMu.Lock()
			stats.PacketsReceived++
			stats.Bytes += len(pkt.Payload)
			statsMu.Unlock()

			currentIdx := pktIndex
			pktIndex++

			if len(pendingLost) > 0 {
				lastIdx := len(pendingLost) - 1
				for i, lost := range pendingLost {
					recoverLostFrame(pkt.Payload, currentIdx, lost, i == lastIdx)
				}
				pendingLost = pendingLost[:0]
			}

			n, err := decoder.Decode(pkt.Payload, frameSize, false, pcmBuf)
			if err != nil || n <= 0 {
				statsMu.Lock()
				stats.DecodeErrors++
				statsMu.Unlock()
				continue
			}

			statsMu.Lock()
			stats.DecodedFrames++
			statsMu.Unlock()
			out := stereoToMono(pcmBuf, n)
			samplesMu.Lock()
			samples = append(samples, out...)
			samplesMu.Unlock()
		}

		// 收尾：最后一段丢失帧没有“下一包”可用于LBRR/DRED，全部PLC
		for range pendingLost {
			n, err := decoder.DecodePLC(frameSize, pcmBuf)
			if err != nil || n <= 0 {
				statsMu.Lock()
				stats.DecodeErrors++
				statsMu.Unlock()
				continue
			}
			statsMu.Lock()
			stats.PLCFrames++
			stats.DecodedFrames++
			statsMu.Unlock()
			out := stereoToMono(pcmBuf, n)
			samplesMu.Lock()
			samples = append(samples, out...)
			samplesMu.Unlock()
		}

		select {
		case doneCh <- struct{}{}:
		default:
		}
	})

	log.Printf("waiting offer for session=%s ...", sessionID)
	offer, err := client.WaitOffer(sessionID, signalWait)
	if err != nil {
		log.Fatalf("wait offer failed: %v", err)
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.NewSDPType(offer.Type),
		SDP:  offer.SDP,
	}); err != nil {
		log.Fatalf("set remote description failed: %v", err)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		log.Fatalf("create answer failed: %v", err)
	}
	gatherDone := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		log.Fatalf("set local description failed: %v", err)
	}
	<-gatherDone

	localAnswer := pc.LocalDescription()
	if err := client.PublishAnswer(sessionID, signal.SDP{
		Type: localAnswer.Type.String(),
		SDP:  localAnswer.SDP,
	}); err != nil {
		log.Fatalf("publish answer failed: %v", err)
	}
	log.Printf("answer published, receiving for %s ...", duration)

	timer := time.NewTimer(duration)
	select {
	case <-timer.C:
	case <-doneCh:
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}

	_ = pc.Close()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
	}

	samplesMu.Lock()
	outSamples := make([]int16, len(samples))
	copy(outSamples, samples)
	samplesMu.Unlock()

	if err := wav.WritePCM16Mono(outputWAV, 48000, outSamples); err != nil {
		log.Fatalf("write output wav failed: %v", err)
	}

	statsMu.Lock()
	stats.OutputSamples = len(outSamples)
	recoveredTotal := stats.RecoveredLBRR + stats.RecoveredDRED
	if stats.PacketsLost > 0 {
		stats.RecoveryRate = float64(recoveredTotal) / float64(stats.PacketsLost)
	}
	finalStats := stats
	statsMu.Unlock()
	log.Printf(
		"receiver done: recv=%d lost=%d(sim=%d,gap=%d) recovered(lbrr=%d,dred=%d) plc=%d decode_err=%d output_samples=%d output=%s",
		finalStats.PacketsReceived,
		finalStats.PacketsLost,
		finalStats.PacketsSimDropped,
		finalStats.PacketsGapLost,
		finalStats.RecoveredLBRR,
		finalStats.RecoveredDRED,
		finalStats.PLCFrames,
		finalStats.DecodeErrors,
		finalStats.OutputSamples,
		outputWAV,
	)

	if statsJSON != "" {
		data, err := json.MarshalIndent(finalStats, "", "  ")
		if err != nil {
			log.Fatalf("marshal stats json failed: %v", err)
		}
		if err := os.WriteFile(statsJSON, data, 0o644); err != nil {
			log.Fatalf("write stats json failed: %v", err)
		}
		log.Printf("stats json saved: %s", statsJSON)
	}
}

func stereoToMono(stereo []int16, perChannelSamples int) []int16 {
	need := perChannelSamples * 2
	if need > len(stereo) {
		need = len(stereo) - (len(stereo) % 2)
	}
	mono := make([]int16, need/2)
	for i := 0; i+1 < need; i += 2 {
		left := int(stereo[i])
		right := int(stereo[i+1])
		avg := (left + right) / 2
		if avg > math.MaxInt16 {
			avg = math.MaxInt16
		} else if avg < math.MinInt16 {
			avg = math.MinInt16
		}
		mono[i/2] = int16(avg)
	}
	return mono
}
