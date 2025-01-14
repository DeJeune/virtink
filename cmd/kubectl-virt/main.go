package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	virtv1alpha1 "github.com/smartxworks/virtink/pkg/apis/virt/v1alpha1"
	virtclient "github.com/smartxworks/virtink/pkg/generated/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const helpText = `Usage: kubectl virt COMMAND VM_NAME[,VM_NAME...] [options]

Commands:
    start     Power on the VM(s)
    stop      Force power off the VM(s)
    restart   Reset the VM(s)
    shutdown  Gracefully shutdown the VM(s)  
    pause     Pause the VM(s)
    resume    Resume the paused VM(s)

Options:
    -n, --namespace string   Namespace (default "default")
    --all                   Apply command to all VMs in the namespace

Examples:
    # Start a single VM
    kubectl virt start my-vm

    # Start multiple VMs
    kubectl virt start vm1,vm2,vm3

    # Start all VMs in the default namespace
    kubectl virt start --all

    # Start all VMs in a specific namespace
    kubectl virt start --all -n my-namespace

    # Gracefully shutdown a VM
    kubectl virt shutdown my-vm
`

func main() {
	// 设置自定义 Usage
	flag.Usage = func() {
		log.Print(helpText)
		os.Exit(1)
	}

	// 定义命令行参数
	namespacePtr := flag.String("n", "default", "namespace")
	flag.StringVar(namespacePtr, "namespace", "default", "namespace")
	allFlag := flag.Bool("all", false, "Apply command to all VMs in the namespace")

	// 解析命令行参数
	flag.Parse()

	// 检查剩余的非flag参数
	args := flag.Args()
	if len(args) < 1 || (len(args) < 2 && !*allFlag) {
		flag.Usage()
	}

	command := args[0]
	namespace := *namespacePtr

	// Get kubeconfig path
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = filepath.Join(homedir.HomeDir(), ".kube", "config")
	}

	// Verify kubeconfig file exists and is accessible
	if _, err := os.Stat(kubeconfig); err != nil {
		log.Fatalf("Error accessing kubeconfig at %s: %v", kubeconfig, err)
	}

	// 创建 config
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Error building kubeconfig: %v", err)
	}

	// 创建 virtink client
	virtClient, err := virtclient.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating virtink client: %v\n", err)
	}

	// Map commands to power actions
	actionMap := map[string]string{
		"start":    string(virtv1alpha1.VirtualMachinePowerOn),
		"stop":     string(virtv1alpha1.VirtualMachinePowerOff),
		"restart":  string(virtv1alpha1.VirtualMachineReset),
		"shutdown": string(virtv1alpha1.VirtualMachineShutdown),
		"pause":    string(virtv1alpha1.VirtualMachinePause),
		"resume":   string(virtv1alpha1.VirtualMachineResume),
	}

	powerAction, ok := actionMap[command]
	if !ok {
		log.Fatalf("Unknown command: %s\n\n%s", command, helpText)
	}

	var vmNames []string
	if *allFlag {
		// Get all VMs in the namespace
		log.Printf("Listing VMs in namespace '%s'...\n", namespace)
		vmList, err := virtClient.VirtV1alpha1().VirtualMachines(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			log.Fatalf("Error listing VMs in namespace '%s': %v\n", namespace, err)
		}

		// Filter VMs based on their current state
		for _, vm := range vmList.Items {
			// Skip VMs that are already in desired state or transitioning
			switch command {
			case "start":
				if vm.Status.Phase == virtv1alpha1.VirtualMachineRunning ||
					vm.Status.Phase == virtv1alpha1.VirtualMachineScheduling ||
					vm.Status.Phase == virtv1alpha1.VirtualMachineScheduled ||
					vm.Status.Phase == virtv1alpha1.VirtualMachinePending {
					log.Printf("Skipping VM '%s/%s': already running or starting\n", namespace, vm.Name)
					continue
				}
			case "stop", "shutdown":
				if vm.Status.Phase == virtv1alpha1.VirtualMachineFailed ||
					vm.Status.Phase == virtv1alpha1.VirtualMachineSucceeded ||
					vm.Status.Phase == "" {
					log.Printf("Skipping VM '%s/%s': already stopped\n", namespace, vm.Name)
					continue
				}
			case "pause":
				if vm.Status.PowerAction == virtv1alpha1.VirtualMachinePause {
					log.Printf("Skipping VM '%s/%s': already paused or pausing\n", namespace, vm.Name)
					continue
				}
			case "resume":
				if vm.Status.PowerAction != virtv1alpha1.VirtualMachinePause {
					log.Printf("Skipping VM '%s/%s': not paused\n", namespace, vm.Name)
					continue
				}
			}
			vmNames = append(vmNames, vm.Name)
		}
		if len(vmNames) == 0 {
			log.Printf("No eligible VMs found in namespace '%s'\n", namespace)
			return
		}
	} else {
		// For explicitly specified VMs, verify their existence and state first
		vmNames = strings.Split(args[1], ",")
		var validVMNames []string

		for _, vmName := range vmNames {
			vm, err := virtClient.VirtV1alpha1().VirtualMachines(namespace).Get(context.TODO(), vmName, metav1.GetOptions{})
			if err != nil {
				log.Printf("Error getting VM '%s/%s': %v\n", namespace, vmName, err)
				continue
			}

			// Check VM state
			skip := false
			switch command {
			case "start":
				if vm.Status.Phase == virtv1alpha1.VirtualMachineRunning ||
					vm.Status.Phase == virtv1alpha1.VirtualMachineScheduling ||
					vm.Status.Phase == virtv1alpha1.VirtualMachineScheduled ||
					vm.Status.Phase == virtv1alpha1.VirtualMachinePending {
					log.Printf("Skipping VM '%s/%s': already running or starting\n", namespace, vmName)
					skip = true
				}
			case "stop", "shutdown":
				if vm.Status.Phase == virtv1alpha1.VirtualMachineFailed ||
					vm.Status.Phase == virtv1alpha1.VirtualMachineSucceeded ||
					vm.Status.Phase == "" {
					log.Printf("Skipping VM '%s/%s': already stopped\n", namespace, vmName)
					skip = true
				}
			case "pause":
				if vm.Status.PowerAction == virtv1alpha1.VirtualMachinePause {
					log.Printf("Skipping VM '%s/%s': already paused or pausing\n", namespace, vmName)
					skip = true
				}
			case "resume":
				if vm.Status.PowerAction != virtv1alpha1.VirtualMachinePause {
					log.Printf("Skipping VM '%s/%s': not paused\n", namespace, vmName)
					skip = true
				}
			}

			if !skip {
				validVMNames = append(validVMNames, vmName)
			}
		}

		if len(validVMNames) == 0 {
			log.Printf("No eligible VMs found among specified VMs in namespace '%s'\n", namespace)
			return
		}
		vmNames = validVMNames
	}

	// Process each VM
	for _, vmName := range vmNames {
		// 构建 patch 数据
		patchData := fmt.Sprintf(`{"status":{"powerAction":"%s"}}`, powerAction)

		// 执行 patch 操作
		_, err = virtClient.VirtV1alpha1().VirtualMachines(namespace).Patch(
			context.TODO(),
			vmName,
			types.MergePatchType,
			[]byte(patchData),
			metav1.PatchOptions{},
			"status",
		)
		if err != nil {
			log.Printf("Error patching VM '%s/%s': %v\n", namespace, vmName, err)
			continue
		}

		log.Printf("Successfully sent %s command to VM '%s/%s'\n", command, namespace, vmName)
	}
}
