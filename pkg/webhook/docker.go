package webhook

import (
	"net/http"

	"gopkg.in/go-playground/webhooks.v5/docker"

	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
)

type DockerWebhook struct {
	dockerhub *docker.Webhook
	secret    string
}

// NewDockerWebhook creates and returns a RegistryWebhook instance
func NewDockerWebhook(secret string) RegistryWebhook {
	dockerhook, _ := docker.New()
	hook := DockerWebhook{
		dockerhub: dockerhook,
	}
	return &hook
}

func (hook *DockerWebhook) New(secret string) (RegistryWebhook, error) {
	hook.secret = secret
	return hook, nil
}

func (hook *DockerWebhook) Parse(r *http.Request, events ...Event) (*WebhookEvent, error) {
	pl, err := hook.dockerhub.Parse(r, docker.BuildEvent)
	buildPayload := pl.(docker.BuildPayload)
	if err != nil {
		log.Errorf("Could not parse from Docker %v", err)
	}

	webhookEvent := WebhookEvent{
		ImageName: buildPayload.Repository.Name,
		RepoName:  buildPayload.Repository.RepoName,
		TagName:   buildPayload.PushData.Tag,
	}

	return &webhookEvent, nil
}
