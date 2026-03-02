package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"ogrok/internal/client"
	"ogrok/internal/shared"

	"github.com/fatih/color"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server string `yaml:"server"`
	Token  string `yaml:"token"`
}

var Version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("ogrok version %s\n", Version)
		return
	}

	if len(os.Args) < 3 {
		printUsage()
		os.Exit(1)
	}

	protocol := os.Args[1]
	if protocol != "http" {
		color.Red("Error: only 'http' protocol is supported currently")
		os.Exit(1)
	}

	portStr := os.Args[2]
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		color.Red("Error: invalid port number: %s", portStr)
		os.Exit(1)
	}

	var subdomain, domain, server, token string
	args := os.Args[3:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--subdomain":
			if i+1 >= len(args) {
				color.Red("Error: --subdomain requires a value")
				os.Exit(1)
			}
			subdomain = args[i+1]
			i++
		case "--domain":
			if i+1 >= len(args) {
				color.Red("Error: --domain requires a value")
				os.Exit(1)
			}
			domain = args[i+1]
			i++
		case "--server":
			if i+1 >= len(args) {
				color.Red("Error: --server requires a value")
				os.Exit(1)
			}
			server = args[i+1]
			i++
		case "--token":
			if i+1 >= len(args) {
				color.Red("Error: --token requires a value")
				os.Exit(1)
			}
			token = args[i+1]
			i++
		default:
			color.Red("Error: unknown flag: %s", args[i])
			printUsage()
			os.Exit(1)
		}
	}

	if subdomain != "" && domain != "" {
		color.Red("Error: cannot specify both --subdomain and --domain")
		os.Exit(1)
	}

	// priority: flag > config file > default
	if server == "" {
		server = getConfigValue("server", "ogrok.dev")
	}

	// priority: flag > env > config file
	if token == "" {
		token = os.Getenv("OGROK_TOKEN")
		if token == "" {
			token = getConfigValue("token", "")
		}
	}

	if token == "" {
		printTokenHelp()
		os.Exit(1)
	}

	config := &shared.ClientConfig{
		Server:       server,
		Token:        token,
		LocalPort:    port,
		Subdomain:    subdomain,
		CustomDomain: domain,
		TLS:          !isLocalhost(server),
	}

	color.Green("ogrok - secure tunnels to localhost")
	fmt.Println()

	c := client.NewClient(config)
	c.SetTLS(config.TLS)
	if err := c.Connect(); err != nil {
		color.Red("Connection error: %v", err)
		os.Exit(1)
	}
}

func printUsage() {
	color.Green("ogrok - secure tunnels to localhost")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  ogrok http <port>                          # expose local port via random subdomain")
	fmt.Println("  ogrok http <port> --subdomain myapp        # expose as myapp.ogrok.dev")
	fmt.Println("  ogrok http <port> --domain dev.mysite.com  # expose via custom domain")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --subdomain <name>    Use specific subdomain")
	fmt.Println("  --domain <domain>     Use custom domain")
	fmt.Println("  --server <address>    Tunnel server address (default: ogrok.dev)")
	fmt.Println("  --token <token>       Authentication token")
	fmt.Println()
	fmt.Println("Token can be set via:")
	fmt.Println("  1. --token flag")
	fmt.Println("  2. OGROK_TOKEN environment variable")
	fmt.Println("  3. ~/.ogrok/config.yaml file")
}

func printTokenHelp() {
	color.Red("Error: No authentication token found")
	fmt.Println()
	fmt.Println("Set your token using one of these methods:")
	fmt.Println()
	color.Cyan("1. Environment variable:")
	fmt.Println("   export OGROK_TOKEN=your-token-here")
	fmt.Println()
	color.Cyan("2. Config file (~/.ogrok/config.yaml):")
	fmt.Println("   mkdir -p ~/.ogrok")
	fmt.Println("   echo 'token: your-token-here' > ~/.ogrok/config.yaml")
	fmt.Println()
	color.Cyan("3. Command line flag:")
	fmt.Println("   ogrok http 3000 --token your-token-here")
}

func getConfigValue(key, defaultValue string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return defaultValue
	}

	data, err := os.ReadFile(filepath.Join(homeDir, ".ogrok", "config.yaml"))
	if err != nil {
		return defaultValue
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return defaultValue
	}

	switch key {
	case "server":
		if config.Server != "" {
			return config.Server
		}
	case "token":
		if config.Token != "" {
			return config.Token
		}
	}

	return defaultValue
}

func isLocalhost(server string) bool {
	return strings.HasPrefix(server, "localhost") ||
		strings.HasPrefix(server, "127.0.0.1") ||
		strings.HasPrefix(server, "::1")
}
