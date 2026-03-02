package shared

import "encoding/json"

const (
	MsgTypeRegister      = "register"
	MsgTypeRegisterOK    = "register_ok"
	MsgTypeRegisterError = "register_error"
	MsgTypeHTTPRequest   = "http_request"
	MsgTypeHTTPResponse  = "http_response"
	MsgTypePing          = "ping"
	MsgTypePong          = "pong"
)

type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type RegisterMessage struct {
	Token        string `json:"token"`
	Subdomain    string `json:"subdomain,omitempty"`
	CustomDomain string `json:"custom_domain,omitempty"`
	LocalPort    int    `json:"local_port"`
}

type RegisterOKMessage struct {
	URL      string `json:"url"`
	TunnelID string `json:"tunnel_id"`
}

type RegisterErrorMessage struct {
	Error string `json:"error"`
}

type HTTPRequestMessage struct {
	RequestID  string            `json:"request_id"`
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Headers    map[string]string `json:"headers"`
	BodyBase64 string            `json:"body_base64,omitempty"`
}

type HTTPResponseMessage struct {
	RequestID  string            `json:"request_id"`
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	BodyBase64 string            `json:"body_base64,omitempty"`
}

type PingMessage struct {
	Timestamp int64 `json:"timestamp"`
}

type PongMessage struct {
	Timestamp int64 `json:"timestamp"`
}
