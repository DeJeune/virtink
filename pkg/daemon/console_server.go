package daemon

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
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

	socketPath := filepath.Join("/var/lib/kubelet/pods", string(vm.Status.VMPodUID), "volumes/kubernetes.io~empty-dir/virtink/serial.sock")
	if _, err := os.Stat(socketPath); err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "serial socket not found", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("stat serial socket: %v", err), http.StatusInternalServerError)
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

	unixConn, err := net.Dial("unix", socketPath)
	if err != nil {
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("dial serial socket: %v", err)))
		return
	}
	defer unixConn.Close()

	errCh := make(chan error, 2)
	go copyWebSocketToConn(wsConn, unixConn, errCh)
	go copyConnToWebSocket(unixConn, wsConn, errCh)

	select {
	case <-r.Context().Done():
	case <-errCh:
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
