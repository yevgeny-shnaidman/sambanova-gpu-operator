package daemoset

import (
	kmmlabels "github.com/rh-ecosystem-edge/kernel-module-management/pkg/labels"
	sambanovav1alpha1 "github.com/yevgeny-shnaidman/sambanova-gpu-operator/api/v1alpha1"
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
)

//go:generate mockgen -source=daemoset.go -package=daemoset -destination=mock_daemoset.go DaemonSetAPI
type DaemonSetAPI interface {
	SetSNDDaemonSetAsDesired(ds *appsv1.DaemonSet, devConfig *sambanovav1alpha1.DeviceConfig) error
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
