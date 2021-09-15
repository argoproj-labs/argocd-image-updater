module github.com/argoproj-labs/argocd-image-updater

go 1.16

require (
	github.com/Masterminds/semver v1.5.0
	github.com/argoproj/argo-cd/v2 v2.1.2
	github.com/argoproj/gitops-engine v0.4.1
	github.com/argoproj/pkg v0.9.1
	github.com/distribution/distribution/v3 v3.0.0-20210820130019-1cdeff259b9d
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/libtrust v0.0.0-20160708172513-aabc10ec26b7 // indirect
	github.com/go-openapi/spec v0.19.5 // indirect
	github.com/opencontainers/go-digest v1.0.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/prometheus/client_golang v1.7.1
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.1.1
	github.com/stretchr/testify v1.6.1
	go.uber.org/ratelimit v0.1.1-0.20201110185707-e86515f0dda9
	golang.org/x/crypto v0.0.0-20201002170205-7f63de1d35b0
	golang.org/x/oauth2 v0.0.0-20210819190943-2bc19b11175f // indirect
	golang.org/x/sync v0.0.0-20201207232520-09787c993a3a
	golang.org/x/sys v0.0.0-20210119212857-b64e53b001e4 // indirect
	gopkg.in/src-d/go-git.v4 v4.13.1
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v1.21.0
	k8s.io/apimachinery v1.21.0
	k8s.io/client-go v11.0.1-0.20190816222228-6d55c1b1f1ca+incompatible
	sigs.k8s.io/kustomize v2.0.3+incompatible
)

replace (
	github.com/golang/protobuf => github.com/golang/protobuf v1.4.2

	google.golang.org/grpc => google.golang.org/grpc v1.15.0

	k8s.io/api => k8s.io/api v0.21.0
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.21.0
	k8s.io/apimachinery => k8s.io/apimachinery v0.21.0
	k8s.io/apiserver => k8s.io/apiserver v0.21.0
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.21.0
	k8s.io/client-go => k8s.io/client-go v0.21.0
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.21.0
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.21.0
	k8s.io/code-generator => k8s.io/code-generator v0.21.0
	k8s.io/component-base => k8s.io/component-base v0.21.0
	k8s.io/component-helpers => k8s.io/component-helpers v0.21.0
	k8s.io/controller-manager => k8s.io/controller-manager v0.21.0
	k8s.io/cri-api => k8s.io/cri-api v0.21.0
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.21.0
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.21.0
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.21.0
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.21.0
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.21.0
	k8s.io/kubectl => k8s.io/kubectl v0.21.0
	k8s.io/kubelet => k8s.io/kubelet v0.21.0
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.21.0
	k8s.io/metrics => k8s.io/metrics v0.21.0
	k8s.io/mount-utils => k8s.io/mount-utils v0.21.0
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.21.0
)
