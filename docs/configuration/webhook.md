# Webhook Configuration

Image Updater can be configured to respond to webhook notifications from 
various container registries. 

Currently Supported Registries:

- Docker Hub
- GitHub Container Registry
- Harbor
- Quay
- Aliyun ACR
- AWS ECR (via EventBridge CloudEvents)

Using webhooks can help reduce some of the stress that is put on the 
container registries and where you are running Image Updater by reducing the 
need to poll.

## Enabling the Webhook Server

There are two ways to enable the webhook server. You can either use it with 
polling still enabled through the `run` command or just have the webhook 
server running through the webhook command. 

### Enabling with `run` Command
Below is the command for running the webhook server with polling through the `
run` command. The `--enable-webhook` flag is all that is required. The 
default port of the webhook server in this method is `8082`. 
```
argocd-image-updater run --enable-webhook --webhook-port [PORT]
```

### Enabling with `webhook` Command
You can also run the webhook server without polling. See below for the 
command for this. The default port for this method is `8080`.
```
argocd-image-updater webhook --webhook-port [PORT]
```

### Altering Manifests to Use Webhook
If you are running Image Updater within your cluster, to enable the webhook 
you will need to alter the manifests of the application. What you need to 
edit depends on what command you plan to use. 

If you want to use the webhook with polling through the `run` command you 
need to edit the `argocd-image-updater-config` ConfigMap with the following data:
```yaml
data:
  # enable the webhook server
  webhook.enable: true
  # (OPTIONAL) set the port for the webhook server
  webhook.port: <Value between 0 - 65535>
```

If you plan to use the webhook command for the server then the `argocd-image-
updater` Deployment must be updated. Adjustments to the `argocd-image-updater-config` 
ConfigMap are optional. 
```yaml
# argocd-image-updater Deployment, container args need to be changed to webhook
spec:
  template:
    spec:
      containers:
      - args:
        - webhook
---
# (OPTIONAL) argocd-image-updater-config ConfigMap, edit to change the webhook server port
data:
  webhook.port: <Value between 0 - 65535>
```

## Endpoints
The webhook server contains two endpoints, `webhook` and `healthz`. 

The `webhook` endpoint is used to receive and process webhook notifications.

The `healthz` endpoint acts has a health endpoint to check to see if the server is alive.

## Setting Up a Webhook Notification

To set up a webhook notification, refer to your container registries 
documentation on how to do that. Documentation for the supported registries 
can be found here:

- [Docker Hub](https://docs.docker.com/docker-hub/repos/manage/webhooks/)
- [GitHub Container Registry](https://docs.github.com/en/webhooks/webhook-events-and-payloads)
- [Harbor](https://goharbor.io/docs/2.2.0/working-with-projects/project-configuration/configure-webhooks/)
- [Quay](https://docs.quay.io/guides/notifications.html)
- [Aliyun ACR](https://www.alibabacloud.com/help/en/acr/user-guide/manage-webhooks)
- [AWS ECR via EventBridge](#aws-ecr-via-eventbridge-cloudevents) (see below)

For the URL that you set for the webhook, your link should go as the following:

```text
https://app1.example.com/webhook?type=<YOUR_REGISTRY_TYPE>
# Value of `type` for each supported container registry
# Docker = docker.io
# GitHub Container Registry = ghcr.io
# Harbor = harbor
# Quay = quay.io
# Aliyun ACR = aliyun-acr
# AWS ECR (via CloudEvents) = cloudevents
```

### Aliyun ACR Specifics

Aliyun ACR (especially Enterprise Edition) uses various registry endpoints (e.g. `<instance>-registry.cn-shanghai.cr.aliyuncs.com`). Since the webhook payload does not contain the instance name, you must provide the registry URL via the `registry_url` query parameter if it differs from the default `registry.<region>.aliyuncs.com`:

```text
https://app1.example.com/webhook?type=aliyun-acr&registry_url=my-instance-registry.cn-shanghai.cr.aliyuncs.com
```

### AWS ECR via EventBridge CloudEvents

AWS ECR doesn't have built-in webhook support, but you can use AWS EventBridge to
transform ECR push events into CloudEvents format and send them to the webhook server.

For more information about the CloudEvents specification, see the [CloudEvents v1.0 spec](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/spec.md).

#### Prerequisites

- AWS account with ECR repositories
- EventBridge configured in the same region as your ECR
- Network access from EventBridge to your webhook endpoint

#### Setup Steps

1. **Create an EventBridge Rule** to capture ECR push events
2. **Configure an Input Transformer** to convert to CloudEvents format
3. **Create an API Destination** pointing to your webhook endpoint
4. **Set up IAM permissions** for EventBridge to invoke the API destination

#### Input Transformer Configuration

When setting up the EventBridge target, configure the Input Transformer with:

**input_paths:**
```json
{
  "id": "$.id",
  "time": "$.time",
  "account": "$.account",
  "region": "$.region",
  "repo": "$.detail.repository-name",
  "digest": "$.detail.image-digest",
  "tag": "$.detail.image-tag"
}
```

**input_template:**
```json
{
  "specversion": "1.0",
  "id": "<id>",
  "type": "com.amazon.ecr.image.push",
  "source": "urn:aws:ecr:<region>:<account>:repository/<repo>",
  "subject": "<repo>:<tag>",
  "time": "<time>",
  "datacontenttype": "application/json",
  "data": {
    "repositoryName": "<repo>",
    "imageDigest": "<digest>",
    "imageTag": "<tag>",
    "registryId": "<account>"
  }
}
```

> **Note:** Use exact field names in the `data` object (camelCase: `repositoryName`, `imageTag`, `imageDigest`). The registry URL is automatically extracted from the `source` field.

#### Example Terraform Configuration

See `config/examples/cloudevents/terraform/` for a complete Terraform module that sets up:

- EventBridge rule for ECR push events
- Input transformer converting to CloudEvents format
- API destination with authentication
- IAM roles and policies

#### CloudEvents Payload Format

The EventBridge input transformer creates CloudEvents v1.0 compliant payloads:

```json
{
  "specversion": "1.0",
  "id": "<event-id>",
  "type": "com.amazon.ecr.image.push",
  "source": "urn:aws:ecr:<region>:<account>:repository/<repo>",
  "subject": "<repo>:<tag>",
  "time": "<timestamp>",
  "datacontenttype": "application/json",
  "data": {
    "repositoryName": "<repo>",
    "imageDigest": "sha256:...",
    "imageTag": "<tag>",
    "registryId": "<account>"
  }
}
```

#### Webhook URL for ECR

```text
https://your-webhook.example.com/webhook?type=cloudevents&secret=<YOUR_SECRET>
```

Or use the `X-Webhook-Secret` header for authentication (recommended with EventBridge
Connection authentication).

For complete setup instructions and examples, see `config/examples/cloudevents/terraform/` for Terraform configuration.

## Secrets

To help secure the webhook server you can apply a secret that is used to 
validate the incoming notification. The secrets can be set by editing the 
`argocd-image-updater-secret` secret.

```yaml
stringData:
  webhook.docker-secret: <YOUR_SECRET>
  webhook.ghcr-secret: <YOUR_SECRET>
  webhook.harbor-secret: <YOUR_SECRET>
  webhook.quay-secret: <YOUR_SECRET>
  webhook.aliyun-acr-secret: <YOUR_SECRET>
  webhook.cloudevents-secret: <YOUR_SECRET>
```

You also need to configure the webhook notification to use the secret based 
on the methods below. See below for the two ways and which of the supported registries use that.

### Registries With Preexisting Support For Secrets

There are container registries that have built in secrets support. How you 
apply the secret will vary depending on the registry so follow the 
instructions linked in the documentation for that registry. 

Supported Registries That Use This:

- GitHub Container Registry
- Harbor

### Parameter Secrets

Because some container registries do not support secrets, there is a method 
included to have secrets for registries. This is through the query parameters 
in the URL of the webhook. This is not the most secure method and is there 
for a small extra layer.

!!!warning
    This is not the most secure method, it is just here for a small extra layer. 
    **Do not use a secret that is shared with other critical services for this method!**

It can be applied to the URL as below:
```
https://app1.example.com/webhook?type=<YOUR_REGISTRY_TYPE>&secret=<YOUR_SECRET>
```

Supported Registries That Use This:

- Docker Hub
- Quay
- Aliyun ACR
- AWS ECR (via CloudEvents/EventBridge)

Also be aware that if the container registry has a built-in secrets method you will
not be able to use this method.

## Exposing the Server

To expose the webhook server we have provided a service and ingress to get 
started. These manifests are not applied with `install.yaml` so you will need 
to apply them yourself. 

They are located in the `manifets/base/networking` directory.

## Rate Limiting

To prevent overloading from the `/webhook` endpoint which could cause Image 
Updater to use too many resources rate limiting is implemented for the endpoint.

The rate limiting allows for a certain amount of requests per hour. This setting
is configurable and can be set with the configuration value below. If you go over
the limit the request will wait until it is allowed. The rate limit value defaults 
to 0 which means that it is disabled.
```yaml
data:
  # How many requests can be made per second. The default is 0 meaning disabled.
  webhook.ratelimit-allowed: <SOME_NUMBER>
```

## TLS Configuration

!!!warning "Breaking Change"
    Starting with this release, the webhook server runs with **TLS enabled by default**.
    If you previously relied on plain HTTP, you must explicitly opt out by setting the
    `--disable-tls` flag or the `DISABLE_TLS` environment variable.

By default the webhook server listens over HTTPS using TLS 1.3. It loads a TLS
certificate and key from the `argocd-image-updater-tls` Kubernetes Secret (mounted
at `/app/config/tls/`). If the secret is not provided or its fields are empty, the
server automatically generates a self-signed certificate in memory so that TLS is
still active.

### Disabling TLS (plain HTTP)

To revert to plain HTTP, pass `--disable-tls` or set the environment variable:

```bash
argocd-image-updater webhook --disable-tls
```

or via environment variable:

```bash
DISABLE_TLS=true argocd-image-updater webhook
```

### TLS Version and Cipher Configuration

The minimum and maximum TLS versions both default to **1.3**. Valid values are `1.1`, `1.2`, and `1.3`. TLS 1.0 is not supported.

```bash
# Allow TLS 1.2 and 1.3
argocd-image-updater webhook --tlsminversion 1.2 --tlsmaxversion 1.3

# Restrict cipher suites (only applies when min version < 1.3)
argocd-image-updater webhook \
  --tlsminversion 1.2 \
  --tlsciphers TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384:TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
```

!!!note
    TLS 1.3 cipher suites are not configurable by design. The `--tlsciphers` flag only
    affects connections that negotiate TLS 1.2 or lower. A warning is logged if ciphers
    are specified while the minimum version is 1.3.

### Providing Your Own Certificate

To use your own TLS certificate, create the Secret `argocd-image-updater-tls`, which is
configured as an optional volume mount in the install manifest.

```yaml
apiVersion: v1
kind: Secret
type: kubernetes.io/tls
metadata:
  name: argocd-image-updater-tls
  labels:
    app.kubernetes.io/name: argocd-image-updater-tls
    app.kubernetes.io/part-of: argocd-image-updater-controller
data:
  # Base64-encoded TLS certificate and private key
  tls.crt: <BASE64_ENCODED_CERT>
  tls.key: <BASE64_ENCODED_KEY>
```

### Configuring TLS via ConfigMap

TLS settings can also be configured through the `argocd-image-updater-config` ConfigMap:

```yaml
data:
  # Disable TLS (use plain HTTP)
  disable-tls: "true"
  # Minimum TLS version (default: 1.3)
  tls.min-version: "1.2"
  # Maximum TLS version (default: 1.3)
  tls.max-version: "1.3"
  # Colon-separated list of TLS cipher suites
  tls.ciphers: "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384:TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"
```

## Environment Variables

The flags for both the `run` and `webhook` CLI commands can also be set via 
environment variables. Below is the list of which variables correspond to which flag. 

|Environment Variable|Corresponding Flag|
|--------|--------|
|`ENABLE_WEBHOOK`|`--enable-webhook`|
|`WEBHOOK_PORT`|`--webhook-port`|
|`DOCKER_WEBHOOK_SECRET` |`--docker-webhook-secret`|
|`GHCR_WEBHOOK_SECRET` |`--ghcr-webhook-secret`|
|`HARBOR_WEBHOOK_SECRET` |`--harbor-webhook-secret`|
|`QUAY_WEBHOOK_SECRET` |`--quay-webhook-secret`|
|`ALIYUN_ACR_WEBHOOK_SECRET` |`--aliyun-acr-webhook-secret`|
|`CLOUDEVENTS_WEBHOOK_SECRET` |`--cloudevents-webhook-secret`|
|`WEBHOOK_RATELIMIT_ALLOWED`|`--webhook-ratelimit-allowed`|
|`DISABLE_TLS`|`--disable-tls`|
|`TLS_MIN_VERSION`|`--tlsminversion`|
|`TLS_MAX_VERSION`|`--tlsmaxversion`|
|`TLS_CIPHERS`|`--tlsciphers`|

## Adding Support For Other Registries

If the container registry that you use is not supported yet, feel free to 
implement a handler for it.  You can find examples on how the other handlers
are implemented in the `pkg/webhook` directory. If you intend to open a PR for
your handler to be added please update this documentation page to include the
information about yours with the others.

## Example Payloads

Below is a list of links for finding example payloads for each of the container
registries supported. 

- [Docker Hub](https://docs.docker.com/docker-hub/repos/manage/webhooks/#example-webhook-payload)
- [GitHub Container Registry](https://docs.github.com/en/webhooks/webhook-events-and-payloads#example-webhook-delivery)
- [Harbor](https://goharbor.io/docs/2.2.0/working-with-projects/project-configuration/configure-webhooks/)
(View Payload Format Section)
- [Quay](https://docs.quay.io/guides/notifications.html)
(View Repository Push Section)
- [Aliyun ACR](https://www.alibabacloud.com/help/en/acr/user-guide/manage-webhooks)
- [CloudEvents](#aws-ecr-via-eventbridge-cloudevents) (AWS ECR via EventBridge)

## Troubleshooting

This section will cover some potential errors that may arise when sending
notifications to the webhook server. Errors are propagated through the response body.

**Failed to process webhook/webhook event: <SOME_ERROR>**

If you are consistently seeing this error then there may be something wrong
with the handler for that registry or there could be a problem with something
else. If this continuously occurs please open an issue with the error information.

**no handler available for this webhook**

Make sure you included the `type` query parameter for the type of webhook
handler and ensure that it is correct. 

**Missing/incorrect webhook secret**

If you are seeing this message make sure that you have secrets configured
properly in your container registry whether it is through their service
or the query parameters.
