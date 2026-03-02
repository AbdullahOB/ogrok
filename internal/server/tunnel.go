package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"ogrok/internal/shared"

	"github.com/gorilla/websocket"
	"golang.org/x/time/rate"
)

type Tunnel struct {
	ID           string
	Subdomain    string
	CustomDomain string
	LocalPort    int
	Token        string
	Conn         *websocket.Conn
	RateLimiter  *rate.Limiter
	LastSeen     time.Time
	mu           sync.RWMutex
	writeMu      sync.Mutex
}

type TunnelManager struct {
	tunnels            map[string]*Tunnel
	subdomains         map[string]*Tunnel
	customDomains      map[string]*Tunnel
	tokenTunnels       map[string][]*Tunnel
	mu                 sync.RWMutex
	baseDomain         string
	maxTunnelsPerToken int
	maxTotalTunnels    int
}

func NewTunnelManager(baseDomain string) *TunnelManager {
	return &TunnelManager{
		tunnels:            make(map[string]*Tunnel),
		subdomains:         make(map[string]*Tunnel),
		customDomains:      make(map[string]*Tunnel),
		tokenTunnels:       make(map[string][]*Tunnel),
		baseDomain:         baseDomain,
		maxTunnelsPerToken: 10,
		maxTotalTunnels:    100,
	}
}

func (tm *TunnelManager) RegisterTunnel(conn *websocket.Conn, req *shared.RegisterMessage) (*Tunnel, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if len(tm.tunnels) >= tm.maxTotalTunnels {
		return nil, fmt.Errorf("maximum number of tunnels reached")
	}

	if len(tm.tokenTunnels[req.Token]) >= tm.maxTunnelsPerToken {
		return nil, fmt.Errorf("maximum number of tunnels per token reached")
	}

	tunnelID := generateTunnelID()

	// assign a random subdomain if the client didn't request one
	if req.Subdomain == "" && req.CustomDomain == "" {
		req.Subdomain = generateSubdomain()
		for {
			if _, exists := tm.subdomains[req.Subdomain]; !exists {
				break
			}
			req.Subdomain = generateSubdomain()
		}
	}

	if req.Subdomain != "" {
		if _, exists := tm.subdomains[req.Subdomain]; exists {
			return nil, fmt.Errorf("subdomain already in use")
		}
	}

	if req.CustomDomain != "" {
		if _, exists := tm.customDomains[req.CustomDomain]; exists {
			return nil, fmt.Errorf("custom domain already in use")
		}
	}

	tunnel := &Tunnel{
		ID:           tunnelID,
		Subdomain:    req.Subdomain,
		CustomDomain: req.CustomDomain,
		LocalPort:    req.LocalPort,
		Token:        req.Token,
		Conn:         conn,
		RateLimiter:  rate.NewLimiter(rate.Limit(100), 10),
		LastSeen:     time.Now(),
	}

	tm.tunnels[tunnelID] = tunnel
	if req.Subdomain != "" {
		tm.subdomains[req.Subdomain] = tunnel
	}
	if req.CustomDomain != "" {
		tm.customDomains[req.CustomDomain] = tunnel
	}
	tm.tokenTunnels[req.Token] = append(tm.tokenTunnels[req.Token], tunnel)

	log.Printf("Registered tunnel %s: subdomain=%s custom_domain=%s port=%d",
		tunnelID, req.Subdomain, req.CustomDomain, req.LocalPort)

	return tunnel, nil
}

func (tm *TunnelManager) UnregisterTunnel(tunnelID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tunnel, ok := tm.tunnels[tunnelID]
	if !ok {
		return
	}

	delete(tm.tunnels, tunnelID)
	if tunnel.Subdomain != "" {
		delete(tm.subdomains, tunnel.Subdomain)
	}
	if tunnel.CustomDomain != "" {
		delete(tm.customDomains, tunnel.CustomDomain)
	}

	toks := tm.tokenTunnels[tunnel.Token]
	for i, t := range toks {
		if t.ID == tunnelID {
			tm.tokenTunnels[tunnel.Token] = append(toks[:i], toks[i+1:]...)
			break
		}
	}
	if len(tm.tokenTunnels[tunnel.Token]) == 0 {
		delete(tm.tokenTunnels, tunnel.Token)
	}

	log.Printf("Unregistered tunnel %s", tunnelID)
}

func (tm *TunnelManager) GetTunnelByHost(host string) *Tunnel {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if t, ok := tm.customDomains[host]; ok {
		return t
	}

	if sub := tm.extractSubdomain(host); sub != "" {
		if t, ok := tm.subdomains[sub]; ok {
			return t
		}
	}

	return nil
}

func (tm *TunnelManager) IsCustomDomain(host string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	_, ok := tm.customDomains[host]
	return ok
}

func (tm *TunnelManager) extractSubdomain(host string) string {
	if tm.baseDomain == "" {
		return ""
	}

	suffix := "." + tm.baseDomain
	if len(host) <= len(suffix) || !endsWith(host, suffix) {
		return ""
	}

	return host[:len(host)-len(suffix)]
}

func (tm *TunnelManager) GetTunnelURL(tunnel *Tunnel) string {
	if tunnel.CustomDomain != "" {
		return "https://" + tunnel.CustomDomain
	}
	if tunnel.Subdomain != "" {
		return fmt.Sprintf("https://%s.%s", tunnel.Subdomain, tm.baseDomain)
	}
	return ""
}

func (tm *TunnelManager) SendHTTPRequest(tunnel *Tunnel, requestID string, r *http.Request) error {
	tunnel.mu.Lock()
	defer tunnel.mu.Unlock()

	if !tunnel.RateLimiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}

	headers := make(map[string]string)
	for k, v := range r.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	body, err := readLimitedBody(r.Body, 10*1024*1024)
	if err != nil {
		return fmt.Errorf("bad request - body read error: %v", err)
	}

	reqMsg := &shared.HTTPRequestMessage{
		RequestID:  requestID,
		Method:     r.Method,
		Path:       r.URL.RequestURI(),
		Headers:    headers,
		BodyBase64: encodeBase64(body),
	}

	msg := &shared.Message{Type: shared.MsgTypeHTTPRequest}
	msg.Data, err = json.Marshal(reqMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}

	tunnel.LastSeen = time.Now()
	return tunnel.WriteJSON(msg)
}

func (t *Tunnel) WriteJSON(v interface{}) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return t.Conn.WriteJSON(v)
}

func (t *Tunnel) UpdateLastSeen() {
	t.mu.Lock()
	t.LastSeen = time.Now()
	t.mu.Unlock()
}

func generateTunnelID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("tunnel_%d_%d", time.Now().UnixNano(), os.Getpid())
	}
	return "tunnel_" + hex.EncodeToString(b)
}

func generateSubdomain() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
