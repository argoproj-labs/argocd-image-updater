module github.com/argoproj-labs/argocd-image-updater

go 1.14

require (
	github.com/Masterminds/semver v1.5.0
	github.com/argoproj/argo-cd v1.7.4
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/libtrust v0.0.0-20160708172513-aabc10ec26b7 // indirect
	github.com/gorilla/mux v1.7.4 // indirect
	github.com/nokia/docker-registry-client v0.0.0-20201015093031-af1a6d3b4fb1
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/sirupsen/logrus v1.6.0
	github.com/spf13/cobra v1.0.0
	github.com/stretchr/testify v1.6.1
	go.uber.org/ratelimit v0.1.1-0.20201110185707-e86515f0dda9
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	gopkg.in/yaml.v2 v2.3.0
	k8s.io/api v1.18.8
	k8s.io/apimachinery v1.18.8
	k8s.io/client-go v11.0.1-0.20190816222228-6d55c1b1f1ca+incompatible
	k8s.io/kubectl v1.18.8 // indirect
)

replace (
	github.com/golang/protobuf => github.com/golang/protobuf v1.3.2
	github.com/grpc-ecosystem/grpc-gateway => github.com/grpc-ecosystem/grpc-gateway v1.9.5
	github.com/improbable-eng/grpc-web => github.com/improbable-eng/grpc-web v0.0.0-20181111100011-16092bd1d58a

	google.golang.org/grpc => google.golang.org/grpc v1.15.0

	k8s.io/api => k8s.io/api v0.18.8
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.18.8
	k8s.io/apimachinery => k8s.io/apimachinery v0.18.8
	k8s.io/apiserver => k8s.io/apiserver v0.18.8
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.18.8
	k8s.io/client-go => k8s.io/client-go v0.18.8
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.18.8
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.18.8
	k8s.io/code-generator => k8s.io/code-generator v0.18.8
	k8s.io/component-base => k8s.io/component-base v0.18.8
	k8s.io/cri-api => k8s.io/cri-api v0.18.8
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.18.8
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.18.8
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.18.8
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.18.8
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.18.8
	k8s.io/kubectl => k8s.io/kubectl v0.18.8
	k8s.io/kubelet => k8s.io/kubelet v0.18.8
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.18.8
	k8s.io/metrics => k8s.io/metrics v0.18.8
	k8s.io/node-api => k8s.io/node-api v0.18.8
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.18.8
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.18.8
	k8s.io/sample-controller => k8s.io/sample-controller v0.18.8
)
