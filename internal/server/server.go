package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ogrok/internal/shared"

	"golang.org/x/crypto/acme/autocert"
	"gopkg.in/yaml.v3"
)

type Server struct {
	config        *shared.ServerConfig
	tunnelManager *TunnelManager
	proxyHandler  *ProxyHandler
	wsHandler     *WebSocketHandler
	auth          *AuthMiddleware
	httpServer    *http.Server
	httpsServer   *http.Server
	adminServer   *http.Server
	certManager   *autocert.Manager
	done          chan struct{}
}

func NewServer(configPath string) (*Server, error) {
	config, err := loadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %v", err)
	}

	tunnelManager := NewTunnelManager(config.Server.BaseDomain)
	if config.Server.MaxTunnelsPerToken > 0 {
		tunnelManager.maxTunnelsPerToken = config.Server.MaxTunnelsPerToken
	}
	if config.Server.MaxTotalTunnels > 0 {
		tunnelManager.maxTotalTunnels = config.Server.MaxTotalTunnels
	}

	auth := NewAuthMiddleware(config.Auth.Tokens)

	s := &Server{
		config:        config,
		tunnelManager: tunnelManager,
		auth:          auth,
		done:          make(chan struct{}),
	}

	s.proxyHandler = NewProxyHandler(tunnelManager, s.done)
	s.wsHandler = NewWebSocketHandler(tunnelManager, s.proxyHandler, auth)

	return s, nil
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.Handle("/_tunnel/connect", s.wsHandler)
	mux.Handle("/", s.proxyHandler)

	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/health", s.healthHandler)
	adminMux.HandleFunc("/stats", s.statsHandler)

	if s.config.TLS.AutoCert {
		s.certManager = &autocert.Manager{
			Prompt: autocert.AcceptTOS,
			HostPolicy: func(ctx context.Context, host string) error {
				if host == s.config.Server.BaseDomain {
					return nil
				}
				if strings.HasSuffix(host, "."+s.config.Server.BaseDomain) {
					return nil
				}
				return fmt.Errorf("acme/autocert: host %q not allowed", host)
			},
			Cache: autocert.DirCache(s.config.TLS.CertCacheDir),
		}

		mux.Handle("/.well-known/acme-challenge/", s.certManager.HTTPHandler(nil))
	}

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Server.HTTPPort),
		Handler: mux,
	}

	s.adminServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Server.AdminPort),
		Handler: adminMux,
	}

	go func() {
		log.Printf("HTTP server on :%d", s.config.Server.HTTPPort)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	go func() {
		log.Printf("Admin server on :%d", s.config.Server.AdminPort)
		if err := s.adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Admin server error: %v", err)
		}
	}()

	if s.config.TLS.AutoCert || (s.config.TLS.CertFile != "" && s.config.TLS.KeyFile != "") {
		s.httpsServer = &http.Server{
			Addr:    fmt.Sprintf(":%d", s.config.Server.HTTPSPort),
			Handler: mux,
		}

		go func() {
			log.Printf("HTTPS server on :%d", s.config.Server.HTTPSPort)
			if err := s.startHTTPS(); err != nil && err != http.ErrServerClosed {
				log.Printf("HTTPS server error: %v", err)
			}
		}()
	}

	go s.heartbeatRoutine()

	return s.waitForShutdown()
}

func (s *Server) startHTTPS() error {
	if s.config.TLS.AutoCert {
		s.httpsServer.TLSConfig = &tls.Config{
			GetCertificate: s.certManager.GetCertificate,
		}
		return s.httpsServer.ListenAndServeTLS("", "")
	}
	return s.httpsServer.ListenAndServeTLS(s.config.TLS.CertFile, s.config.TLS.KeyFile)
}

func (s *Server) heartbeatRoutine() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.tunnelManager.mu.RLock()
			tunnels := make([]*Tunnel, 0, len(s.tunnelManager.tunnels))
			for _, t := range s.tunnelManager.tunnels {
				tunnels = append(tunnels, t)
			}
			s.tunnelManager.mu.RUnlock()

			for _, t := range tunnels {
				t.mu.RLock()
				lastSeen := t.LastSeen
				t.mu.RUnlock()

				if time.Since(lastSeen) > 2*time.Minute {
					log.Printf("Removing stale tunnel: %s", t.ID)
					s.tunnelManager.UnregisterTunnel(t.ID)
					t.Conn.Close()
					continue
				}

				pingMsg := &shared.PingMessage{Timestamp: time.Now().Unix()}
				msg := &shared.Message{Type: shared.MsgTypePing}
				msg.Data, _ = json.Marshal(pingMsg)

				if err := t.WriteJSON(msg); err != nil {
					log.Printf("Failed to ping tunnel %s: %v", t.ID, err)
					s.tunnelManager.UnregisterTunnel(t.ID)
					t.Conn.Close()
				}
			}
		case <-s.done:
			return
		}
	}
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) statsHandler(w http.ResponseWriter, r *http.Request) {
	s.tunnelManager.mu.RLock()
	n := len(s.tunnelManager.tunnels)
	s.tunnelManager.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"active_tunnels": %d}`, n)
}

func (s *Server) waitForShutdown() error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutting down...")

	close(s.done)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if s.httpServer != nil {
		s.httpServer.Shutdown(ctx)
	}
	if s.httpsServer != nil {
		s.httpsServer.Shutdown(ctx)
	}
	if s.adminServer != nil {
		s.adminServer.Shutdown(ctx)
	}

	log.Println("Server shutdown complete")
	return nil
}

func loadConfig(path string) (*shared.ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config shared.ServerConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if config.Server.HTTPPort == 0 {
		config.Server.HTTPPort = 80
	}
	if config.Server.HTTPSPort == 0 {
		config.Server.HTTPSPort = 443
	}
	if config.Server.AdminPort == 0 {
		config.Server.AdminPort = 8080
	}
	if config.Server.MaxTunnelsPerToken == 0 {
		config.Server.MaxTunnelsPerToken = 10
	}
	if config.Server.MaxTotalTunnels == 0 {
		config.Server.MaxTotalTunnels = 100
	}
	if config.TLS.CertCacheDir == "" {
		config.TLS.CertCacheDir = "/var/lib/ogrok/certs"
	}

	return &config, nil
}
