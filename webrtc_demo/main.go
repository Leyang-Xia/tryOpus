package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/pion/webrtc/v4"
)

type signalRequest struct {
	SDP  string `json:"sdp"`
	Type string `json:"type"`
}

type signalResponse struct {
	SDP  string `json:"sdp"`
	Type string `json:"type"`
}

func newPeerConnection() (*webrtc.PeerConnection, error) {
	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, fmt.Errorf("register codecs failed: %w", err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, fmt.Errorf("create peer connection failed: %w", err)
	}
	return pc, nil
}

func handleOffer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req signalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid json: %v", err), http.StatusBadRequest)
		return
	}

	offer := webrtc.SessionDescription{
		Type: webrtc.NewSDPType(req.Type),
		SDP:  req.SDP,
	}
	if offer.Type != webrtc.SDPTypeOffer || offer.SDP == "" {
		http.Error(w, "request must include a valid offer", http.StatusBadRequest)
		return
	}

	pc, err := newPeerConnection()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("peer state: %s", state.String())
		switch state {
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateDisconnected, webrtc.PeerConnectionStateClosed:
			if closeErr := pc.Close(); closeErr != nil {
				log.Printf("peer close failed: %v", closeErr)
			}
		}
	})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		log.Printf("datachannel created: label=%s id=%d", dc.Label(), dc.ID())

		dc.OnOpen(func() {
			log.Printf("datachannel open: label=%s", dc.Label())
			if err := dc.SendText("hello from pion server"); err != nil {
				log.Printf("initial send failed: %v", err)
			}
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			in := string(msg.Data)
			reply := fmt.Sprintf("echo[%s]: %s", time.Now().Format("15:04:05"), in)
			if err := dc.SendText(reply); err != nil {
				log.Printf("echo send failed: %v", err)
			}
		})
	})

	if err := pc.SetRemoteDescription(offer); err != nil {
		http.Error(w, fmt.Sprintf("set remote description failed: %v", err), http.StatusBadRequest)
		return
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("create answer failed: %v", err), http.StatusInternalServerError)
		return
	}

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		http.Error(w, fmt.Sprintf("set local description failed: %v", err), http.StatusInternalServerError)
		return
	}
	<-gatherComplete

	resp := signalResponse{
		SDP:  pc.LocalDescription().SDP,
		Type: pc.LocalDescription().Type.String(),
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("encode response failed: %v", err)
	}
}

func newServerMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(indexHTML))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/offer", handleOffer)
	return mux
}

func main() {
	addr := ":8080"
	log.Printf("starting pion webrtc demo on %s", addr)
	log.Printf("open http://127.0.0.1%s in your browser", addr)
	if err := http.ListenAndServe(addr, newServerMux()); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
