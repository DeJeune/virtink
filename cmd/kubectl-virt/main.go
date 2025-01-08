package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	virtv1alpha1 "github.com/smartxworks/virtink/pkg/apis/virt/v1alpha1"
	virtclient "github.com/smartxworks/virtink/pkg/generated/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const helpText = `Usage: kubectl virt COMMAND VM_NAME [options]

Commands:
    start     Power on the VM
    stop      Force power off the VM
    restart   Reset the VM
    shutdown  Gracefully shutdown the VM  
    pause     Pause the VM
    resume    Resume the paused VM

Examples:
    # Start a VM
    kubectl virt start my-vm

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

	// 解析命令行参数
	flag.Parse()

	// 检查剩余的非flag参数
	args := flag.Args()
	if len(args) < 2 {
		flag.Usage()
	}

	command := args[0]
	vmName := args[1]
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
		log.Fatalf("Error patching VM: %v\n", err)
	}

	log.Printf("Successfully sent %s command to VM %s in namespace %s\n", command, vmName, namespace)
}
