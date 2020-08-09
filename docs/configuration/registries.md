# Configuring Container Registries

Argo CD Image Updater comes with support for the following registries out of the
box:

* Docker Hub Registry
* Google Container Registry
* RedHat Quay Registry

Adding additional (and custom) container registries is supported by means of a
configuration file. If you run Argo CD Image Updater within Kubernetes, you can
edit the registries in a ConfigMap resource, which will get mounted to the pod
running Argo CD Image Updater.

## Configuring a custom container registry

A sample configuration configuring a couple of registries might look like the
following:

```yaml
registries:
- name: Docker Hub
  api_url: https://registry-1.docker.io
  ping: yes
  credentials: secret:foo/bar#creds
- name: Google Container Registry
  api_url: https://gcr.io
  prefix: gcr.io
  ping: no
  credentials: pullsecret:foo/bar
- name: RedHat Quay
  api_url: https://quay.io
  ping: no
  prefix: quay.io
  credentials: env:REGISTRY_SECRET
```

The above example defines access to three registries. The properties have the
following semantics:

* `name` is just a symbolic name for the registry. Must be unique.
* `api_url` is the base URL (without `/v2` suffix) to the API of the registry
* `ping` specifies whether to send a ping request to `/v2` endpoint first.
  Some registries don't support this.
* `prefix` is the prefix used in the image specification. This prefix will
  be consulted when determining the configuration for given image(s).
* `credentials` is a reference to the credentials to use for accessing the
   registry API (see below)

If you want to take above example to the `argocd-image-updater-cm` ConfigMap,
you need to define the key `registries.conf` in the data of the ConfigMap as
below:

```yaml
data:
  registries.conf: |
    registries:
    - name: Docker Hub
      api_url: https://registry-1.docker.io
      ping: yes
      credentials: secret:foo/bar#creds
    - name: Google Container Registry
      api_url: https://gcr.io
      prefix: gcr.io
      ping: no
      credentials: pullsecret:foo/bar
    - name: RedHat Quay
      api_url: https://quay.io
      ping: no
      prefix: quay.io
      credentials: env:REGISTRY_SECRET
```

!!!note
    Argo CD Image Updater pod must be restarted for changes to the registries
    configuration to take effect. There are plans to change this behaviour so
    that changes will be reload automatically in a future release.

## Specifying credentials for accessing container registries

You can optionally specify a reference to a secret or an environment variable
which contain credentials for accessing the container registry with each image.

Credentials can be referenced as follows:

* A typical pull secret, i.e. a secret containing a `.dockerconfigjson` field
  which holds a Docker client configuration with auth information in JSON
  format.

* A custom secret, which has the credentials stored in a configurable field in
  the format `<username>:<password>`

* An environment variable which holds the credentials in the format
  `<username>:<password>`
