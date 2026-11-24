# sambanova-gpu-operator
// TODO(user): Add simple overview of use/purpose

## Description
// TODO(user): An in-depth paragraph about your project and overview of use

## Getting Started

### Prerequisites
- go version v1.26.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/sambanova-gpu-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/sambanova-gpu-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## SND application

For each DeviceConfig, the operator reconciles an owned `snd-<deviceconfig-name>`
DaemonSet in the same namespace. SND runs only on nodes carrying the corresponding
KMM Module's `kmm.node.kubernetes.io/<namespace>.<deviceconfig-name>.ready` label.
KMM adds this label once the kernel module is ready and removes it when it is no
longer ready.

The default SND image is `quay.io/yshnaidm/sm-snd`; override it with `spec.sndImage`.
The image's default entrypoint must start SND. `spec.imageRepoSecret`, when set,
is also used to pull the SND image.

SND runs privileged and mounts the node's existing `/var/sm` directory at
`/opt/sambaflow/runtime/kshim_intfc/lib`, which is also its `LD_LIBRARY_PATH`.
The namespace and service account must permit privileged pods, host networking,
and hostPath volumes (for example through an appropriate SCC on OpenShift).

SND uses host networking because it serves SNML gRPC on `127.0.0.1:50053`.
Client pods must run on the same node with `hostNetwork: true` and should set
`dnsPolicy: ClusterFirstWithHostNet`. They can then call `127.0.0.1:50053` directly.
Use non-overlapping DeviceConfig node selections so only one SND instance binds
that address on each node.

## SambaNova device plugin

The operator reconciles `device-plugin-<deviceconfig-name>` in the same namespace
as the DeviceConfig. It uses the same KMM Module ready label as SND. The plugin
uses host networking to call SND at `127.0.0.1:50053` and mounts
`/var/lib/kubelet/device-plugins` to register with kubelet.

The default image is `quay.io/yshnaidm/sm-device-plugin`. Set
`spec.devicePluginImage` to override it and `spec.imageRepoSecret` if the image
requires registry credentials. The image's default entrypoint must start the
device plugin with the local SND endpoint.

The default installation creates `sambanova-gpu-operator-kmm-device-plugin` in
the operator namespace. For DeviceConfigs in other namespaces, create the same
service account there. The namespace and service account must allow privileged
containers, host networking, and hostPath mounts.

When upgrading, the operator clears `spec.devicePlugin` from its KMM Modules.
KMM then removes the DaemonSets it previously managed.

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/sambanova-gpu-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/sambanova-gpu-operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
operator-sdk edit --plugins=helm/v1-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

