package daemoset

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	sambanovav1alpha1 "github.com/yevgeny-shnaidman/sambanova-gpu-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("SetSNDDaemonSetAsDesired", func() {
	var (
		config  *sambanovav1alpha1.DeviceConfig
		ds      *appsv1.DaemonSet
		handler DaemonSetAPI
	)

	BeforeEach(func() {
		scheme := runtime.NewScheme()
		Expect(sambanovav1alpha1.AddToScheme(scheme)).To(Succeed())
		config = &sambanovav1alpha1.DeviceConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "accelerators", Namespace: "operator", UID: types.UID("config-uid")},
		}
		ds = &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "snd-accelerators", Namespace: config.Namespace}}
		handler = NewDaemonSet(scheme)
	})

	DescribeTable("uses the configured image and its default entrypoint", func(image, expectedImage string) {
		config.Spec.SNDImage = image
		Expect(handler.SetSNDDaemonSetAsDesired(ds, config)).To(Succeed())
		Expect(ds.Spec.Template.Spec.Containers).To(HaveLen(1))
		snd := ds.Spec.Template.Spec.Containers[0]
		Expect(snd.Image).To(Equal(expectedImage))
		Expect(snd.Command).To(BeEmpty())
		Expect(snd.Args).To(BeEmpty())
	},
		Entry("default image", "", "quay.io/yshnaidm/sm-snd"),
		Entry("custom image", "registry.example.com/snd:v2", "registry.example.com/snd:v2"),
	)

	DescribeTable("selects nodes with the corresponding KMM ready label", func(nodeLabels labels.Set, matches bool) {
		Expect(handler.SetSNDDaemonSetAsDesired(ds, config)).To(Succeed())
		selector := labels.SelectorFromSet(ds.Spec.Template.Spec.NodeSelector)
		Expect(selector.Matches(nodeLabels)).To(Equal(matches))
	},
		Entry("matching module and namespace", labels.Set{"kmm.node.kubernetes.io/operator.accelerators.ready": ""}, true),
		Entry("different namespace", labels.Set{"kmm.node.kubernetes.io/other.accelerators.ready": ""}, false),
		Entry("different module", labels.Set{"kmm.node.kubernetes.io/operator.other.ready": ""}, false),
		Entry("missing ready label", labels.Set{}, false),
	)

	It("sets the controller reference and a selector matching its pods", func() {
		Expect(handler.SetSNDDaemonSetAsDesired(ds, config)).To(Succeed())
		Expect(metav1.IsControlledBy(ds, config)).To(BeTrue())
		selector, err := metav1.LabelSelectorAsSelector(ds.Spec.Selector)
		Expect(err).NotTo(HaveOccurred())
		Expect(selector.Matches(labels.Set(ds.Spec.Template.Labels))).To(BeTrue())
	})

	It("runs privileged with the host library directory and LD_LIBRARY_PATH", func() {
		Expect(handler.SetSNDDaemonSetAsDesired(ds, config)).To(Succeed())
		pod := ds.Spec.Template.Spec
		Expect(pod.Containers).To(HaveLen(1))
		snd := pod.Containers[0]
		Expect(snd.SecurityContext.Privileged).To(HaveValue(BeTrue()))
		Expect(snd.Env).To(ContainElement(corev1.EnvVar{Name: "LD_LIBRARY_PATH", Value: "/opt/sambaflow/runtime/kshim_intfc/lib"}))
		Expect(snd.VolumeMounts).To(HaveLen(1))
		Expect(snd.VolumeMounts[0].MountPath).To(Equal("/opt/sambaflow/runtime/kshim_intfc/lib"))
		Expect(pod.Volumes).To(HaveLen(1))
		Expect(pod.Volumes[0].Name).To(Equal(snd.VolumeMounts[0].Name))
		Expect(pod.Volumes[0].HostPath.Path).To(Equal("/var/sm"))
		Expect(pod.Volumes[0].HostPath.Type).To(HaveValue(Equal(corev1.HostPathDirectory)))
	})

	It("shares the host network for SNML gRPC with cluster DNS", func() {
		Expect(handler.SetSNDDaemonSetAsDesired(ds, config)).To(Succeed())
		pod := ds.Spec.Template.Spec
		Expect(pod.HostNetwork).To(BeTrue())
		Expect(pod.DNSPolicy).To(Equal(corev1.DNSClusterFirstWithHostNet))
		Expect(pod.Containers).To(HaveLen(1))
		Expect(pod.Containers[0].Ports).To(ContainElement(corev1.ContainerPort{Name: "snml", ContainerPort: 50053, HostPort: 50053, Protocol: corev1.ProtocolTCP}))
	})

	It("adds and removes the configured image pull secret", func() {
		config.Spec.ImageRepoSecret = &corev1.LocalObjectReference{Name: "pull-secret"}
		Expect(handler.SetSNDDaemonSetAsDesired(ds, config)).To(Succeed())
		Expect(ds.Spec.Template.Spec.ImagePullSecrets).To(Equal([]corev1.LocalObjectReference{{Name: "pull-secret"}}))

		config.Spec.ImageRepoSecret = nil
		Expect(handler.SetSNDDaemonSetAsDesired(ds, config)).To(Succeed())
		Expect(ds.Spec.Template.Spec.ImagePullSecrets).To(BeEmpty())
	})
})
