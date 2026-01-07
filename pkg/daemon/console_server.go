package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrl "sigs.k8s.io/controller-runtime"

	virtv1alpha1 "github.com/DeJeune/virtink/pkg/apis/virt/v1alpha1"
)

const (
	consoleEndpointPrefix = "/api/v1/vms/"
	consoleEndpointSuffix = "/console"
)

type ConsoleServer struct {
	client   client.Client
	nodeName string
	addr     string
}

func NewConsoleServer(c client.Client, nodeName string, addr string) *ConsoleServer {
	return &ConsoleServer{
		client:   c,
		nodeName: nodeName,
		addr:     addr,
	}
}

func (s *ConsoleServer) Start(ctx context.Context) error {
	logger := ctrl.Log.WithName("console-server")
	mux := http.NewServeMux()
	mux.HandleFunc(consoleEndpointPrefix, s.handleConsole)

	server := &http.Server{
		Addr:              s.addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error(err, "console server shutdown")
		}
	}()

	logger.Info("starting console server", "addr", s.addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *ConsoleServer) handleConsole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	namespace, name, ok := parseConsolePath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Parse query parameter: type=console or type=serial (default: serial)
	consoleType := r.URL.Query().Get("type")
	if consoleType == "" {
		consoleType = "serial"
	}
	if consoleType != "console" && consoleType != "serial" {
		http.Error(w, "invalid type parameter, must be 'console' or 'serial'", http.StatusBadRequest)
		return
	}

	var vm virtv1alpha1.VirtualMachine
	if err := s.client.Get(r.Context(), types.NamespacedName{Namespace: namespace, Name: name}, &vm); err != nil {
		http.Error(w, fmt.Sprintf("get VM: %v", err), http.StatusNotFound)
		return
	}
	if vm.Status.VMPodUID == "" || vm.Status.NodeName == "" {
		http.Error(w, "VM is not scheduled", http.StatusConflict)
		return
	}
	if s.nodeName != "" && vm.Status.NodeName != s.nodeName {
		http.Error(w, "VM is not on this node", http.StatusConflict)
		return
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer wsConn.Close()

	if consoleType == "console" {
		// Connect to console (pty device)
		s.handleConsolePty(r.Context(), wsConn, vm)
	} else {
		// Connect to serial (unix socket)
		s.handleSerialSocket(r.Context(), wsConn, vm)
	}
}

func (s *ConsoleServer) handleSerialSocket(ctx context.Context, wsConn *websocket.Conn, vm virtv1alpha1.VirtualMachine) {
	socketPath := filepath.Join("/var/lib/kubelet/pods", string(vm.Status.VMPodUID), "volumes/kubernetes.io~empty-dir/virtink/serial.sock")
	if _, err := os.Stat(socketPath); err != nil {
		if os.IsNotExist(err) {
			_ = wsConn.WriteMessage(websocket.TextMessage, []byte("serial socket not found"))
			return
		}
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("stat serial socket: %v", err)))
		return
	}

	// Unix socket path length limit is 108 bytes on Linux.
	// The kubelet pod volume path can exceed this limit.
	// Create a temporary symlink with a shorter path to work around this.
	dialPath := socketPath
	if len(socketPath) > 100 {
		tmpLink := filepath.Join("/tmp", fmt.Sprintf("serial-%s.sock", vm.Status.VMPodUID[:8]))
		_ = os.Remove(tmpLink) // Remove any stale symlink
		if err := os.Symlink(socketPath, tmpLink); err == nil {
			dialPath = tmpLink
			defer os.Remove(tmpLink)
		}
	}

	unixConn, err := net.Dial("unix", dialPath)
	if err != nil {
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("dial serial socket: %v", err)))
		return
	}
	defer unixConn.Close()

	errCh := make(chan error, 2)
	go copyWebSocketToConn(wsConn, unixConn, errCh)
	go copyConnToWebSocket(unixConn, wsConn, errCh)

	select {
	case <-ctx.Done():
	case <-errCh:
	}
}

func (s *ConsoleServer) handleConsolePty(ctx context.Context, wsConn *websocket.Conn, vm virtv1alpha1.VirtualMachine) {
	podBasePath := filepath.Join("/var/lib/kubelet/pods", string(vm.Status.VMPodUID))

	// Read vm-config.json to get console file path
	configPath := filepath.Join(podBasePath, "volumes/kubernetes.io~empty-dir/virtink/vm-config.json")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("read vm-config.json: %v", err)))
		return
	}

	var config struct {
		Console struct {
			Mode string `json:"mode"`
			File string `json:"file"`
		} `json:"console"`
	}
	if err := json.Unmarshal(configData, &config); err != nil {
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("parse vm-config.json: %v", err)))
		return
	}

	if config.Console.Mode != "Pty" {
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("console mode is '%s', not 'Pty'", config.Console.Mode)))
		return
	}

	consolePath := config.Console.File
	if consolePath == "" {
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte("console file path not found in config"))
		return
	}

	// Find the container PID to access its namespace
	// The container is typically named "cloud-hypervisor"
	containerID, err := s.findContainerID(vm.Status.VMPodUID, "cloud-hypervisor")
	if err != nil {
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("find container: %v", err)))
		return
	}

	pid, err := s.getContainerPID(containerID)
	if err != nil {
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("get container PID: %v", err)))
		return
	}

	// Access the pty via /proc/<pid>/root<consolePath>
	ptyPath := filepath.Join("/proc", fmt.Sprintf("%d", pid), "root", consolePath)

	// Open the pty for read/write
	ptyFile, err := os.OpenFile(ptyPath, os.O_RDWR, 0)
	if err != nil {
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("open pty %s: %v", ptyPath, err)))
		return
	}
	defer ptyFile.Close()

	// Bridge WebSocket and PTY
	errCh := make(chan error, 2)
	go copyWebSocketToFile(wsConn, ptyFile, errCh)
	go copyFileToWebSocket(ptyFile, wsConn, errCh)

	select {
	case <-ctx.Done():
	case <-errCh:
	}
}

func (s *ConsoleServer) findContainerID(podUID types.UID, containerName string) (string, error) {
	// Use crictl to find the container ID
	cmd := exec.Command("crictl", "ps", "--pod", string(podUID), "--name", containerName, "-q")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("crictl ps: %w", err)
	}

	containerID := strings.TrimSpace(string(output))
	if containerID == "" {
		return "", fmt.Errorf("container not found")
	}

	return containerID, nil
}

func (s *ConsoleServer) getContainerPID(containerID string) (int, error) {
	// Use crictl to get container info and extract PID
	cmd := exec.Command("crictl", "inspect", containerID)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("crictl inspect: %w", err)
	}

	var info struct {
		Info struct {
			Pid int `json:"pid"`
		} `json:"info"`
	}

	if err := json.Unmarshal(output, &info); err != nil {
		return 0, fmt.Errorf("parse crictl output: %w", err)
	}

	if info.Info.Pid == 0 {
		return 0, fmt.Errorf("PID not found in container info")
	}

	return info.Info.Pid, nil
}

func copyWebSocketToFile(ws *websocket.Conn, file *os.File, errCh chan<- error) {
	for {
		msgType, reader, err := ws.NextReader()
		if err != nil {
			errCh <- err
			return
		}
		if msgType != websocket.BinaryMessage && msgType != websocket.TextMessage {
			continue
		}
		if _, err := io.Copy(file, reader); err != nil {
			errCh <- err
			return
		}
	}
}

func copyFileToWebSocket(file *os.File, ws *websocket.Conn, errCh chan<- error) {
	buf := make([]byte, 4096)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			writer, err := ws.NextWriter(websocket.BinaryMessage)
			if err != nil {
				errCh <- err
				return
			}
			if _, err := writer.Write(buf[:n]); err != nil {
				_ = writer.Close()
				errCh <- err
				return
			}
			if err := writer.Close(); err != nil {
				errCh <- err
				return
			}
		}
		if err != nil {
			errCh <- err
			return
		}
	}
}

func parseConsolePath(path string) (string, string, bool) {
	trimmed := strings.Trim(path, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 6 {
		return "", "", false
	}
	if parts[0] != "api" || parts[1] != "v1" || parts[2] != "vms" || parts[5] != "console" {
		return "", "", false
	}
	if parts[3] == "" || parts[4] == "" {
		return "", "", false
	}
	return parts[3], parts[4], true
}

func copyWebSocketToConn(ws *websocket.Conn, conn net.Conn, errCh chan<- error) {
	for {
		msgType, reader, err := ws.NextReader()
		if err != nil {
			errCh <- err
			return
		}
		if msgType != websocket.BinaryMessage && msgType != websocket.TextMessage {
			continue
		}
		if _, err := io.Copy(conn, reader); err != nil {
			errCh <- err
			return
		}
	}
}

func copyConnToWebSocket(conn net.Conn, ws *websocket.Conn, errCh chan<- error) {
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			writer, err := ws.NextWriter(websocket.BinaryMessage)
			if err != nil {
				errCh <- err
				return
			}
			if _, err := writer.Write(buf[:n]); err != nil {
				_ = writer.Close()
				errCh <- err
				return
			}
			if err := writer.Close(); err != nil {
				errCh <- err
				return
			}
		}
		if err != nil {
			errCh <- err
			return
		}
	}
}
