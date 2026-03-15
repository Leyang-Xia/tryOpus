package signal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) CreateSession(sessionID string) (string, error) {
	body, err := json.Marshal(SessionResponse{SessionID: sessionID})
	if err != nil {
		return "", fmt.Errorf("marshal create session request failed: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+"/api/session", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create session failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create session got status=%d body=%s", resp.StatusCode, string(payload))
	}

	var created SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", fmt.Errorf("decode create session response failed: %w", err)
	}
	if created.SessionID == "" {
		return "", fmt.Errorf("empty session id in response")
	}
	return created.SessionID, nil
}

func (c *Client) PublishOffer(sessionID string, sdp SDP) error {
	return c.publishSDP("/api/session/"+url.PathEscape(sessionID)+"/offer", sdp)
}

func (c *Client) PublishAnswer(sessionID string, sdp SDP) error {
	return c.publishSDP("/api/session/"+url.PathEscape(sessionID)+"/answer", sdp)
}

func (c *Client) WaitOffer(sessionID string, timeout time.Duration) (SDP, error) {
	return c.waitSDP("/api/session/"+url.PathEscape(sessionID)+"/offer", timeout)
}

func (c *Client) WaitAnswer(sessionID string, timeout time.Duration) (SDP, error) {
	return c.waitSDP("/api/session/"+url.PathEscape(sessionID)+"/answer", timeout)
}

func (c *Client) publishSDP(path string, sdp SDP) error {
	body, err := json.Marshal(sdp)
	if err != nil {
		return fmt.Errorf("marshal sdp failed: %w", err)
	}
	resp, err := c.httpClient.Post(c.baseURL+path, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post sdp failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("publish sdp got status=%d body=%s", resp.StatusCode, string(payload))
	}
	return nil
}

func (c *Client) waitSDP(path string, timeout time.Duration) (SDP, error) {
	deadline := time.Now().Add(timeout)
	for {
		sdp, found, err := c.tryGetSDP(path)
		if err != nil {
			return SDP{}, err
		}
		if found {
			return sdp, nil
		}
		if time.Now().After(deadline) {
			return SDP{}, fmt.Errorf("wait sdp timeout after %s", timeout)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (c *Client) tryGetSDP(path string) (SDP, bool, error) {
	resp, err := c.httpClient.Get(c.baseURL + path)
	if err != nil {
		return SDP{}, false, fmt.Errorf("get sdp failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return SDP{}, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return SDP{}, false, fmt.Errorf("get sdp got status=%d body=%s", resp.StatusCode, string(payload))
	}

	var sdp SDP
	if err := json.NewDecoder(resp.Body).Decode(&sdp); err != nil {
		return SDP{}, false, fmt.Errorf("decode sdp failed: %w", err)
	}
	if sdp.SDP == "" || sdp.Type == "" {
		return SDP{}, false, fmt.Errorf("invalid sdp payload")
	}
	return sdp, true, nil
}
