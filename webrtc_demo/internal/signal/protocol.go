package signal

type SDP struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

type SessionResponse struct {
	SessionID string `json:"session_id"`
}
