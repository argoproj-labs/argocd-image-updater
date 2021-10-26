package registry

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"gopkg.in/go-playground/webhooks.v5/docker"

	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/pkg/registry/nexus"
	"github.com/argoproj-labs/argocd-image-updater/pkg/tag"
)

type WebhookEvent struct {
	RegistryPrefix string
	RepoName       string
	ImageName      string
	TagName        string
	CreatedAt      time.Time
	Digest         string
}

var webhookEventCh (chan WebhookEvent) = make(chan WebhookEvent)

// StartRegistryHookServer return a chan for WebhookEvent
func GetWebhookEventChan() chan WebhookEvent {
	return webhookEventCh
}

// StartRegistryHookServer starts a new HTTP server for registry hook on given port
func StartRegistryHookServer(port int) chan error {
	errCh := make(chan error)
	go func() {
		sm := http.NewServeMux()

		for _, reg := range registries {
			log.Debugf("registry prefix: %s, %s", reg.RegistryPrefix, reg.RegistryName)
			var regPrefix string = reg.RegistryPrefix
			if reg.RegistryPrefix == "" {
				regPrefix = "docker.io"
			}
			var path string = fmt.Sprintf("/api/webhook/%s", regPrefix)
			sm.HandleFunc(path, webhookHandler)
		}
		errCh <- http.ListenAndServe(fmt.Sprintf(":%d", port), sm)
	}()
	return errCh
}

func parsePayloadToWebhookEvent(payloadIf interface{}) (webhookEvent WebhookEvent) {
	switch payload := payloadIf.(type) {
	case docker.BuildPayload:
		webhookEvent.ImageName = payload.Repository.Name
		webhookEvent.RepoName = payload.Repository.RepoName
		webhookEvent.TagName = payload.PushData.Tag
	case nexus.RepositoryComponentPayload:
		webhookEvent.ImageName = payload.Component.Name
		webhookEvent.RepoName = payload.RepositoryName
		webhookEvent.TagName = payload.Component.Version
	}
	return webhookEvent
}

func getTagMetadata(regPrefix string, imageName string, tagStr string) (*tag.TagInfo, error) {
	rep, err := GetRegistryEndpoint(regPrefix)
	if err != nil {
		log.Errorf("Could not get registry endpoint for %s", regPrefix)
		return nil, err
	}

	regClient, err := NewClient(rep, "", "")
	if err != nil {
		log.Errorf("Could not creating new registry client for %s", regPrefix)
		return nil, err
	}

	var nameInRegistry string
	if len := len(strings.Split(imageName, "/")); len == 1 && rep.DefaultNS != "" {
		nameInRegistry = rep.DefaultNS + "/" + imageName
		log.Debugf("Using canonical image name '%s' for image '%s'", nameInRegistry, imageName)
	} else {
		nameInRegistry = imageName
	}

	err = regClient.NewRepository(nameInRegistry)
	if err != nil {
		log.Errorf("Could not create new repository for %s", nameInRegistry)
		return nil, err
	}

	manifest, err := regClient.Manifest(tagStr)
	if err != nil {
		log.Errorf("Could not fetch manifest for %s:%s - no manifest returned by registry: %v", regPrefix, tagStr, err)
		return nil, err
	}

	tagInfo, err := regClient.TagMetadata(manifest)
	if err != nil {
		log.Errorf("Could not fetch metadata for %s:%s - no metadata returned by registry: %v", regPrefix, tagStr, err)
		return nil, err
	}

	return tagInfo, nil
}

func getWebhookSecretByPrefix(regPrefix string) string {
	for _, reg := range registries {
		if regPrefix == reg.RegistryPrefix {
			return reg.HookSecret
		}
	}
	return ""
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
	var payload interface{}
	var err error

	parts := strings.Split(r.URL.Path, "/")
	regPrefix := parts[3]
	hookSecret := getWebhookSecretByPrefix(regPrefix)

	// TODO: support dockerhub, nexus for now. quay, gcr and ghcr are coming up next
	switch {
	case r.Header.Get("X-Docker-Event") != "":
		log.Debugf("Callback from Dockerhub, X-Docker-Event=%s", r.Header.Get("X-Docker-Event"))
		dockerWebhook, err := docker.New()
		if err != nil {
			log.Errorf("Could not create DockerHub webhook")
			return
		}
		payload, err = dockerWebhook.Parse(r, docker.BuildEvent)
		if err != nil {
			log.Errorf("Could not parse DockerHub payload %v", err)
		}
	case r.Header.Get("X-Nexus-Webhook-Id") != "":
		webhookID := r.Header.Get("X-Nexus-Webhook-Id")
		log.Debugf("Callback from Nexus, X-Nexus-Webhook-Id=%s", webhookID)
		if webhookID != string(nexus.RepositoryComponentEvent) {
			log.Debugf("Expecting X-Nexus-Webhook-Id header to be %s, got %s", nexus.RepositoryComponentEvent, webhookID)
			return
		}
		nexusHook, err := nexus.New(nexus.Options.Secret(hookSecret))
		if err != nil {
			log.Errorf("Could not create Nexus webhook")
			return
		}
		payload, err = nexusHook.Parse(r, nexus.RepositoryComponentEvent)
		if err != nil {
			log.Errorf("Could not parse Nexus payload %v", err)
		}
	default:
		log.Debugf("Ignoring unknown webhook event")
		http.Error(w, "Unknown webhook event", http.StatusBadRequest)
		return
	}

	if err != nil {
		log.Infof("Webhook processing failed: %s", err)
		status := http.StatusBadRequest
		if r.Method != "POST" {
			status = http.StatusMethodNotAllowed
		}

		http.Error(w, fmt.Sprintf("Webhook processing failed: %s", html.EscapeString(err.Error())), status)
		return
	}

	log.Debugf("Payload: %v", payload)

	webhookEvent := parsePayloadToWebhookEvent(payload)
	webhookEvent.RegistryPrefix = regPrefix

	tagInfo, err := getTagMetadata(regPrefix, webhookEvent.ImageName, webhookEvent.TagName)
	if err != nil {
		log.Errorf("Could not get tag metadata for %s:%s. Stop updating.", webhookEvent.ImageName, webhookEvent.TagName)
		return
	}
	webhookEvent.Digest = string(tagInfo.Digest[:])
	webhookEvent.CreatedAt = tagInfo.CreatedAt

	log.Debugf("HandleEvent: imageName=%s, repoName=%s, tag id=%s", webhookEvent.ImageName, webhookEvent.RepoName, webhookEvent.TagName)
	webhookEventCh <- webhookEvent
}
