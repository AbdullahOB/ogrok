package client

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"ogrok/internal/shared"

	"github.com/fatih/color"
	"github.com/gorilla/websocket"
)

type Client struct {
	config     *shared.ClientConfig
	conn       *websocket.Conn
	proxy      *LocalProxy
	connected  bool
	tunnelURL  string
	tunnelID   string
	stats      *Stats
	workerPool chan struct{}
	useTLS     bool
	mu         sync.RWMutex
	writeMu    sync.Mutex
}

type Stats struct {
	RequestsHandled  uint64
	BytesTransferred uint64
	ConnectedAt      time.Time
	mu               sync.RWMutex
}

func NewClient(config *shared.ClientConfig) *Client {
	useTLS := config.Server != "localhost" && config.Server != "127.0.0.1"

	return &Client{
		config:     config,
		workerPool: make(chan struct{}, 50),
		useTLS:     useTLS,
		stats: &Stats{
			ConnectedAt: time.Now(),
		},
	}
}

func (c *Client) SetTLS(useTLS bool) {
	c.useTLS = useTLS
}

func (c *Client) writeJSON(v interface{}) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteJSON(v)
}

// Connect establishes and maintains the tunnel connection with auto-retry.
func (c *Client) Connect() error {
	for {
		err := c.connectOnce()
		if err == nil {
			return nil
		}

		if isPermanentError(err) {
			return err
		}

		if err := c.retryConnection(); err != nil {
			return err
		}
	}
}

func (c *Client) connectOnce() error {
	scheme := "ws"
	if c.useTLS {
		scheme = "wss"
	}

	wsURL := url.URL{
		Scheme: scheme,
		Host:   c.config.Server,
		Path:   "/_tunnel/connect",
	}

	color.Yellow("Connecting to %s...", wsURL.String())

	headers := make(map[string][]string)
	headers["Authorization"] = []string{"Bearer " + c.config.Token}

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL.String(), headers)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
		}
		return fmt.Errorf("failed to connect: %v", err)
	}

	c.conn = conn
	c.proxy = NewLocalProxy(c.config.LocalPort)

	regMsg := &shared.RegisterMessage{
		Token:        c.config.Token,
		Subdomain:    c.config.Subdomain,
		CustomDomain: c.config.CustomDomain,
		LocalPort:    c.config.LocalPort,
	}

	msg := &shared.Message{Type: shared.MsgTypeRegister}
	msg.Data, err = json.Marshal(regMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal registration: %v", err)
	}

	if err := c.writeJSON(msg); err != nil {
		return fmt.Errorf("failed to send registration: %v", err)
	}

	go c.handleMessages()
	go c.displayStatus()

	return c.waitForShutdown()
}

func (c *Client) handleMessages() {
	defer c.conn.Close()

	for {
		var msg shared.Message
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				color.Red("Connection lost: %v", err)
			}
			c.setConnected(false)
			break
		}

		switch msg.Type {
		case shared.MsgTypeRegisterOK:
			var okMsg shared.RegisterOKMessage
			if err := json.Unmarshal(msg.Data, &okMsg); err != nil {
				color.Red("Invalid registration response: %v", err)
				continue
			}

			c.mu.Lock()
			c.tunnelURL = okMsg.URL
			c.tunnelID = okMsg.TunnelID
			c.mu.Unlock()

			c.setConnected(true)

			fmt.Println()
			color.Green("Status:     connected")
			color.Cyan("Forwarding: %s -> http://localhost:%d", okMsg.URL, c.config.LocalPort)
			fmt.Println()
			fmt.Printf("Connections: 0\n")
			fmt.Println()

		case shared.MsgTypeRegisterError:
			var errMsg shared.RegisterErrorMessage
			if err := json.Unmarshal(msg.Data, &errMsg); err != nil {
				color.Red("Invalid error response: %v", err)
				continue
			}
			color.Red("Registration failed: %s", errMsg.Error)
			return

		case shared.MsgTypeHTTPRequest:
			var reqMsg shared.HTTPRequestMessage
			if err := json.Unmarshal(msg.Data, &reqMsg); err != nil {
				color.Red("Invalid HTTP request: %v", err)
				continue
			}
			go c.handleHTTPRequestWithPool(&reqMsg)

		case shared.MsgTypePing:
			pongMsg := &shared.PongMessage{Timestamp: time.Now().Unix()}
			respMsg := &shared.Message{Type: shared.MsgTypePong}
			respMsg.Data, _ = json.Marshal(pongMsg)
			c.writeJSON(respMsg)

		default:
			log.Printf("Unknown message type: %s", msg.Type)
		}
	}
}

func (c *Client) handleHTTPRequestWithPool(req *shared.HTTPRequestMessage) {
	select {
	case c.workerPool <- struct{}{}:
		defer func() { <-c.workerPool }()
		c.handleHTTPRequest(req)
	default:
		// pool exhausted, reject
		resp := &shared.HTTPResponseMessage{
			RequestID:  req.RequestID,
			StatusCode: 503,
			Headers:    map[string]string{"Content-Type": "text/plain"},
			BodyBase64: encodeBase64([]byte("Service Unavailable")),
		}
		msg := &shared.Message{Type: shared.MsgTypeHTTPResponse}
		msg.Data, _ = json.Marshal(resp)
		c.writeJSON(msg)
		color.Yellow("Worker pool full, rejected request: %s %s", req.Method, req.Path)
	}
}

func (c *Client) handleHTTPRequest(req *shared.HTTPRequestMessage) {
	resp, err := c.proxy.ForwardRequest(req)
	if err != nil {
		color.Red("Failed to forward request: %v", err)
		resp = &shared.HTTPResponseMessage{
			RequestID:  req.RequestID,
			StatusCode: 502,
			Headers:    map[string]string{"Content-Type": "text/plain"},
			BodyBase64: encodeBase64([]byte("Bad Gateway")),
		}
	}

	msg := &shared.Message{Type: shared.MsgTypeHTTPResponse}
	msg.Data, _ = json.Marshal(resp)

	if err := c.writeJSON(msg); err != nil {
		color.Red("Failed to send response: %v", err)
		return
	}

	c.stats.mu.Lock()
	c.stats.RequestsHandled++
	if resp.BodyBase64 != "" {
		c.stats.BytesTransferred += uint64(len(resp.BodyBase64))
	}
	c.stats.mu.Unlock()

	color.Blue("%s %s -> %d", req.Method, req.Path, resp.StatusCode)
}

func (c *Client) displayStatus() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.RLock()
		connected := c.connected
		c.mu.RUnlock()

		if !connected {
			continue
		}

		c.stats.mu.RLock()
		requests := c.stats.RequestsHandled
		c.stats.mu.RUnlock()

		fmt.Printf("\rConnections: %d", requests)
	}
}

func (c *Client) setConnected(connected bool) {
	c.mu.Lock()
	c.connected = connected
	c.mu.Unlock()
}

func (c *Client) waitForShutdown() error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	color.Yellow("\nShutting down...")

	if c.conn != nil {
		c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.conn.Close()
	}

	color.Green("Goodbye!")
	return nil
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func encodeBase64(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}

func (c *Client) retryConnection() error {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		color.Yellow("Reconnecting in %v...", backoff)
		time.Sleep(backoff)

		err := c.connectOnce()
		if err == nil {
			return nil
		}

		if isPermanentError(err) {
			return err
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func isPermanentError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "401") ||
		strings.Contains(errStr, "403") ||
		strings.Contains(errStr, "Invalid token") ||
		strings.Contains(errStr, "Registration failed")
}
