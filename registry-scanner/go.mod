module github.com/argoproj-labs/argocd-image-updater/registry-scanner

go 1.22.3

require (
	github.com/distribution/distribution/v3 v3.0.0-20230722181636-7b502560cad4
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.9.0
	k8s.io/api v0.31.2
	k8s.io/apimachinery v0.31.2
	k8s.io/client-go v0.31.2
	sigs.k8s.io/kustomize/api v0.12.1
	sigs.k8s.io/kustomize/kyaml v0.13.9
)

require (
	github.com/Masterminds/semver/v3 v3.2.1
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	golang.org/x/sys v0.21.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
