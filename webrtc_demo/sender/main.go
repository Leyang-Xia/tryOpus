package main

import (
	"flag"
	"log"
	"time"

	"github.com/hraban/opus"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"opus_lab/webrtc_demo/internal/rtc"
	"opus_lab/webrtc_demo/internal/signal"
	"opus_lab/webrtc_demo/internal/wav"
)

func main() {
	var (
		signalURL   string
		sessionID   string
		inputWAV    string
		frameMS     int
		bitrate     int
		packetLoss  int
		enableFEC   bool
		connectWait time.Duration
		signalWait  time.Duration
	)

	flag.StringVar(&signalURL, "signal", "http://127.0.0.1:8090", "signaling server base URL")
	flag.StringVar(&sessionID, "session", "", "session id (must match receiver)")
	flag.StringVar(&inputWAV, "input", "", "input WAV path (PCM16 mono 48k)")
	flag.IntVar(&frameMS, "frame-ms", 20, "frame duration in milliseconds")
	flag.IntVar(&bitrate, "bitrate", 32000, "opus bitrate in bps")
	flag.IntVar(&packetLoss, "plp", 10, "expected packet loss percentage for opus encoder")
	flag.BoolVar(&enableFEC, "fec", true, "enable opus in-band FEC")
	flag.DurationVar(&connectWait, "connect-timeout", 10*time.Second, "peer connection timeout")
	flag.DurationVar(&signalWait, "signal-timeout", 20*time.Second, "answer wait timeout")
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

	audio, err := wav.ReadPCM16Mono(inputWAV)
	if err != nil {
		log.Fatalf("read input wav failed: %v", err)
	}
	if audio.SampleRate != 48000 {
		log.Fatalf("unsupported sample rate=%d, sender currently requires 48000Hz", audio.SampleRate)
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

	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, readErr := rtpSender.Read(buf); readErr != nil {
				return
			}
		}
	}()

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

	encoder, err := opus.NewEncoder(48000, 2, opus.AppVoIP)
	if err != nil {
		log.Fatalf("create opus encoder failed: %v", err)
	}
	if err := encoder.SetBitrate(bitrate); err != nil {
		log.Fatalf("set encoder bitrate failed: %v", err)
	}
	if err := encoder.SetInBandFEC(enableFEC); err != nil {
		log.Fatalf("set in-band fec failed: %v", err)
	}
	if err := encoder.SetPacketLossPerc(packetLoss); err != nil {
		log.Fatalf("set packet loss percentage failed: %v", err)
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

		n, encodeErr := encoder.Encode(stereoPCM, packetBuf)
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
