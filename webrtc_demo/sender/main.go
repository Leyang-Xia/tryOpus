package main

import (
	"flag"
	"log"
	"path/filepath"
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
		controllerCfg.Cooling = 3 * time.Second
		controllerCfg.PromoteConsecutive = maxInt(2, int((2*time.Second)/feedbackInterval))
		controllerCfg.DemoteConsecutive = maxInt(5, int((adaptWindow)/feedbackInterval))
		var (
			controllerState adaptation.ControllerState
			prevPacketsLost int32
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
					log.Printf("adaptive snapshot loss=%.3f burst=%.3f jitter=%.3f rtt=%.3f remb=%.0f mode=%s reason=%s",
						snap.FractionLost, snap.BurstLossRate, snap.JitterSeconds, snap.RTTSeconds, snap.REMBBps, decision.Mode, decision.Reason)
				}
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
				log.Printf("adaptive applied mode=%s fec=%v plp=%d dred=%d reason=%s",
					decision.Mode, decision.FEC, decision.PLP, decision.DRED, decision.Reason)
			}
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
		time.Sleep(frameDuration)
	}

	log.Printf("send complete: %d frames, input=%s", frameCount, inputWAV)
	time.Sleep(500 * time.Millisecond)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
