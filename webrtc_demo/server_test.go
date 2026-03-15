package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func TestOfferHandshakeAndEcho(t *testing.T) {
	server := httptest.NewServer(newServerMux())
	defer server.Close()

	clientPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("create client peer failed: %v", err)
	}
	defer func() {
		_ = clientPC.Close()
	}()

	openCh := make(chan struct{}, 1)
	msgCh := make(chan string, 2)

	dc, err := clientPC.CreateDataChannel("demo", nil)
	if err != nil {
		t.Fatalf("create datachannel failed: %v", err)
	}
	dc.OnOpen(func() {
		openCh <- struct{}{}
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		msgCh <- string(msg.Data)
	})

	offer, err := clientPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("create offer failed: %v", err)
	}
	gatherDone := webrtc.GatheringCompletePromise(clientPC)
	if err := clientPC.SetLocalDescription(offer); err != nil {
		t.Fatalf("set local description failed: %v", err)
	}
	<-gatherDone

	reqBody, err := json.Marshal(signalRequest{
		SDP:  clientPC.LocalDescription().SDP,
		Type: clientPC.LocalDescription().Type.String(),
	})
	if err != nil {
		t.Fatalf("marshal request failed: %v", err)
	}

	resp, err := http.Post(server.URL+"/offer", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("post offer failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: %d body=%s", resp.StatusCode, string(body))
	}

	var answer signalResponse
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		t.Fatalf("decode answer failed: %v", err)
	}

	if err := clientPC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.NewSDPType(answer.Type),
		SDP:  answer.SDP,
	}); err != nil {
		t.Fatalf("set remote description failed: %v", err)
	}

	select {
	case <-openCh:
	case <-time.After(8 * time.Second):
		t.Fatal("datachannel open timeout")
	}

	if err := dc.SendText("hello from test"); err != nil {
		t.Fatalf("send text failed: %v", err)
	}

	var got []string
	timeout := time.After(8 * time.Second)
	for len(got) < 2 {
		select {
		case msg := <-msgCh:
			got = append(got, msg)
		case <-timeout:
			t.Fatalf("receive message timeout, got=%v", got)
		}
	}

	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "hello from pion server") {
		t.Fatalf("missing server welcome message, got=%v", got)
	}
	if !strings.Contains(joined, "hello from test") {
		t.Fatalf("missing echoed client message, got=%v", got)
	}
}
