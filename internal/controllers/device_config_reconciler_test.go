/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	sambanovav1alpha1 "github.com/yevgeny-shnaidman/sambanova-gpu-operator/api/v1alpha1"
	mock_client "github.com/yevgeny-shnaidman/sambanova-gpu-operator/internal/client"
	"github.com/yevgeny-shnaidman/sambanova-gpu-operator/internal/daemoset"
	"github.com/yevgeny-shnaidman/sambanova-gpu-operator/internal/kmmmodule"

	"github.com/yevgeny-shnaidman/sambanova-gpu-operator/internal/upgrade"
	"go.uber.org/mock/gomock"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	devConfigName      = "devConfigName"
	devConfigNamespace = "devConfigNamespace"
)

var _ = Describe("Reconcile", func() {
	var (
		mockHelper *MockdeviceConfigReconcilerHelperAPI
		dcr        *DeviceConfigReconciler
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockHelper = NewMockdeviceConfigReconcilerHelperAPI(ctrl)
		dcr = &DeviceConfigReconciler{
			helper: mockHelper,
		}
	})

	ctx := context.Background()

	DescribeTable("reconciler error flow", func(setFinalizerError,
		buildConfigMapError,
		handleKMMModuleError,
		handleSNDDaemonSetError,
		handleModuleVersionUpgradeError bool) {
		devConfig := &sambanovav1alpha1.DeviceConfig{}
		if setFinalizerError {
			mockHelper.EXPECT().setFinalizer(ctx, devConfig).Return(fmt.Errorf("some error"))
			goto executeTestFunction
		}
		mockHelper.EXPECT().setFinalizer(ctx, devConfig).Return(nil)

		if buildConfigMapError {
			mockHelper.EXPECT().handleBuildConfigMap(ctx, devConfig).Return(fmt.Errorf("some error"))
			goto executeTestFunction
		}
		mockHelper.EXPECT().handleBuildConfigMap(ctx, devConfig).Return(nil)

		if handleKMMModuleError {
			mockHelper.EXPECT().handleKMMModule(ctx, devConfig).Return(fmt.Errorf("some error"))
			goto executeTestFunction
		}
		mockHelper.EXPECT().handleKMMModule(ctx, devConfig).Return(nil)
		if handleSNDDaemonSetError {
			mockHelper.EXPECT().handleSNDDaemonSet(ctx, devConfig).Return(fmt.Errorf("some error"))
			goto executeTestFunction
		}
		mockHelper.EXPECT().handleSNDDaemonSet(ctx, devConfig).Return(nil)
		if handleModuleVersionUpgradeError {
			mockHelper.EXPECT().handleModuleVersionUpgrade(ctx, devConfig).Return(fmt.Errorf("some error"))
			goto executeTestFunction
		}
		mockHelper.EXPECT().handleModuleVersionUpgrade(ctx, devConfig).Return(nil)

	executeTestFunction:

		res, err := dcr.Reconcile(ctx, devConfig)
		if setFinalizerError || buildConfigMapError || handleKMMModuleError || handleSNDDaemonSetError || handleModuleVersionUpgradeError {
			Expect(err).To(HaveOccurred())
		} else {
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(ctrl.Result{}))
		}
	},
		Entry("good flow, no requeue", false, false, false, false, false),
		Entry("setFinalizer failed", true, false, false, false, false),

		Entry("buildConfigMap failed", false, true, false, false, false),

		Entry("handleKMMModule failed", false, false, true, false, false),
		Entry("handleModuleVersionUpgrade failed", false, false, false, false, true),
		Entry("handleSNDDaemonSet failed", false, false, false, true, false),
	)

	It("device config finalization", func() {
		devConfig := &sambanovav1alpha1.DeviceConfig{}
		devConfig.SetDeletionTimestamp(&metav1.Time{})

		mockHelper.EXPECT().finalizeDeviceConfig(ctx, devConfig).Return(nil)

		res, err := dcr.Reconcile(ctx, devConfig)

		Expect(err).ToNot(HaveOccurred())
		Expect(res).To(Equal(ctrl.Result{}))

		mockHelper.EXPECT().finalizeDeviceConfig(ctx, devConfig).Return(fmt.Errorf("some error"))

		res, err = dcr.Reconcile(ctx, devConfig)
		Expect(err).To(HaveOccurred())
		Expect(res).To(Equal(ctrl.Result{}))
	})
})

var _ = Describe("setFinalizer", func() {
	var (
		kubeClient *mock_client.MockClient
		dcrh       deviceConfigReconcilerHelperAPI
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		kubeClient = mock_client.NewMockClient(ctrl)
		dcrh = newDeviceConfigReconcilerHelper(kubeClient, nil, nil, nil, nil)
	})

	ctx := context.Background()

	It("good flow", func() {
		devConfig := &sambanovav1alpha1.DeviceConfig{}

		kubeClient.EXPECT().Patch(ctx, gomock.Any(), gomock.Any()).Return(nil)

		err := dcrh.setFinalizer(ctx, devConfig)
		Expect(err).ToNot(HaveOccurred())

		err = dcrh.setFinalizer(ctx, devConfig)
		Expect(err).ToNot(HaveOccurred())
	})

	It("error flow", func() {
		devConfig := &sambanovav1alpha1.DeviceConfig{}

		kubeClient.EXPECT().Patch(ctx, gomock.Any(), gomock.Any()).Return(fmt.Errorf("some error"))

		err := dcrh.setFinalizer(ctx, devConfig)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("finalizeDeviceConfig", func() {
	var (
		kubeClient *mock_client.MockClient
		dcrh       deviceConfigReconcilerHelperAPI
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		kubeClient = mock_client.NewMockClient(ctrl)
		dcrh = newDeviceConfigReconcilerHelper(kubeClient, nil, nil, nil, nil)
	})

	ctx := context.Background()
	devConfig := &sambanovav1alpha1.DeviceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      devConfigName,
			Namespace: devConfigNamespace,
		},
	}

	nn := types.NamespacedName{
		Name:      devConfigName,
		Namespace: devConfigNamespace,
	}

	It("failed to get KMM Module", func() {
		gomock.InOrder(

			kubeClient.EXPECT().Get(ctx, nn, gomock.Any()).Return(fmt.Errorf("some error")),
		)

		err := dcrh.finalizeDeviceConfig(ctx, devConfig)
		Expect(err).To(HaveOccurred())
	})

	It("KMM module not found, removing finalizer", func() {
		expectedDevConfig := devConfig.DeepCopy()
		expectedDevConfig.SetFinalizers([]string{})
		controllerutil.AddFinalizer(devConfig, deviceConfigFinalizer)

		gomock.InOrder(

			kubeClient.EXPECT().Get(ctx, nn, gomock.Any()).Return(k8serrors.NewNotFound(schema.GroupResource{}, "moduleName")),
			kubeClient.EXPECT().Patch(ctx, expectedDevConfig, gomock.Any()).Return(nil),
		)

		err := dcrh.finalizeDeviceConfig(ctx, devConfig)
		Expect(err).ToNot(HaveOccurred())
	})

	It("KMM module found, deleting it", func() {
		mod := kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Name:      devConfigName,
				Namespace: devConfigNamespace,
			},
		}

		expectedDevConfig := devConfig.DeepCopy()
		expectedDevConfig.SetFinalizers([]string{})
		controllerutil.AddFinalizer(devConfig, deviceConfigFinalizer)

		gomock.InOrder(

			kubeClient.EXPECT().Get(ctx, nn, gomock.Any()).Do(
				func(_ interface{}, _ interface{}, mod *kmmv1beta1.Module, _ ...client.GetOption) {
					mod.Name = nn.Name
					mod.Namespace = nn.Namespace
				},
			),
			kubeClient.EXPECT().Delete(ctx, &mod).Return(nil),
		)

		err := dcrh.finalizeDeviceConfig(ctx, devConfig)
		Expect(err).ToNot(HaveOccurred())
	})
})

var _ = Describe("handleKMMModule", func() {
	var (
		kubeClient *mock_client.MockClient
		kmmHelper  *kmmmodule.MockKMMModuleAPI
		dcrh       deviceConfigReconcilerHelperAPI
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		kubeClient = mock_client.NewMockClient(ctrl)
		kmmHelper = kmmmodule.NewMockKMMModuleAPI(ctrl)
		dcrh = newDeviceConfigReconcilerHelper(kubeClient, kmmHelper, nil, nil, nil)
	})

	ctx := context.Background()
	devConfig := &sambanovav1alpha1.DeviceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      devConfigName,
			Namespace: devConfigNamespace,
		},
	}

	It("KMM Module does not exist", func() {
		newMod := &kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: devConfig.Namespace,
				Name:      devConfig.Name,
			},
		}
		gomock.InOrder(
			kubeClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Return(k8serrors.NewNotFound(schema.GroupResource{}, "whatever")),
			kmmHelper.EXPECT().SetKMMModuleAsDesired(newMod, devConfig).Return(nil),
			kubeClient.EXPECT().Create(ctx, gomock.Any()).Return(nil),
		)

		err := dcrh.handleKMMModule(ctx, devConfig)
		Expect(err).ToNot(HaveOccurred())
	})

	It("KMM Module exists", func() {
		existingMod := &kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: devConfig.Namespace,
				Name:      devConfig.Name,
			},
		}
		gomock.InOrder(
			kubeClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Do(
				func(_ interface{}, _ interface{}, mod *kmmv1beta1.Module, _ ...client.GetOption) {
					mod.Name = devConfig.Name
					mod.Namespace = devConfig.Namespace
				},
			),
			kmmHelper.EXPECT().SetKMMModuleAsDesired(existingMod, devConfig).Return(nil),
		)

		err := dcrh.handleKMMModule(ctx, devConfig)
		Expect(err).ToNot(HaveOccurred())
	})
})

var _ = Describe("handleSNDDaemonSet", func() {
	var (
		kubeClient      *mock_client.MockClient
		daemonSetHelper *daemoset.MockDaemonSetAPI
		dcrh            deviceConfigReconcilerHelperAPI
		devConfig       *sambanovav1alpha1.DeviceConfig
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		kubeClient = mock_client.NewMockClient(ctrl)
		daemonSetHelper = daemoset.NewMockDaemonSetAPI(ctrl)
		dcrh = newDeviceConfigReconcilerHelper(kubeClient, nil, daemonSetHelper, nil, nil)
		devConfig = &sambanovav1alpha1.DeviceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      devConfigName,
				Namespace: devConfigNamespace,
			},
		}
	})

	DescribeTable("create and patch flow", func(exists, changed, getError, configureError, writeError bool) {
		ctx := context.Background()
		key := types.NamespacedName{Name: "snd-" + devConfig.Name, Namespace: devConfig.Namespace}
		ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace}}
		var calls []any
		calls = append(calls, kubeClient.EXPECT().Get(ctx, key, gomock.Any()).DoAndReturn(
			func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
				if getError {
					return fmt.Errorf("get failed")
				}
				if !exists {
					return k8serrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, key.Name)
				}
				ds.DeepCopyInto(obj.(*appsv1.DaemonSet))
				return nil
			},
		))
		if !getError {
			calls = append(calls, daemonSetHelper.EXPECT().SetSNDDaemonSetAsDesired(ds, devConfig).DoAndReturn(
				func(ds *appsv1.DaemonSet, _ *sambanovav1alpha1.DeviceConfig) error {
					if configureError {
						return fmt.Errorf("configure failed")
					}
					if changed {
						ds.Labels = map[string]string{"test": "updated"}
					}
					return nil
				},
			))
		}
		if !getError && !configureError {
			var writeErr error
			if writeError {
				writeErr = fmt.Errorf("write failed")
			}
			if !exists {
				calls = append(calls, kubeClient.EXPECT().Create(ctx, ds).Return(writeErr))
			} else if changed {
				calls = append(calls, kubeClient.EXPECT().Patch(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, obj client.Object, patch client.Patch, _ ...client.PatchOption) error {
						Expect(client.ObjectKeyFromObject(obj)).To(Equal(key))
						data, err := patch.Data(obj)
						Expect(err).NotTo(HaveOccurred())
						Expect(string(data)).To(MatchJSON(`{"metadata":{"labels":{"test":"updated"}}}`))
						return writeErr
					},
				))
			}
		}
		gomock.InOrder(calls...)

		err := dcrh.handleSNDDaemonSet(ctx, devConfig)
		switch {
		case getError:
			Expect(err).To(MatchError("get failed"))
		case configureError:
			Expect(err).To(MatchError("configure failed"))
		case writeError:
			Expect(err).To(MatchError("write failed"))
		default:
			Expect(err).NotTo(HaveOccurred())
		}
	},
		Entry("creates a missing DaemonSet", false, false, false, false, false),
		Entry("leaves an unchanged DaemonSet alone", true, false, false, false, false),
		Entry("patches a changed DaemonSet", true, true, false, false, false),
		Entry("returns get errors", false, false, true, false, false),
		Entry("returns configuration errors when creating", false, false, false, true, false),
		Entry("returns configuration errors when patching", true, true, false, true, false),
		Entry("returns create errors", false, false, false, false, true),
		Entry("returns patch errors", true, true, false, false, true),
	)
})

var _ = Describe("handleModuleVersionUpgrade", func() {
	var (
		kubeClient    *mock_client.MockClient
		upgradeHelper *upgrade.MockUpgradeAPI
		dcrh          deviceConfigReconcilerHelperAPI
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		kubeClient = mock_client.NewMockClient(ctrl)
		upgradeHelper = upgrade.NewMockUpgradeAPI(ctrl)
		dcrh = newDeviceConfigReconcilerHelper(kubeClient, nil, nil, upgradeHelper, nil)
	})

	ctx := context.Background()
	devConfig := &sambanovav1alpha1.DeviceConfig{
		Spec: sambanovav1alpha1.DeviceConfigSpec{
			DriverVersion: "some verison",
		},
	}
	targetedNodes := []corev1.Node{
		{},
		{},
	}
	upgradedNode := &corev1.Node{}
	nodeForUpgrade := &corev1.Node{}

	DescribeTable("upgrade good and error flow", func(getTargetedNodesError,
		uncordonUpgradedNodeError,
		cordonNodeForUpgradeError,
		kickoffUpgradeError bool) {
		if getTargetedNodesError {
			upgradeHelper.EXPECT().GetTargetedNodes(ctx, devConfig).Return(nil, fmt.Errorf("some error"))
			goto executeTestFunction
		}
		upgradeHelper.EXPECT().GetTargetedNodes(ctx, devConfig).Return(targetedNodes, nil)
		upgradeHelper.EXPECT().GetUpgradedNode(ctx, devConfig, targetedNodes).Return(upgradedNode)
		if uncordonUpgradedNodeError {
			upgradeHelper.EXPECT().UncordonUpgradedNode(ctx, upgradedNode).Return(fmt.Errorf("some error"))
			goto executeTestFunction
		}
		upgradeHelper.EXPECT().UncordonUpgradedNode(ctx, upgradedNode).Return(nil)
		upgradeHelper.EXPECT().GetNodeForUpgrade(ctx, devConfig, targetedNodes).Return(nodeForUpgrade)
		if cordonNodeForUpgradeError {
			upgradeHelper.EXPECT().CordonNodeForUpgrade(ctx, devConfig, nodeForUpgrade).Return(fmt.Errorf("some error"))
			goto executeTestFunction
		}
		upgradeHelper.EXPECT().CordonNodeForUpgrade(ctx, devConfig, nodeForUpgrade).Return(nil)
		if kickoffUpgradeError {
			upgradeHelper.EXPECT().KickoffUpgrade(ctx, devConfig, nodeForUpgrade).Return(fmt.Errorf("some error"))
			goto executeTestFunction
		}
		upgradeHelper.EXPECT().KickoffUpgrade(ctx, devConfig, nodeForUpgrade).Return(nil)

	executeTestFunction:

		err := dcrh.handleModuleVersionUpgrade(ctx, devConfig)
		if getTargetedNodesError || uncordonUpgradedNodeError || cordonNodeForUpgradeError || kickoffUpgradeError {
			Expect(err).To(HaveOccurred())
		} else {
			Expect(err).ToNot(HaveOccurred())
		}
	},
		Entry("good flow, no errors", false, false, false, false),
		Entry("getTargetedNodes failed", true, false, false, false),
		Entry("uncordonUpgradedNode failed", false, true, false, false),
		Entry("cordonNodeForUpgrade failed", false, false, true, false),
		Entry("kickoffUpgrade failed", false, false, false, true),
	)

	It("driverVersion is not defined in the Spec", func() {
		devConfig := &sambanovav1alpha1.DeviceConfig{}
		err := dcrh.handleModuleVersionUpgrade(ctx, devConfig)
		Expect(err).ToNot(HaveOccurred())
	})
})

var _ = Describe("handleBuildConfigMap", func() {
	var (
		kubeClient *mock_client.MockClient
		kmmHelper  *kmmmodule.MockKMMModuleAPI
		dcrh       deviceConfigReconcilerHelperAPI
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		kubeClient = mock_client.NewMockClient(ctrl)
		kmmHelper = kmmmodule.NewMockKMMModuleAPI(ctrl)
		dcrh = newDeviceConfigReconcilerHelper(kubeClient, kmmHelper, nil, nil, nil)
	})

	ctx := context.Background()
	devConfig := &sambanovav1alpha1.DeviceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      devConfigName,
			Namespace: devConfigNamespace,
		},
	}

	It("BuildConfig does not exist", func() {
		newBuildCM := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: devConfig.Namespace,
				Name:      "dockerfile-" + devConfig.Name,
			},
		}
		gomock.InOrder(
			kubeClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Return(k8serrors.NewNotFound(schema.GroupResource{}, "whatever")),
			kmmHelper.EXPECT().SetBuildConfigMapAsDesired(newBuildCM, devConfig).Return(nil),
			kubeClient.EXPECT().Create(ctx, gomock.Any()).Return(nil),
		)

		err := dcrh.handleBuildConfigMap(ctx, devConfig)
		Expect(err).ToNot(HaveOccurred())
	})

	It("BuildConfig exists", func() {
		existingBuildCM := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: devConfig.Namespace,
				Name:      "dockerfile-" + devConfig.Name,
			},
		}
		gomock.InOrder(
			kubeClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Do(
				func(_ interface{}, _ interface{}, buildCM *v1.ConfigMap, _ ...client.GetOption) {
					buildCM.Name = "dockerfile-" + devConfig.Name
					buildCM.Namespace = devConfig.Namespace
				},
			),
			kmmHelper.EXPECT().SetBuildConfigMapAsDesired(existingBuildCM, devConfig).Return(nil),
		)

		err := dcrh.handleBuildConfigMap(ctx, devConfig)
		Expect(err).ToNot(HaveOccurred())
	})
})
