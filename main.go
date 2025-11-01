package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

// Config structures

type General struct {
	Listen           string `toml:"listenPort"`
	MainHostKeyword  string `toml:"mainHostKeyword"`
	Destination      string `toml:"destination"`
	SkipCheckTimeout int    `toml:"skipCheckTimeout"` // seconds
}

type Target struct {
	Destination  string   `toml:"destination"`
	MacAddress   string   `toml:"macAddress"`
	BroadcastIP  string   `toml:"broadcastIP"`
	WolPort      int      `toml:"wolPort"`
	WOL          bool     `toml:"wolEnable"`
	IgnoredHosts []string `toml:"ignoredHosts"`
	IgnoredPaths []string `toml:"ignoredPaths"`
}

type Config struct {
	General  General           `toml:"proxy"`
	Backends map[string]Target `toml:"backends"`
}

// Backend state caching

type backendState struct {
	lastOnline time.Time
	mu         sync.Mutex
}

var backendStates = map[string]*backendState{}

// Load config

func LoadConfig(filename string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(filename, &cfg); err != nil {
		return nil, err
	}

	for name := range cfg.Backends {
		backendStates[name] = &backendState{}
	}

	return &cfg, nil
}

// Utils

func checkHealth(url string) bool {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func sendWOL(macAddr, broadcastIP string, port int) error {
	mac, err := net.ParseMAC(macAddr)
	if err != nil {
		return fmt.Errorf("invalid MAC: %w", err)
	}
	packet := make([]byte, 102)
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}
	for i := 6; i < 102; i += 6 {
		copy(packet[i:], mac)
	}
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", broadcastIP, port))
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write(packet)
	return err
}

func makeProxy(targetURL string) http.Handler {
	u, _ := url.Parse(targetURL)
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("proxy error: %v", err)
		http.Error(w, "backend unavailable", http.StatusBadGateway)
	}
	return proxy
}

func recentlyOnline(state *backendState, timeout time.Duration) bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	return !state.lastOnline.IsZero() && time.Since(state.lastOnline) < timeout
}

func setOnline(state *backendState) {
	state.mu.Lock()
	state.lastOnline = time.Now()
	state.mu.Unlock()
}

func shouldSendWOL(backend Target, host, path string) bool {
	if !backend.WOL {
		return false
	}
	for _, h := range backend.IgnoredHosts {
		if h == host {
			return false
		}
	}
	for _, p := range backend.IgnoredPaths {
		if p == path {
			return false
		}
	}
	return true
}

// Handler

func handler(cfg *Config) http.HandlerFunc {
	skipTimeout := time.Duration(cfg.General.SkipCheckTimeout) * time.Second
	proxy := makeProxy(cfg.General.Destination)

	return func(w http.ResponseWriter, r *http.Request) {
		clientIP := r.RemoteAddr
		host := r.Host
		path := r.URL.Path
		log.Printf("[%s] Request host=%s path=%s", clientIP, host, path)

		if !strings.Contains(host, cfg.General.MainHostKeyword) {
			io.WriteString(w, "Host does not match main backend target")
			return
		}

		// Check backends and send WoL
		for name, backend := range cfg.Backends {
			state := backendStates[name]
			up := false

			if recentlyOnline(state, skipTimeout) {
				up = true
			} else if checkHealth(backend.Destination) {
				up = true
				setOnline(state)
			}

			if !up && shouldSendWOL(backend, host, path) {
				log.Printf("Backend %s down -> sending WoL", name)
				if err := sendWOL(backend.MacAddress, backend.BroadcastIP, backend.WolPort); err != nil {
					log.Printf("WOL %s failed: %v", name, err)
				}
			}
		}

		// Always forward request to main service, if up
		if checkHealth(cfg.General.Destination) {
			proxy.ServeHTTP(w, r)
			return
		}

		http.Error(w, "Destination backend unavailable", http.StatusServiceUnavailable)
	}
}

// Main

func main() {
	configFile := "config.toml"
	if len(os.Args) > 1 {
		configFile = os.Args[1]
	}

	cfg, err := LoadConfig(configFile)
	if err != nil {
		log.Fatalf("Failed to load config file: %v", err)
	}

	if cfg.General.Listen == "" {
		cfg.General.Listen = ":8080"
	}
	if cfg.General.SkipCheckTimeout == 0 {
		cfg.General.SkipCheckTimeout = 30
	}

	http.HandleFunc("/", handler(cfg))
	log.Printf("Proxy listening on %s", cfg.General.Listen)
	log.Fatal(http.ListenAndServe(cfg.General.Listen, nil))
}
