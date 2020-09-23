# Configuring Container Registries

Argo CD Image Updater comes with support for the following registries out of the
box:

* Docker Hub Registry
* Google Container Registry
* RedHat Quay Registry
* GitHub Docker Registry

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
  defaultns: library
- name: Google Container Registry
  api_url: https://gcr.io
  prefix: gcr.io
  ping: no
  credentials: pullsecret:foo/bar
- name: RedHat Quay
  api_url: https://quay.io
  ping: no
  prefix: quay.io
  insecure: yes
  credentials: env:REGISTRY_SECRET
- name: GitHub Container Registry
  api_url: https://docker.pkg.github.com
  ping: no
  tagsortmode: latest_first
```

The above example defines access to three registries. The properties have the
following semantics:

* `name` is just a symbolic name for the registry. Must be unique.

* `api_url` is the base URL (without `/v2` suffix) to the API of the registry

* `ping` specifies whether to send a ping request to `/v2` endpoint first.
  Some registries don't support this.

* `prefix` is the prefix used in the image specification. This prefix will
  be consulted when determining the configuration for given image(s). If no
  prefix is specified, will be used as the default registry. The prefix is
  mandatory, except for one of the registries in the configuration.

* `credentials` (optional) is a reference to the credentials to use for
  accessing the registry API (see below). Credentials can also be specified
  [per image](../images/#specifying-pull-secrets)

* `insecure` (optional) if set to true, does not validate the TLS certificate
  for the connection to the registry. Use with care.

* `defaultns` (optional) defines a default namespace for images that do not
  specify one. For example, Docker Hub uses the default namespace `library`
  which turns an image specification of `nginx:latest` into the canonical name
  `library/nginx:latest`.

* `tagsortmode` (optional) defines whether and how the list of tags is sorted
  chronologically by the registry. Valid values are `latest_first` (the last
  pushed image will appear first in list), `latest_last` (the last pushed image
  will appear last in list) or `none` (the default, list is not chronological
  sorted). If `tagsortmode` is set to one of `latest_first` or `latest_last`,
  Argo CD Image Updater will not request additional meta data from the registry
  if the `<image_alias>.sort-mode` is `latest` but will instead use the sorting
  from the tag list.

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
      defaultns: library
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
which contain credentials for accessing the container registry. If credentials
are configured for a registry, they will be used as a default when accessing
the registry and no dedicated credential is specified for the image that is
being processed.

Credentials can be referenced as follows:

* A typical pull secret, i.e. a secret containing a `.dockerconfigjson` field
  which holds a Docker client configuration with auth information in JSON
  format. This kind of secret is specified using the notation
  `pullsecret:<namespace>/<secret_name>`

* A custom secret, which has the credentials stored in a configurable field in
  the format `<username>:<password>`. This kind of secret is specified using
  the notation `secret:<namespace>/<secret_name>#<field_in_secret>`

* An environment variable which holds the credentials in the format
  `<username>:<password>`. This kind of secret is specified using the notation
  `env:<env_var_name>`.
