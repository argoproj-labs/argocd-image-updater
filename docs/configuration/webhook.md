# Webhook Configuration

Image Updater can be configured to respond to webhook notifications from 
various container registries. 

Currently Supported Registries:

- Docker Hub
- GitHub Container Registry
- Harbor
- Quay
- AWS ECR (via EventBridge CloudEvents)
- Google Artifact Registry (via Pub/Sub)

Using webhooks can help reduce some of the stress that is put on the
container registries and where you are running Image Updater by reducing the
need to poll.

## Enabling the Webhook Server

There are two ways to enable the webhook server. You can either use it with 
polling still enabled through the `run` command or just have the webhook 
server running through the webhook command. 

### Enabling with `run` Command
Below is the command for running the webhook server with polling through the `run` command. The `--enable-webhook` flag is all that is required. The
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

The `healthz` endpoint acts as a health endpoint to check if the server is alive.

## Setting Up a Webhook Notification

To set up a webhook notification, refer to your container registries 
documentation on how to do that. Documentation for the supported registries 
can be found here:

- [Docker Hub](https://docs.docker.com/docker-hub/repos/manage/webhooks/)
- [GitHub Container Registry](https://docs.github.com/en/webhooks/webhook-events-and-payloads)
- [Harbor](https://goharbor.io/docs/1.10/working-with-projects/project-configuration/configure-webhooks/)
- [Quay](https://docs.quay.io/guides/notifications.html)
- [AWS ECR via EventBridge](#aws-ecr-via-eventbridge-cloudevents) (see below)
- [Google Artifact Registry via Pub/Sub](#google-artifact-registry)

For the URL that you set for the webhook, your link should go as the following:

```text
https://app1.example.com/webhook?type=<YOUR_REGISTRY_TYPE>
# Value of `type` for each supported container registry
# Docker = docker.io
# GitHub Container Registry = ghcr.io
# Harbor = harbor
# Quay = quay.io
# AWS ECR (via CloudEvents) = cloudevents
# Google Artifact Registry (via Pub/Sub) = artifact-registry
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

## Google Artifact Registry

Google Artifact Registry sends notifications via [Pub/Sub](https://cloud.google.com/artifact-registry/docs/configure-notifications).
Image Updater supports both Pub/Sub delivery models:

- **Push**: Pub/Sub sends HTTP POST to your webhook endpoint
- **Pull**: Image Updater pulls messages from a Pub/Sub subscription (no inbound HTTP)

Only `INSERT` actions are processed. Other actions (e.g. `DELETE`) are ignored.

### Push Model

Use this model when you want Pub/Sub to call Image Updater directly over HTTP.

Prerequisites:

- Webhook server is enabled (either `argocd-image-updater run --enable-webhook` or `argocd-image-updater webhook`)
- Pub/Sub push subscription can reach the webhook URL (network/DNS/TLS)

Configure a Pub/Sub push subscription to deliver to:

```text
https://your-webhook.example.com/webhook?type=artifact-registry&secret=<YOUR_SECRET>
```

Pub/Sub push subscriptions can deliver messages in either format (see <https://docs.cloud.google.com/pubsub/docs/push#receive_push> and <https://docs.cloud.google.com/pubsub/docs/payload-unwrapping>):

- **Wrapped** (default): request body is the Pub/Sub push envelope containing base64-encoded `message.data`.
- **Unwrapped** (payload unwrapping): request body is the raw message data.

Image Updater expects the HTTP request body to look like this (fields abbreviated):

```json
{
  "message": {
    "data": "<base64-encoded-json>",
    "messageId": "1234567890",
    "publishTime": "2026-01-01T00:00:00Z"
  },
  "subscription": "projects/<project>/subscriptions/<subscription>"
}
```

Notes:

- `message.data` (if present) must be base64-encoded JSON.
- For unwrapped delivery, the HTTP body must be the Artifact Registry JSON message (and Pub/Sub may include metadata in `x-goog-pubsub-*` headers if enabled).

The Artifact Registry message itself (after base64-decoding `message.data`) is a JSON object. Examples from Google's documentation:

```json
{"action":"INSERT","digest":"us-west1-docker.pkg.dev/my-project/my-repo/hello-world@sha256:..."}
```

```json
{"action":"INSERT","digest":"us-west1-docker.pkg.dev/my-project/my-repo/hello-world@sha256:...","tag":"us-west1-docker.pkg.dev/my-project/my-repo/hello-world:1.1"}
```

```json
{"action":"DELETE","tag":"us-west1-docker.pkg.dev/my-project/my-repo/hello-world:1.1"}
```

The `secret` parameter is validated against `webhook.artifact-registry-secret` (see [Secrets](#secrets)).

### Pull Model

Use this model when you do not want to expose an HTTP endpoint. Image Updater will pull messages from a Pub/Sub subscription.

Notes:

- The pull subscriber currently runs only in controller mode (`argocd-image-updater run`).
- When running multiple replicas, only the leader processes messages.

Enable the pull subscriber via ConfigMap:

```yaml
data:
  artifact-registry.subscriber.enabled: "true"
  artifact-registry.subscriber.project-id: "your-gcp-project-id"
  artifact-registry.subscriber.subscription-id: "your-subscription-id"
  # Optional tuning
  # artifact-registry.subscriber.max-outstanding-messages: "100"
  # artifact-registry.subscriber.max-outstanding-bytes: "104857600" # 100MiB
  # artifact-registry.subscriber.num-goroutines: "4"
  # Optional: path to a mounted service account JSON file (uses ADC if empty)
  # artifact-registry.subscriber.credentials-file: "/app/gcp/service-account.json"
```

Authentication:

- On GKE, prefer Workload Identity / Application Default Credentials (ADC).
- Alternatively, mount a service account JSON file and set `artifact-registry.subscriber.credentials-file`
  (or `GOOGLE_APPLICATION_CREDENTIALS`).

Configuration can also be done via CLI flags or environment variables; see
[Environment Variables](#environment-variables).

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
  webhook.cloudevents-secret: <YOUR_SECRET>
  webhook.artifact-registry-secret: <YOUR_SECRET>
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
- AWS ECR (via CloudEvents/EventBridge)
- Google Artifact Registry (Pub/Sub push)

Also be aware that if the container registry has a built-in secrets method you will
not be able to use this method.

## Exposing the Server

To expose the webhook server we have provided a service and ingress to get 
started. These manifests are not applied with `install.yaml` so you will need 
to apply them yourself. 

They are located in the `config/networking` directory.

## Rate Limiting

To prevent overloading from the `/webhook` endpoint which could cause Image 
Updater to use too many resources rate limiting is implemented for the endpoint.

The rate limiting allows for a certain amount of requests per hour. This setting
is configurable and can be set with the configuration value below. If you go over
the limit the request will wait until it is allowed. The rate limit value defaults 
to 0 which means that it is disabled.
```yaml
data:
  # How many requests can be made per hour. The default is 0 meaning disabled.
  webhook.ratelimit-allowed: <SOME_NUMBER>
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
|`CLOUDEVENTS_WEBHOOK_SECRET` |`--cloudevents-webhook-secret`|
|`ARTIFACT_REGISTRY_WEBHOOK_SECRET` |`--artifact-registry-webhook-secret`|
|`ARTIFACT_REGISTRY_SUBSCRIBER_ENABLED` |`--artifact-registry-subscriber-enabled`|
|`ARTIFACT_REGISTRY_SUBSCRIBER_PROJECT_ID` |`--artifact-registry-subscriber-project`|
|`ARTIFACT_REGISTRY_SUBSCRIBER_SUBSCRIPTION_ID` |`--artifact-registry-subscriber-subscription`|
|`GOOGLE_APPLICATION_CREDENTIALS` |`--artifact-registry-subscriber-credentials-file`|
|`ARTIFACT_REGISTRY_SUBSCRIBER_MAX_OUTSTANDING_MESSAGES` |`--artifact-registry-subscriber-max-outstanding-messages`|
|`ARTIFACT_REGISTRY_SUBSCRIBER_MAX_OUTSTANDING_BYTES` |`--artifact-registry-subscriber-max-outstanding-bytes`|
|`ARTIFACT_REGISTRY_SUBSCRIBER_NUM_GOROUTINES` |`--artifact-registry-subscriber-num-goroutines`|
|`WEBHOOK_RATELIMIT_ALLOWED`|`--webhook-ratelimit-allowed`|

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
- [Harbor](https://goharbor.io/docs/1.10/working-with-projects/project-configuration/configure-webhooks/)
(View JSON Payload Format Section)
- [Quay](https://docs.quay.io/guides/notifications.html)
(View Repository Push Section)
- [CloudEvents](#aws-ecr-via-eventbridge-cloudevents) (AWS ECR via EventBridge)
- [Google Artifact Registry](https://cloud.google.com/artifact-registry/docs/configure-notifications)
- [Pub/Sub push format](https://cloud.google.com/pubsub/docs/push#receive_push)

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
