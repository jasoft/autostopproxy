package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// ServiceManager manages the state of the docker service and proxy
type ServiceManager struct {
	PathPrefix   string
	ContainerID  string
	TargetURL    *url.URL
	HealthPath   string
	Timeout      time.Duration
	DockerCli    *client.Client
	Proxy        *httputil.ReverseProxy
	
	mu           sync.Mutex
	lastAccess   time.Time
	running      bool
	stopTimer    *time.Timer
}

func NewServiceManager(cli *client.Client, pathPrefix, containerID, target, healthPath string, timeout time.Duration) (*ServiceManager, error) {
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	sm := &ServiceManager{
		PathPrefix:  pathPrefix,
		ContainerID: containerID,
		TargetURL:   targetURL,
		HealthPath:  healthPath,
		Timeout:     timeout,
		DockerCli:   cli,
		lastAccess:  time.Now(),
	}

	// Create the reverse proxy
	sm.Proxy = httputil.NewSingleHostReverseProxy(targetURL)
	
	// Customize the Director to strip the prefix if needed, 
	// or ensure the path is forwarded correctly.
	// For this example, we'll keep it simple. 
	// Often /ocr/foo -> target/ocr/foo. 
	// If we want /ocr/foo -> target/foo, we'd use Rewrite (Go 1.20+) or modify Request.URL.Path.
	// Let's assume we pass the path through as is.
	originalDirector := sm.Proxy.Director
	sm.Proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Set host header to target host
		req.Host = targetURL.Host
	}
	
	// Custom error handler to handle connection refused during startup race or crashes
	sm.Proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("Proxy error: %v", err)
		http.Error(w, "Service Unavailable: "+err.Error(), http.StatusBadGateway)
	}

	return sm, nil
}

// ensureRunning checks if container is running, if not starts it.
func (sm *ServiceManager) ensureRunning(ctx context.Context) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Update access time
	sm.lastAccess = time.Now()
	
	// Reset stop timer if active
	if sm.stopTimer != nil {
		sm.stopTimer.Stop()
		sm.stopTimer = nil
	}

	// Schedule a new stop check (we use AfterFunc or similar logic)
	// Actually, a simpler way is a background loop or resetting a timer here.
	// Let's use a timer that we reset on every request.
	sm.stopTimer = time.AfterFunc(sm.Timeout, func() {
		sm.checkAndStop()
	})

	// Check if we think it's running. 
	// Ideally we should also check docker status to be sure, 
	// but for performance we might cache 'running' state and handle errors.
	// However, initial check or recovery is needed.
	
	// Let's query docker to be safe and robust.
	inspect, err := sm.DockerCli.ContainerInspect(ctx, sm.ContainerID)
	if err != nil {
		return fmt.Errorf("failed to inspect container: %w", err)
	}

	if inspect.State.Running {
		sm.running = true
		return nil
	}

	log.Printf("Container %s is not running. Starting...", sm.ContainerID)
	if err := sm.DockerCli.ContainerStart(ctx, sm.ContainerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Wait for port to be ready
	if err := sm.waitForPort(ctx); err != nil {
		return fmt.Errorf("container started but port not reachable: %w", err)
	}

	// Wait for health check if configured
	if sm.HealthPath != "" {
		if err := sm.waitForHealth(ctx); err != nil {
			return fmt.Errorf("container started but health check failed: %w", err)
		}
	}

	sm.running = true
	log.Printf("Container %s started and ready.", sm.ContainerID)
	return nil
}

func (sm *ServiceManager) waitForPort(ctx context.Context) error {
	// Simple retry loop connecting to the target TCP port
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second) // Max wait 30s
	defer cancel()

	host := sm.TargetURL.Host // e.g. localhost:9000
	
	for {
		select {
		case <-timeoutCtx.Done():
			return timeoutCtx.Err()
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", host, 500*time.Millisecond)
			if err == nil {
				conn.Close()
				return nil
			}
		}
	}
}

func (sm *ServiceManager) waitForHealth(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Wait up to 60 seconds for application to initialize
	timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	healthURL := sm.TargetURL.ResolveReference(&url.URL{Path: sm.HealthPath}).String()
	log.Printf("Waiting for health check at %s...", healthURL)

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	for {
		select {
		case <-timeoutCtx.Done():
			return timeoutCtx.Err()
		case <-ticker.C:
			resp, err := client.Get(healthURL)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
				// log.Printf("Health check status: %d", resp.StatusCode)
			}
		}
	}
}

func (sm *ServiceManager) checkAndStop() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if time.Since(sm.lastAccess) >= sm.Timeout {
		log.Printf("Idle timeout reached (%v). Stopping container %s...", sm.Timeout, sm.ContainerID)
		ctx := context.Background() // Background context for stopping
		
		// Set a timeout for the stop operation itself
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		if err := sm.DockerCli.ContainerStop(ctx, sm.ContainerID, container.StopOptions{}); err != nil {
			log.Printf("Error stopping container: %v", err)
		} else {
			log.Printf("Container %s stopped.", sm.ContainerID)
			sm.running = false
		}
	} else {
		// Activity detected since timer fired.
		// ensureRunning() must have updated lastAccess and started a new timer.
		// So we don't need to reschedule here.
		log.Printf("Activity detected, postponing stop.")
	}
}

func (sm *ServiceManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Intercept request
	if err := sm.ensureRunning(r.Context()); err != nil {
		http.Error(w, fmt.Sprintf("Failed to start service: %v", err), http.StatusServiceUnavailable)
		return
	}

	// Forward request
	sm.Proxy.ServeHTTP(w, r)
}

func main() {
	route := flag.String("route", "/ocr", "Path prefix to forward")
	containerName := flag.String("container", "ocr-service", "Docker container Name or ID")
	target := flag.String("target", "http://localhost:8081", "Target URL of the service (where the container listens)")
	listenAddr := flag.String("listen", ":8080", "Address for the proxy to listen on")
	timeoutStr := flag.String("timeout", "15m", "Idle timeout before stopping service")
	healthPath := flag.String("health", "", "Path to check for service health (e.g., /health). Waits until 200 OK.")

	flag.Parse()

	timeout, err := time.ParseDuration(*timeoutStr)
	if err != nil {
		log.Fatalf("Invalid timeout format: %v", err)
	}

	// Init Docker Client
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Failed to create docker client: %v", err)
	}
	defer cli.Close()

	// Verify we can talk to docker
	_, err = cli.Ping(context.Background())
	if err != nil {
		log.Fatalf("Cannot connect to Docker daemon: %v", err)
	}

	sm, err := NewServiceManager(cli, *route, *containerName, *target, *healthPath, timeout)
	if err != nil {
		log.Fatalf("Failed to create service manager: %v", err)
	}

	mux := http.NewServeMux()
	// Handle the specific route prefix.
	// Note: Handle "/" matches everything, but usually you want precise matching.
	// If route is "/ocr", we want to match "/ocr" and "/ocr/..."
	
	// We wrap the handler to strip the prefix if that's desired behavior? 
	// The prompt said "proxy specific url", usually means mapping.
	// If I request localhost:8080/ocr, it goes to target/ocr or target/? 
	// Standard reverse proxy usually preserves path unless rewritten.
	// Let's assume preservation.
	
	mux.Handle(*route, sm)
	if *route != "/" {
		mux.Handle(*route+"/", sm) // Handle subpaths if not root
	}

	log.Printf("Proxy listening on %s", *listenAddr)
	log.Printf("Forwarding %s -> %s (Container: %s, Timeout: %v)", *route, *target, *containerName, timeout)
	if *healthPath != "" {
		log.Printf("Health check enabled on path: %s", *healthPath)
	}

	if err := http.ListenAndServe(*listenAddr, mux); err != nil {
		log.Fatalf("Server exited: %v", err)
	}
}
