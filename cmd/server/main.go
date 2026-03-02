package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"ogrok/internal/server"
	"ogrok/internal/shared"

	"gopkg.in/yaml.v3"
)

var Version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		handleInit()
		return
	}

	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("ogrok-server version %s\n", Version)
		return
	}

	var configPath string
	flag.StringVar(&configPath, "config", "configs/server.yaml", "Path to server configuration file")
	flag.Parse()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("Configuration file not found: %s", configPath)
	}

	srv, err := server.NewServer(configPath)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	log.Println("Starting ogrok server...")
	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func handleInit() {
	var domain string

	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		if args[i] == "--domain" && i+1 < len(args) {
			domain = args[i+1]
			i++
		}
	}

	if domain == "" {
		fmt.Print("Enter your base domain (e.g., ogrok.dev): ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			domain = strings.TrimSpace(scanner.Text())
		}
		if domain == "" {
			log.Fatal("Domain is required")
		}
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		log.Fatalf("Failed to generate token: %v", err)
	}
	token := hex.EncodeToString(tokenBytes)

	config := &shared.ServerConfig{
		Server: shared.ServerSettings{
			BaseDomain:         domain,
			HTTPPort:           80,
			HTTPSPort:          443,
			AdminPort:          8080,
			MaxTunnelsPerToken: 10,
			MaxTotalTunnels:    100,
		},
		TLS: shared.TLSSettings{
			AutoCert:     true,
			CertCacheDir: "/var/lib/ogrok/certs",
		},
		Auth: shared.AuthSettings{
			Tokens: []string{token},
		},
	}

	configDir := "configs"
	if err := os.MkdirAll(configDir, 0755); err != nil {
		log.Fatalf("Failed to create config directory: %v", err)
	}

	configPath := filepath.Join(configDir, "server.yaml")
	configData, err := yaml.Marshal(config)
	if err != nil {
		log.Fatalf("Failed to marshal config: %v", err)
	}

	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		log.Fatalf("Failed to write config file: %v", err)
	}

	fmt.Printf("Configuration created: %s\n", configPath)
	fmt.Printf("Auth token: %s\n", token)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("1. Start the server: ogrok-server --config %s\n", configPath)
	fmt.Printf("2. Give clients this token: %s\n", token)
	fmt.Println()
	fmt.Println("Note: Make sure DNS for *.%s points to this server", domain)
}
