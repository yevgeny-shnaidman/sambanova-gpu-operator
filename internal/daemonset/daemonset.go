package daemonset

import (
	"maps"

	kmmlabels "github.com/rh-ecosystem-edge/kernel-module-management/pkg/labels"
	sambanovav1alpha1 "github.com/yevgeny-shnaidman/sambanova-gpu-operator/api/v1alpha1"
	"github.com/yevgeny-shnaidman/sambanova-gpu-operator/internal/constants"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	libraryPath     = "/opt/sambaflow/runtime/kshim_intfc/lib"
	defaultSNDImage = "quay.io/yshnaidm/sm-snd"

	defaultDevicePluginImage       = "quay.io/yshnaidm/sm-device-plugin"
	devicePluginServiceAccount     = "sambanova-gpu-operator-kmm-device-plugin"
	kubeletDevicePluginsVolumeName = "kubelet-device-plugins"
	kubeletDevicePluginsPath       = "/var/lib/kubelet/device-plugins"
)

//go:generate mockgen -source=daemonset.go -package=daemonset -destination=mock_daemonset.go DaemonSetAPI
type DaemonSetAPI interface {
	SetSNDDaemonSetAsDesired(ds *appsv1.DaemonSet, devConfig *sambanovav1alpha1.DeviceConfig) error
	SetDevicePluginDaemonSetAsDesired(ds *appsv1.DaemonSet, devConfig *sambanovav1alpha1.DeviceConfig) error
}

type daemonSet struct {
	scheme *runtime.Scheme
}

func NewDaemonSet(scheme *runtime.Scheme) DaemonSetAPI {
	return &daemonSet{scheme: scheme}
}

func (d *daemonSet) SetSNDDaemonSetAsDesired(ds *appsv1.DaemonSet, devConfig *sambanovav1alpha1.DeviceConfig) error {
	image := devConfig.Spec.SNDImage
	if image == "" {
		image = defaultSNDImage
	}

	labels := map[string]string{"app.kubernetes.io/name": "snd", "app.kubernetes.io/instance": devConfig.Name}
	ds.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
	ds.Spec.Template.Labels = labels
	ds.Spec.Template.Spec.NodeSelector = map[string]string{
		kmmlabels.GetKernelModuleReadyNodeLabel(devConfig.Namespace, devConfig.Name): "",
	}
	// SND binds to loopback; host-networked clients on this node share it.
	ds.Spec.Template.Spec.HostNetwork = true
	ds.Spec.Template.Spec.DNSPolicy = corev1.DNSClusterFirstWithHostNet
	ds.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Name:                     "snd",
			Image:                    image,
			ImagePullPolicy:          corev1.PullAlways,
			TerminationMessagePath:   corev1.TerminationMessagePathDefault,
			TerminationMessagePolicy: corev1.TerminationMessageReadFile,
			SecurityContext:          &corev1.SecurityContext{Privileged: ptr.To(true)},
			Env:                      []corev1.EnvVar{{Name: "LD_LIBRARY_PATH", Value: libraryPath}},
			VolumeMounts:             []corev1.VolumeMount{{Name: "sm", MountPath: libraryPath}},
			Ports:                    []corev1.ContainerPort{{Name: "snml", ContainerPort: 50053, HostPort: 50053, Protocol: corev1.ProtocolTCP}},
		},
	}
	ds.Spec.Template.Spec.Volumes = []corev1.Volume{{
		Name: "sm",
		VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{
			Path: "/var/sm", Type: ptr.To(corev1.HostPathDirectory),
		}},
	}}
	ds.Spec.Template.Spec.ImagePullSecrets = nil
	if devConfig.Spec.ImageRepoSecret != nil {
		ds.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{*devConfig.Spec.ImageRepoSecret}
	}
	return controllerutil.SetControllerReference(devConfig, ds, d.scheme)
}

func (d *daemonSet) SetDevicePluginDaemonSetAsDesired(ds *appsv1.DaemonSet, devConfig *sambanovav1alpha1.DeviceConfig) error {
	image := devConfig.Spec.DevicePluginImage
	if image == "" {
		image = defaultDevicePluginImage
	}

	selectorLabels := map[string]string{
		"app.kubernetes.io/name":     "sambanova-device-plugin",
		"app.kubernetes.io/instance": devConfig.Name,
	}
	podLabels := maps.Clone(selectorLabels)
	podLabels["app.kubernetes.io/component"] = "device-plugin"
	podLabels["app.kubernetes.io/part-of"] = "sambanova-gpu-operator"
	if ds.Labels == nil {
		ds.Labels = make(map[string]string, len(podLabels))
	}
	maps.Copy(ds.Labels, podLabels)
	ds.Spec.Selector = &metav1.LabelSelector{MatchLabels: selectorLabels}
	ds.Spec.Template.Labels = podLabels

	pod := &ds.Spec.Template.Spec
	pod.NodeSelector = map[string]string{
		kmmlabels.GetKernelModuleReadyNodeLabel(devConfig.Namespace, devConfig.Name): "",
	}
	// SND listens on the node's loopback at 127.0.0.1:50053.
	pod.HostNetwork = true
	pod.DNSPolicy = corev1.DNSClusterFirstWithHostNet
	pod.PriorityClassName = "system-node-critical"
	pod.ServiceAccountName = devicePluginServiceAccount
	pod.Containers = []corev1.Container{{
		Name:                     "device-plugin",
		Image:                    image,
		ImagePullPolicy:          corev1.PullAlways,
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		SecurityContext:          &corev1.SecurityContext{Privileged: ptr.To(true)},
		VolumeMounts: []corev1.VolumeMount{
			{Name: kubeletDevicePluginsVolumeName, MountPath: kubeletDevicePluginsPath},
			{Name: "sys", MountPath: "/sys"},
		},
	}}
	pod.Volumes = []corev1.Volume{
		{
			Name: kubeletDevicePluginsVolumeName,
			VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{
				Path: kubeletDevicePluginsPath, Type: ptr.To(corev1.HostPathDirectory),
			}},
		},
		{
			Name: "sys",
			VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{
				Path: "/sys", Type: ptr.To(corev1.HostPathDirectory),
			}},
		},
	}
	pod.Tolerations = []corev1.Toleration{
		{Key: constants.UpgradeTaintTolerationKey, Operator: corev1.TolerationOpEqual, Value: "true", Effect: corev1.TaintEffectNoExecute},
		{Key: corev1.TaintNodeDiskPressure, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
		{Key: corev1.TaintNodeMemoryPressure, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
		{Key: corev1.TaintNodePIDPressure, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
	}
	pod.ImagePullSecrets = nil
	if devConfig.Spec.ImageRepoSecret != nil {
		pod.ImagePullSecrets = []corev1.LocalObjectReference{*devConfig.Spec.ImageRepoSecret}
	}
	return controllerutil.SetControllerReference(devConfig, ds, d.scheme)
}
