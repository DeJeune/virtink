package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualMachineConsoleOptions contains options for console connection
type VirtualMachineConsoleOptions struct {
	metav1.TypeMeta `json:",inline"`

	// Type specifies the console type: "serial" or "console"
	// Default is "serial"
	Type string `json:"type,omitempty"`
}

// TerminalSize represents the width and height of a terminal
type TerminalSize struct {
	Width  uint16 `json:"width"`
	Height uint16 `json:"height"`
}
