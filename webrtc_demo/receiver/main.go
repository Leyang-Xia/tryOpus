package main

import (
	"flag"
	"log"
	"sync"
	"time"

	"github.com/hraban/opus"
	"github.com/pion/webrtc/v4"

	"opus_lab/webrtc_demo/internal/rtc"
	"opus_lab/webrtc_demo/internal/signal"
	"opus_lab/webrtc_demo/internal/wav"
)

type decodeStats struct {
	Packets      int
	Bytes        int
	DecodedFrame int
	DecodeErrors int
}

func main() {
	var (
		signalURL  string
		sessionID  string
		outputWAV  string
		duration   time.Duration
		signalWait time.Duration
	)

	flag.StringVar(&signalURL, "signal", "http://127.0.0.1:8090", "signaling server base URL")
	flag.StringVar(&sessionID, "session", "", "session id (must match sender)")
	flag.StringVar(&outputWAV, "output", "received.wav", "output WAV path")
	flag.DurationVar(&duration, "duration", 10*time.Second, "receive duration after connected")
	flag.DurationVar(&signalWait, "signal-timeout", 20*time.Second, "offer wait timeout")
	flag.Parse()

	if sessionID == "" {
		log.Fatal("--session is required")
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
		decoder, decErr := opus.NewDecoder(48000, 2)
		if decErr != nil {
			log.Printf("create opus decoder failed: %v", decErr)
			select {
			case doneCh <- struct{}{}:
			default:
			}
			return
		}

		pcmBuf := make([]int16, 5760*2)
		for {
			pkt, _, readErr := track.ReadRTP()
			if readErr != nil {
				break
			}

			statsMu.Lock()
			stats.Packets++
			stats.Bytes += len(pkt.Payload)
			statsMu.Unlock()

			n, err := decoder.Decode(pkt.Payload, pcmBuf)
			if err != nil {
				statsMu.Lock()
				stats.DecodeErrors++
				statsMu.Unlock()
				continue
			}

			statsMu.Lock()
			stats.DecodedFrame++
			statsMu.Unlock()

			decodedSamples := n * 2
			if decodedSamples > len(pcmBuf) {
				decodedSamples = n
			}
			frame := make([]int16, decodedSamples/2)
			for i := 0; i+1 < decodedSamples; i += 2 {
				left := int(pcmBuf[i])
				right := int(pcmBuf[i+1])
				frame[i/2] = int16((left + right) / 2)
			}
			samplesMu.Lock()
			samples = append(samples, frame...)
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
	finalStats := stats
	statsMu.Unlock()
	log.Printf(
		"receiver done: packets=%d bytes=%d decoded_frames=%d decode_errors=%d output_samples=%d output=%s",
		finalStats.Packets,
		finalStats.Bytes,
		finalStats.DecodedFrame,
		finalStats.DecodeErrors,
		len(outSamples),
		outputWAV,
	)
}
