package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"ogrok/internal/shared"

	"github.com/gorilla/websocket"
)

type PendingRequest struct {
	Channel   chan *shared.HTTPResponseMessage
	CreatedAt time.Time
	TunnelID  string
	closeOnce sync.Once
}

func (pr *PendingRequest) Close() {
	pr.closeOnce.Do(func() {
		close(pr.Channel)
	})
}

type ProxyHandler struct {
	tunnelManager *TunnelManager
	pendingReqs   map[string]*PendingRequest
	mu            sync.RWMutex
	done          chan struct{}
}

func NewProxyHandler(tm *TunnelManager, done chan struct{}) *ProxyHandler {
	ph := &ProxyHandler{
		tunnelManager: tm,
		pendingReqs:   make(map[string]*PendingRequest),
		done:          done,
	}
	go ph.cleanupPendingRequests()
	return ph
}

func (p *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := p.sanitizeHost(r.Host)
	if !p.isValidHost(host) {
		log.Printf("Invalid host header: %s", r.Host)
		http.Error(w, "Tunnel not found", http.StatusNotFound)
		return
	}

	tunnel := p.tunnelManager.GetTunnelByHost(host)
	if tunnel == nil {
		http.Error(w, "Tunnel not found", http.StatusNotFound)
		return
	}

	requestID := generateRequestID()

	respChan := make(chan *shared.HTTPResponseMessage, 1)
	pending := &PendingRequest{
		Channel:   respChan,
		CreatedAt: time.Now(),
		TunnelID:  tunnel.ID,
	}

	p.mu.Lock()
	p.pendingReqs[requestID] = pending
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		delete(p.pendingReqs, requestID)
		p.mu.Unlock()
		pending.Close()
	}()

	if err := p.tunnelManager.SendHTTPRequest(tunnel, requestID, r); err != nil {
		log.Printf("Failed to send request to tunnel: %v", err)
		http.Error(w, "Internal server error", http.StatusBadGateway)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	select {
	case resp := <-respChan:
		p.writeResponse(w, resp)
	case <-ctx.Done():
		http.Error(w, "Request timeout", http.StatusGatewayTimeout)
	}
}

func (p *ProxyHandler) HandleResponse(resp *shared.HTTPResponseMessage) {
	p.mu.RLock()
	pending, ok := p.pendingReqs[resp.RequestID]
	p.mu.RUnlock()

	if !ok {
		log.Printf("Received response for unknown request: %s", resp.RequestID)
		return
	}

	select {
	case pending.Channel <- resp:
	default:
		log.Printf("Response channel full for request: %s", resp.RequestID)
	}
}

func (p *ProxyHandler) writeResponse(w http.ResponseWriter, resp *shared.HTTPResponseMessage) {
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}

	w.WriteHeader(resp.StatusCode)

	if resp.BodyBase64 != "" {
		body, err := base64.StdEncoding.DecodeString(resp.BodyBase64)
		if err != nil {
			log.Printf("Failed to decode response body: %v", err)
			return
		}
		w.Write(body)
	}
}

// WebSocketHandler manages tunnel connections from clients.
type WebSocketHandler struct {
	tunnelManager *TunnelManager
	proxyHandler  *ProxyHandler
	auth          *AuthMiddleware
	upgrader      websocket.Upgrader
}

func NewWebSocketHandler(tm *TunnelManager, ph *ProxyHandler, auth *AuthMiddleware) *WebSocketHandler {
	ws := &WebSocketHandler{
		tunnelManager: tm,
		proxyHandler:  ph,
		auth:          auth,
	}

	ws.upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				return false
			}
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				return false
			}
			return ws.auth.ValidateToken(parts[1])
		},
	}

	return ws
}

func (ws *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header required", http.StatusUnauthorized)
		return
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
		return
	}

	if !ws.auth.ValidateToken(parts[1]) {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	conn, err := ws.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	ws.handleConnection(conn)
}

func (ws *WebSocketHandler) handleConnection(conn *websocket.Conn) {
	var tunnel *Tunnel
	var tunnelID string
	var writeMu sync.Mutex

	safeWrite := func(v interface{}) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(v)
	}

	defer func() {
		if tunnelID != "" {
			ws.tunnelManager.UnregisterTunnel(tunnelID)
			ws.proxyHandler.CleanupTunnelRequests(tunnelID)
		}
	}()

	for {
		var msg shared.Message
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		switch msg.Type {
		case shared.MsgTypeRegister:
			var regMsg shared.RegisterMessage
			if err := json.Unmarshal(msg.Data, &regMsg); err != nil {
				ws.sendErrorSafe(safeWrite, "Invalid registration message")
				continue
			}

			if !ws.auth.ValidateToken(regMsg.Token) {
				ws.sendErrorSafe(safeWrite, "Invalid token")
				continue
			}

			var err error
			tunnel, err = ws.tunnelManager.RegisterTunnel(conn, &regMsg)
			if err != nil {
				ws.sendErrorSafe(safeWrite, err.Error())
				continue
			}

			tunnelID = tunnel.ID

			okMsg := &shared.RegisterOKMessage{
				URL:      ws.tunnelManager.GetTunnelURL(tunnel),
				TunnelID: tunnel.ID,
			}
			ws.sendMessageSafe(safeWrite, shared.MsgTypeRegisterOK, okMsg)

		case shared.MsgTypeHTTPResponse:
			var respMsg shared.HTTPResponseMessage
			if err := json.Unmarshal(msg.Data, &respMsg); err != nil {
				log.Printf("Invalid HTTP response message: %v", err)
				continue
			}
			ws.proxyHandler.HandleResponse(&respMsg)

		case shared.MsgTypePong:
			if tunnel != nil {
				tunnel.UpdateLastSeen()
			}

		default:
			log.Printf("Unknown message type: %s", msg.Type)
		}
	}
}

func (ws *WebSocketHandler) sendMessage(conn *websocket.Conn, msgType string, data interface{}) {
	msg := &shared.Message{Type: msgType}
	var err error
	msg.Data, err = json.Marshal(data)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		return
	}
	if err := conn.WriteJSON(msg); err != nil {
		log.Printf("Failed to send message: %v", err)
	}
}

func (ws *WebSocketHandler) sendError(conn *websocket.Conn, errMsg string) {
	ws.sendMessage(conn, shared.MsgTypeRegisterError, &shared.RegisterErrorMessage{Error: errMsg})
}

func (ws *WebSocketHandler) sendMessageSafe(safeWrite func(interface{}) error, msgType string, data interface{}) {
	msg := &shared.Message{Type: msgType}
	var err error
	msg.Data, err = json.Marshal(data)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		return
	}
	if err := safeWrite(msg); err != nil {
		log.Printf("Failed to send message: %v", err)
	}
}

func (ws *WebSocketHandler) sendErrorSafe(safeWrite func(interface{}) error, errMsg string) {
	ws.sendMessageSafe(safeWrite, shared.MsgTypeRegisterError, &shared.RegisterErrorMessage{Error: errMsg})
}

func (p *ProxyHandler) cleanupPendingRequests() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.mu.Lock()
			now := time.Now()
			for id, req := range p.pendingReqs {
				if now.Sub(req.CreatedAt) > 60*time.Second {
					req.Close()
					delete(p.pendingReqs, id)
				}
			}
			p.mu.Unlock()
		case <-p.done:
			return
		}
	}
}

func (p *ProxyHandler) CleanupTunnelRequests(tunnelID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, req := range p.pendingReqs {
		if req.TunnelID == tunnelID {
			req.Close()
			delete(p.pendingReqs, id)
		}
	}
}

func (p *ProxyHandler) sanitizeHost(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(host)
}

func (p *ProxyHandler) isValidHost(host string) bool {
	if net.ParseIP(host) != nil {
		return false
	}

	if host == "localhost" || strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".test") {
		return false
	}

	baseDomain := p.tunnelManager.baseDomain
	if baseDomain != "" {
		if host == baseDomain || strings.HasSuffix(host, "."+baseDomain) {
			return true
		}
	}

	return p.tunnelManager.IsCustomDomain(host)
}

func generateRequestID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "req_" + hex.EncodeToString(b)
}

func readLimitedBody(body io.ReadCloser, limit int64) ([]byte, error) {
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, limit))
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %v", err)
	}
	return data, nil
}

func encodeBase64(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}
