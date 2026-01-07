package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"

	virtv1alpha1 "github.com/DeJeune/virtink/pkg/apis/virt/v1alpha1"
)

func TestBuildVMConfigConsoleSerialDefaults(t *testing.T) {
	vm := &virtv1alpha1.VirtualMachine{
		Spec: virtv1alpha1.VirtualMachineSpec{
			Instance: virtv1alpha1.Instance{
				CPU: virtv1alpha1.CPU{
					Sockets:        1,
					CoresPerSocket: 1,
				},
				Memory: virtv1alpha1.Memory{
					Size: resource.MustParse("1Gi"),
				},
			},
		},
	}

	vmConfig, err := buildVMConfig(context.Background(), vm)
	require.NoError(t, err)
	require.NotNil(t, vmConfig.Console)
	require.NotNil(t, vmConfig.Serial)
	require.Equal(t, "Pty", vmConfig.Console.Mode)
	require.Equal(t, "Tty", vmConfig.Serial.Mode)
}

func TestBuildVMConfigConsoleSerialOverride(t *testing.T) {
	vm := &virtv1alpha1.VirtualMachine{
		Spec: virtv1alpha1.VirtualMachineSpec{
			Instance: virtv1alpha1.Instance{
				CPU: virtv1alpha1.CPU{
					Sockets:        1,
					CoresPerSocket: 1,
				},
				Memory: virtv1alpha1.Memory{
					Size: resource.MustParse("1Gi"),
				},
				Console: &virtv1alpha1.Console{
					Mode:   "Tty",
					File:   "/var/log/vm-console.log",
					Socket: "/var/run/virtink/console.sock",
					IOMMU:  true,
				},
				Serial: &virtv1alpha1.Console{
					Mode:   "Pty",
					File:   "/var/log/vm-serial.log",
					Socket: "/var/run/virtink/serial.sock",
					IOMMU:  false,
				},
			},
		},
	}

	vmConfig, err := buildVMConfig(context.Background(), vm)
	require.NoError(t, err)
	require.NotNil(t, vmConfig.Console)
	require.NotNil(t, vmConfig.Serial)
	require.Equal(t, vm.Spec.Instance.Console.Mode, vmConfig.Console.Mode)
	require.Equal(t, vm.Spec.Instance.Console.File, vmConfig.Console.File)
	require.Equal(t, vm.Spec.Instance.Console.Socket, vmConfig.Console.Socket)
	require.Equal(t, vm.Spec.Instance.Console.IOMMU, vmConfig.Console.Iommu)
	require.Equal(t, vm.Spec.Instance.Serial.Mode, vmConfig.Serial.Mode)
	require.Equal(t, vm.Spec.Instance.Serial.File, vmConfig.Serial.File)
	require.Equal(t, vm.Spec.Instance.Serial.Socket, vmConfig.Serial.Socket)
	require.Equal(t, vm.Spec.Instance.Serial.IOMMU, vmConfig.Serial.Iommu)
}

func TestBuildVMConfigSerialDefaultsToSocket(t *testing.T) {
	vm := &virtv1alpha1.VirtualMachine{
		Spec: virtv1alpha1.VirtualMachineSpec{
			Instance: virtv1alpha1.Instance{
				CPU: virtv1alpha1.CPU{
					Sockets:        1,
					CoresPerSocket: 1,
				},
				Memory: virtv1alpha1.Memory{
					Size: resource.MustParse("1Gi"),
				},
				Serial: &virtv1alpha1.Console{},
			},
		},
	}

	vmConfig, err := buildVMConfig(context.Background(), vm)
	require.NoError(t, err)
	require.NotNil(t, vmConfig.Serial)
	require.Equal(t, "Socket", vmConfig.Serial.Mode)
	require.Equal(t, "/var/run/virtink/serial.sock", vmConfig.Serial.Socket)
}
