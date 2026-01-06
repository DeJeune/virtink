package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=vm
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.nodeName`

// VirtualMachine is a specification for a VirtualMachine resource
type VirtualMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineSpec   `json:"spec"`
	Status VirtualMachineStatus `json:"status,omitempty"`
}

// VirtualMachineSpec is the spec for a VirtualMachine resource
type VirtualMachineSpec struct {
	NodeSelector   map[string]string           `json:"nodeSelector,omitempty"`
	Affinity       *corev1.Affinity            `json:"affinity,omitempty"`
	Tolerations    []corev1.Toleration         `json:"tolerations,omitempty"`
	Resources      corev1.ResourceRequirements `json:"resources,omitempty"`
	LivenessProbe  *corev1.Probe               `json:"livenessProbe,omitempty"`
	ReadinessProbe *corev1.Probe               `json:"readinessProbe,omitempty"`

	RunPolicy RunPolicy `json:"runPolicy,omitempty"`

	Instance Instance  `json:"instance"`
	Volumes  []Volume  `json:"volumes,omitempty"`
	Networks []Network `json:"networks,omitempty"`
}

// +kubebuilder:validation:Enum=Always;RerunOnFailure;Once;Manual;Halted

type RunPolicy string

const (
	RunPolicyAlways         RunPolicy = "Always"
	RunPolicyRerunOnFailure RunPolicy = "RerunOnFailure"
	RunPolicyOnce           RunPolicy = "Once"
	RunPolicyManual         RunPolicy = "Manual"
	RunPolicyHalted         RunPolicy = "Halted"
)

type Instance struct {
	CPU         CPU          `json:"cpu,omitempty"`
	Memory      Memory       `json:"memory,omitempty"`
	Kernel      *Kernel      `json:"kernel,omitempty"`
	Console     *Console     `json:"console,omitempty"`
	Serial      *Console     `json:"serial,omitempty"`
	Disks       []Disk       `json:"disks,omitempty"`
	FileSystems []FileSystem `json:"fileSystems,omitempty"`
	Interfaces  []Interface  `json:"interfaces,omitempty"`
}

type CPU struct {
	Sockets               uint32 `json:"sockets,omitempty"`
	CoresPerSocket        uint32 `json:"coresPerSocket,omitempty"`
	DedicatedCPUPlacement bool   `json:"dedicatedCPUPlacement,omitempty"`
}

type Memory struct {
	Size      resource.Quantity `json:"size,omitempty"`
	Hugepages *Hugepages        `json:"hugepages,omitempty"`
}

type Hugepages struct {
	// +kubebuilder:default="1Gi"
	// +kubebuilder:validation:Enum="2Mi";"1Gi"
	PageSize string `json:"pageSize,omitempty"`
}

type Kernel struct {
	Image           string            `json:"image"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	Cmdline         string            `json:"cmdline"`
}

type Console struct {
	Mode   string `json:"mode,omitempty"`
	File   string `json:"file,omitempty"`
	Socket string `json:"socket,omitempty"`
	IOMMU  bool   `json:"iommu,omitempty"`
}

type Disk struct {
	Name     string `json:"name"`
	ReadOnly *bool  `json:"readOnly,omitempty"`
}

type FileSystem struct {
	Name string `json:"name"`
}

type Interface struct {
	Name                   string `json:"name"`
	MAC                    string `json:"mac,omitempty"`
	InterfaceBindingMethod `json:",inline"`
}

type InterfaceBindingMethod struct {
	Bridge     *InterfaceBridge     `json:"bridge,omitempty"`
	Masquerade *InterfaceMasquerade `json:"masquerade,omitempty"`
	SRIOV      *InterfaceSRIOV      `json:"sriov,omitempty"`
	VDPA       *InterfaceVDPA       `json:"vdpa,omitempty"`
	VhostUser  *InterfaceVhostUser  `json:"vhostUser,omitempty"`
}

type InterfaceBridge struct {
}

type InterfaceMasquerade struct {
	CIDR string `json:"cidr,omitempty"`
}

type InterfaceSRIOV struct {
}

type InterfaceVDPA struct {
	NumQueues int  `json:"numQueues,omitempty"`
	IOMMU     bool `json:"iommu,omitempty"`
}

type InterfaceVhostUser struct {
}

type Volume struct {
	Name         string `json:"name"`
	VolumeSource `json:",inline"`
}

func (v *Volume) IsHotpluggable() bool {
	return v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.Hotpluggable ||
		v.DataVolume != nil && v.DataVolume.Hotpluggable
}

func (v *Volume) PVCName() string {
	switch {
	case v.PersistentVolumeClaim != nil:
		return v.PersistentVolumeClaim.ClaimName
	case v.DataVolume != nil:
		return v.DataVolume.VolumeName
	default:
		return ""
	}
}

type VolumeSource struct {
	ContainerDisk         *ContainerDiskVolumeSource         `json:"containerDisk,omitempty"`
	CloudInit             *CloudInitVolumeSource             `json:"cloudInit,omitempty"`
	ContainerRootfs       *ContainerRootfsVolumeSource       `json:"containerRootfs,omitempty"`
	PersistentVolumeClaim *PersistentVolumeClaimVolumeSource `json:"persistentVolumeClaim,omitempty"`
	DataVolume            *DataVolumeVolumeSource            `json:"dataVolume,omitempty"`
}

type ContainerDiskVolumeSource struct {
	Image           string            `json:"image"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
}

type CloudInitVolumeSource struct {
	UserData              string `json:"userData,omitempty"`
	UserDataBase64        string `json:"userDataBase64,omitempty"`
	UserDataSecretName    string `json:"userDataSecretName,omitempty"`
	NetworkData           string `json:"networkData,omitempty"`
	NetworkDataBase64     string `json:"networkDataBase64,omitempty"`
	NetworkDataSecretName string `json:"networkDataSecretName,omitempty"`
}

type ContainerRootfsVolumeSource struct {
	Image           string            `json:"image"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	Size            resource.Quantity `json:"size"`
}

type PersistentVolumeClaimVolumeSource struct {
	Hotpluggable bool   `json:"hotpluggable,omitempty"`
	ClaimName    string `json:"claimName"`
}

type DataVolumeVolumeSource struct {
	Hotpluggable bool   `json:"hotpluggable,omitempty"`
	VolumeName   string `json:"volumeName"`
}

type Network struct {
	Name          string `json:"name"`
	NetworkSource `json:",inline"`
}

type NetworkSource struct {
	Pod    *PodNetworkSource    `json:"pod,omitempty"`
	Multus *MultusNetworkSource `json:"multus,omitempty"`
}

type PodNetworkSource struct {
}

type MultusNetworkSource struct {
	NetworkName string `json:"networkName"`
}

// VirtualMachineStatus is the status for a VirtualMachine resource
type VirtualMachineStatus struct {
	Phase        VirtualMachinePhase            `json:"phase,omitempty"`
	VMPodName    string                         `json:"vmPodName,omitempty"`
	VMPodUID     types.UID                      `json:"vmPodUID,omitempty"`
	NodeName     string                         `json:"nodeName,omitempty"`
	PowerAction  VirtualMachinePowerAction      `json:"powerAction,omitempty"`
	Migration    *VirtualMachineStatusMigration `json:"migration,omitempty"`
	Conditions   []metav1.Condition             `json:"conditions,omitempty"`
	VolumeStatus []VolumeStatus                 `json:"volumeStatus,omitempty"`
}

// +kubebuilder:validation:Enum=Pending;Scheduling;Scheduled;Running;Succeeded;Failed;Unknown

type VirtualMachinePhase string

const (
	VirtualMachinePending    VirtualMachinePhase = "Pending"
	VirtualMachineScheduling VirtualMachinePhase = "Scheduling"
	VirtualMachineScheduled  VirtualMachinePhase = "Scheduled"
	VirtualMachineRunning    VirtualMachinePhase = "Running"
	VirtualMachineSucceeded  VirtualMachinePhase = "Succeeded"
	VirtualMachineFailed     VirtualMachinePhase = "Failed"
	VirtualMachineUnknown    VirtualMachinePhase = "Unknown"
)

// +kubebuilder:validation:Enum=PowerOn;PowerOff;Shutdown;Reset;Reboot;Pause;Resume

type VirtualMachinePowerAction string

const (
	VirtualMachinePowerOn  VirtualMachinePowerAction = "PowerOn"
	VirtualMachinePowerOff VirtualMachinePowerAction = "PowerOff"
	VirtualMachineShutdown VirtualMachinePowerAction = "Shutdown"
	VirtualMachineReset    VirtualMachinePowerAction = "Reset"
	VirtualMachineReboot   VirtualMachinePowerAction = "Reboot"
	VirtualMachinePause    VirtualMachinePowerAction = "Pause"
	VirtualMachineResume   VirtualMachinePowerAction = "Resume"
)

type VirtualMachineStatusMigration struct {
	UID                types.UID                    `json:"uid,omitempty"`
	Phase              VirtualMachineMigrationPhase `json:"phase,omitempty"`
	TargetNodeName     string                       `json:"targetNodeName,omitempty"`
	TargetNodeIP       string                       `json:"targetNodeIP,omitempty"`
	TargetNodePort     int                          `json:"targetNodePort,omitempty"`
	TargetVMPodName    string                       `json:"targetVMPodName,omitempty"`
	TargetVMPodUID     types.UID                    `json:"targetVMPodUID,omitempty"`
	TargetVolumePodUID types.UID                    `json:"targetVolumePodUID,omitempty"`
}

type VirtualMachineConditionType string

const (
	VirtualMachineMigratable VirtualMachineConditionType = "Migratable"
	VirtualMachineReady      VirtualMachineConditionType = "Ready"
)

type VolumeStatus struct {
	Name          string               `json:"name"`
	Phase         VolumePhase          `json:"phase,omitempty"`
	HotplugVolume *HotplugVolumeStatus `json:"hotplugVolume,omitempty"`
}

type VolumePhase string

const (
	VolumePending        VolumePhase = "Pending"
	VolumeAttachedToNode VolumePhase = "AttachedToNode"
	VolumeMountedToPod   VolumePhase = "MountedToPod"
	VolumeReady          VolumePhase = "Ready"
	VolumeDetaching      VolumePhase = "Detaching"
)

type HotplugVolumeStatus struct {
	VolumePodName string    `json:"volumePodName,omitempty"`
	VolumePodUID  types.UID `json:"volumePodUID,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualMachineList is a list of VirtualMachine resources
type VirtualMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []VirtualMachine `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=vmm
// +kubebuilder:printcolumn:name="VM",type=string,JSONPath=`.spec.vmName`
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.status.sourceNodeName`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.status.targetNodeName`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.phase`

type VirtualMachineMigration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineMigrationSpec   `json:"spec,omitempty"`
	Status VirtualMachineMigrationStatus `json:"status,omitempty"`
}

type VirtualMachineMigrationSpec struct {
	VMName string `json:"vmName"`
}

type VirtualMachineMigrationStatus struct {
	Phase          VirtualMachineMigrationPhase `json:"phase,omitempty"`
	SourceNodeName string                       `json:"sourceNodeName,omitempty"`
	TargetNodeName string                       `json:"targetNodeName,omitempty"`
}

// +kubebuilder:validation:Enum=Pending;Scheduling;Scheduled;TargetReady;Running;Sent;Succeeded;Failed

type VirtualMachineMigrationPhase string

const (
	VirtualMachineMigrationPending     VirtualMachineMigrationPhase = "Pending"
	VirtualMachineMigrationScheduling  VirtualMachineMigrationPhase = "Scheduling"
	VirtualMachineMigrationScheduled   VirtualMachineMigrationPhase = "Scheduled"
	VirtualMachineMigrationTargetReady VirtualMachineMigrationPhase = "TargetReady"
	VirtualMachineMigrationRunning     VirtualMachineMigrationPhase = "Running"
	VirtualMachineMigrationSent        VirtualMachineMigrationPhase = "Sent"
	VirtualMachineMigrationSucceeded   VirtualMachineMigrationPhase = "Succeeded"
	VirtualMachineMigrationFailed      VirtualMachineMigrationPhase = "Failed"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VirtualMachineMigrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []VirtualMachineMigration `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
// +kubebuilder:resource:shortName=vmrs
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Current",type=integer,JSONPath=`.status.replicas`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`

// VirtualMachineReplicaSet represents a ReplicaSet of VirtualMachine resources
type VirtualMachineReplicaSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineReplicaSetSpec   `json:"spec"`
	Status VirtualMachineReplicaSetStatus `json:"status,omitempty"`
}

// VirtualMachineReplicaSetSpec defines the desired state of VirtualMachineReplicaSet
type VirtualMachineReplicaSetSpec struct {
	// Number of desired VirtualMachines. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Paused indicates that the VirtualMachineReplicaSet controller is paused.
	// When set to true, changes to the VirtualMachineReplicaSet will not be processed
	// by the controller. This allows for manual deletion/addition of VirtualMachines.
	// +optional
	Paused bool `json:"paused,omitempty"`

	// Label selector for VirtualMachines. Existing VirtualMachines selected by this selector
	// will be the ones affected by this VirtualMachineReplicaSet.
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Template describes the VirtualMachine that will be created.
	Template *VirtualMachineTemplateSpec `json:"template"`
}

// VirtualMachineTemplateSpec describes the data a VirtualMachine should have when created from a template
type VirtualMachineTemplateSpec struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	ObjectMeta metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the VirtualMachine.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	Spec VirtualMachineSpec `json:"spec"`
}

// VirtualMachineReplicaSetConditionType defines the condition types of a VirtualMachineReplicaSet
type VirtualMachineReplicaSetConditionType string

const (
	// VirtualMachineReplicaSetReplicaFailure is added when one or more replicas of a VirtualMachineReplicaSet
	// failed to be created or deleted.
	VirtualMachineReplicaSetReplicaFailure VirtualMachineReplicaSetConditionType = "ReplicaFailure"

	// VirtualMachineReplicaSetPaused indicates that the VirtualMachineReplicaSet controller
	// is in a paused state and will not process any changes to the VirtualMachineReplicaSet.
	VirtualMachineReplicaSetPaused VirtualMachineReplicaSetConditionType = "ReplicaPaused"
)

// VirtualMachineReplicaSetCondition represents the state of a VirtualMachineReplicaSet at a certain point
type VirtualMachineReplicaSetCondition struct {
	// Type of VirtualMachineReplicaSet condition
	Type VirtualMachineReplicaSetConditionType `json:"type"`

	// Status of the condition, one of True, False, Unknown
	Status corev1.ConditionStatus `json:"status"`

	// Last time the condition was probed
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`

	// The last time the condition transitioned from one status to another
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// The reason for the condition's last transition
	// +optional
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition
	// +optional
	Message string `json:"message,omitempty"`
}

// VirtualMachineReplicaSetStatus represents the current status of a VirtualMachineReplicaSet
type VirtualMachineReplicaSetStatus struct {
	// Total number of non-terminated VirtualMachines targeted by this VirtualMachineReplicaSet
	// (their labels match the selector).
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// The number of ready VirtualMachines targeted by this VirtualMachineReplicaSet.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// The number of available VirtualMachines (ready for at least minReadySeconds)
	// targeted by this VirtualMachineReplicaSet.
	// +optional
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed VirtualMachineReplicaSet.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Represents the latest available observations of a VirtualMachineReplicaSet's current state.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []VirtualMachineReplicaSetCondition `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualMachineReplicaSetList contains a list of VirtualMachineReplicaSet
type VirtualMachineReplicaSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineReplicaSet `json:"items"`
}
