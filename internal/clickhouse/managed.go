package clickhouse

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// ManagedServer manages a ClickHouse subprocess.
type ManagedServer struct {
	cmd      *exec.Cmd
	tcpPort  int
	httpPort int
	dataDir  string
}

// NewManagedServer creates a new managed ClickHouse server configuration.
func NewManagedServer(dataDir string) *ManagedServer {
	return &ManagedServer{dataDir: dataDir}
}

// Start launches the ClickHouse server process and waits for it to become ready.
func (m *ManagedServer) Start(ctx context.Context, binaryPath string) error {
	tcpPort, httpPort, err := findAvailablePorts()
	if err != nil {
		return fmt.Errorf("finding available ports: %w", err)
	}
	m.tcpPort = tcpPort
	m.httpPort = httpPort

	configPath, err := writeConfigXML(m.dataDir, tcpPort, httpPort)
	if err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	log.Printf("Starting managed ClickHouse (tcp=%d, http=%d, data=%s)", tcpPort, httpPort, m.dataDir)

	m.cmd = exec.CommandContext(ctx, binaryPath, "server", "--config-file="+configPath)
	m.cmd.Stdout = os.Stdout
	m.cmd.Stderr = os.Stderr

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("starting ClickHouse: %w", err)
	}

	// Wait for ClickHouse to be ready (poll TCP port)
	if err := m.waitReady(30 * time.Second); err != nil {
		m.Stop()
		return fmt.Errorf("ClickHouse failed to start: %w", err)
	}

	log.Printf("Managed ClickHouse ready on port %d", tcpPort)
	return nil
}

// Stop gracefully stops the ClickHouse process.
func (m *ManagedServer) Stop() {
	if m.cmd == nil || m.cmd.Process == nil {
		return
	}

	log.Println("Stopping managed ClickHouse...")

	if runtime.GOOS == "windows" {
		m.cmd.Process.Kill()
		m.cmd.Wait()
		return
	}

	// SIGTERM for graceful shutdown
	m.cmd.Process.Signal(os.Interrupt)

	done := make(chan error, 1)
	go func() { done <- m.cmd.Wait() }()

	select {
	case <-done:
		log.Println("Managed ClickHouse stopped.")
	case <-time.After(10 * time.Second):
		log.Println("ClickHouse did not stop gracefully, killing...")
		m.cmd.Process.Kill()
		<-done
	}
}

// TCPPort returns the native TCP port.
func (m *ManagedServer) TCPPort() int {
	return m.tcpPort
}

// HTTPPort returns the HTTP port.
func (m *ManagedServer) HTTPPort() int {
	return m.httpPort
}

// waitReady polls the TCP port until ClickHouse accepts connections.
func (m *ManagedServer) waitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("127.0.0.1:%d", m.tcpPort)

	for time.Now().Before(deadline) {
		// Check if process died
		if m.cmd.ProcessState != nil {
			return fmt.Errorf("process exited with: %v", m.cmd.ProcessState)
		}

		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for ClickHouse on %s", addr)
}
