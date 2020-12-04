# Configuring Container Registries

Argo CD Image Updater comes with support for the following registries out of the
box:

* Docker Hub Registry
* Google Container Registry (*gcr.io*)
* RedHat Quay Registry (*quay.io*)
* GitHub Docker Packages (*docker.pkg.github.com*)
* GitHub Container Registry (*ghcr.io*)

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
- name: GitHub Docker Packages
  prefix: docker.pkg.github.com
  api_url: https://docker.pkg.github.com
  ping: no
  tagsortmode: latest-first
- name: GitHub Container Registry
  prefix: ghcr.io
  api_url: https://docker.pkg.github.com
  ping: no
  tagsortmode: latest-last
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

* `credsexpire` (optional) can be used to set an expiry time for the
   credentials. The value must be in a `time.Duration` compatible format,
   having a unit suffix, i.e. `5s` for 5 seconds or `2h` for 2 hours. The
   value should be positive, and `0` can be used to disable it (the default).

   From the golang documentation:

   > A duration string is a possibly signed sequence of decimal numbers, each with optional fraction and a unit suffix, such as "300ms", "-1.5h" or "2h45m". Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".

* `insecure` (optional) if set to true, does not validate the TLS certificate
  for the connection to the registry. Use with care.

* `defaultns` (optional) defines a default namespace for images that do not
  specify one. For example, Docker Hub uses the default namespace `library`
  which turns an image specification of `nginx:latest` into the canonical name
  `library/nginx:latest`.

* `tagsortmode` (optional) defines whether and how the list of tags is sorted
  chronologically by the registry. Valid values are `latest-first` (the last
  pushed image will appear first in list), `latest-last` (the last pushed image
  will appear last in list) or `none` (the default, list is not chronological
  sorted). If `tagsortmode` is set to one of `latest-first` or `latest-last`,
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
    - name: GitHub Container Registry
      api_url: https://gcr.io
      ping: no
      prefix: gcr.io
      credentials: ext:/custom/gcr-creds.sh.io
      credsexpire: 5h
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

* A script that outputs credentials on a single line to stdout, in the format
  `<username:password>`. This can be used to support external authentication
  mechanisms. You can specify this kind of secret in the notation
  `ext:/path/to/script`. Please note that the script must be referenced as
  absolute path, and must be executable (i.e. have the `+x` bit set). You
  can add scripts to `argocd-image-updater` by using an init container.

## Credentials caching

By default, credentials specified in registry configuration are read once on
startup and then cached until `argocd-image-updater` is restarted. There are
two strategies to overcome this:

* Use per-image credentials in annotations - credentials will be read every
  time an image update cycle is performed, and your credentials will always
  be up-to-date (i.e. if you update a secret).

* Specify credential expiry time in the registry configuration - if set, the
  registry credentials will have a defined lifetime, and will be re-read from
  the source after expiration. This can be especially useful if you generate
  credentials with a script which returns a token with a limited lifetime,
  i.e. for getting EKS credentials from the aws CLI. For example, if the
  token has a lifetime of 12 hours, you can set `credsexpire: 12h` and Argo
  CD Image Updater will get a new token after 12 hours.