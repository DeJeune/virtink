package console

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	virtv1alpha1 "github.com/DeJeune/virtink/pkg/apis/virt/v1alpha1"
	subresourcesv1alpha1 "github.com/DeJeune/virtink/pkg/apis/subresources/v1alpha1"
)

const (
	virtDaemonLabelKey   = "name"
	virtDaemonLabelValue = "virt-daemon"
	virtDaemonNamespace  = "virtink-system"
	virtDaemonPort       = 8082
)

// ProxyHandler proxies WebSocket connections to virt-daemon
type ProxyHandler struct {
	kubeClient  kubernetes.Interface
	vm          *virtv1alpha1.VirtualMachine
	consoleOpts *subresourcesv1alpha1.VirtualMachineConsoleOptions
}

// NewProxyHandler creates a new proxy handler
func NewProxyHandler(kubeClient kubernetes.Interface, vm *virtv1alpha1.VirtualMachine, opts *subresourcesv1alpha1.VirtualMachineConsoleOptions) *ProxyHandler {
	return &ProxyHandler{
		kubeClient:  kubeClient,
		vm:          vm,
		consoleOpts: opts,
	}
}

// ServeHTTP implements http.Handler
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Find virt-daemon pod on the target node
	daemonPod, err := h.findVirtDaemonPod(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to find virt-daemon: %v", err), http.StatusServiceUnavailable)
		return
	}

	// Build the WebSocket URL to virt-daemon
	daemonURL, err := h.buildDaemonURL(daemonPod)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to build daemon URL: %v", err), http.StatusInternalServerError)
		return
	}

	// Upgrade client connection to WebSocket
	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool {
			return true
		},
	}
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		klog.Errorf("Failed to upgrade client connection: %v", err)
		return
	}
	defer clientConn.Close()

	// Connect to virt-daemon
	daemonConn, _, err := websocket.DefaultDialer.Dial(daemonURL, nil)
	if err != nil {
		klog.Errorf("Failed to connect to virt-daemon: %v", err)
		return
	}
	defer daemonConn.Close()

	// Proxy data bidirectionally
	h.proxyWebSocket(clientConn, daemonConn)
}

// findVirtDaemonPod finds the virt-daemon pod running on the target node
func (h *ProxyHandler) findVirtDaemonPod(ctx context.Context) (*corev1.Pod, error) {
	// List pods with virt-daemon label on the target node
	podList, err := h.kubeClient.CoreV1().Pods(virtDaemonNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", virtDaemonLabelKey, virtDaemonLabelValue),
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", h.vm.Status.NodeName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list virt-daemon pods: %w", err)
	}

	if len(podList.Items) == 0 {
		return nil, fmt.Errorf("no virt-daemon pod found on node %s", h.vm.Status.NodeName)
	}

	pod := &podList.Items[0]
	if pod.Status.Phase != corev1.PodRunning {
		return nil, fmt.Errorf("virt-daemon pod is not running (current: %s)", pod.Status.Phase)
	}

	return pod, nil
}

// buildDaemonURL builds the WebSocket URL to connect to virt-daemon
func (h *ProxyHandler) buildDaemonURL(pod *corev1.Pod) (string, error) {
	if pod.Status.PodIP == "" {
		return "", fmt.Errorf("virt-daemon pod has no IP")
	}

	// Build URL: ws://<pod-ip>:8082/api/v1/vms/<namespace>/<vm-name>/console?type=<type>
	u := &url.URL{
		Scheme: "ws",
		Host:   fmt.Sprintf("%s:%d", pod.Status.PodIP, virtDaemonPort),
		Path:   fmt.Sprintf("/api/v1/vms/%s/%s/console", h.vm.Namespace, h.vm.Name),
	}

	// Add console type query parameter
	q := u.Query()
	if h.consoleOpts.Type != "" {
		q.Set("type", h.consoleOpts.Type)
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// proxyWebSocket proxies data bidirectionally between client and daemon connections
func (h *ProxyHandler) proxyWebSocket(clientConn, daemonConn *websocket.Conn) {
	errChan := make(chan error, 2)

	// Client -> Daemon
	go func() {
		errChan <- h.copyWebSocket(daemonConn, clientConn, "client->daemon")
	}()

	// Daemon -> Client
	go func() {
		errChan <- h.copyWebSocket(clientConn, daemonConn, "daemon->client")
	}()

	// Wait for either direction to fail or complete
	err := <-errChan
	if err != nil && err != io.EOF {
		klog.V(4).Infof("WebSocket proxy error: %v", err)
	}
}

// copyWebSocket copies WebSocket messages from src to dst
func (h *ProxyHandler) copyWebSocket(dst, src *websocket.Conn, direction string) error {
	for {
		messageType, message, err := src.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				klog.V(4).Infof("WebSocket closed normally (%s)", direction)
				return nil
			}
			return fmt.Errorf("%s read error: %w", direction, err)
		}

		if err := dst.WriteMessage(messageType, message); err != nil {
			return fmt.Errorf("%s write error: %w", direction, err)
		}
	}
}
