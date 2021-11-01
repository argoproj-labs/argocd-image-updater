package webhook

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"gopkg.in/go-playground/webhooks.v5/docker"

	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/pkg/registry"
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

type Event string

type RegistryWebhook interface {
	New(secret string) (RegistryWebhook, error)
	Parse(r *http.Request, events ...Event) (*WebhookEvent, error)
}

var webhookEventCh (chan WebhookEvent) = make(chan WebhookEvent)

// GetWebhookEventChan return a chan for WebhookEvent
func GetWebhookEventChan() chan WebhookEvent {
	return webhookEventCh
}

// StartRegistryHookServer starts a new HTTP server for registry hook on given port
func StartRegistryHookServer(port int) chan error {
	errCh := make(chan error)
	go func() {
		sm := http.NewServeMux()

		regPrefixes := registry.ConfiguredEndpoints()
		for _, prefix := range regPrefixes {
			var regPrefix string = prefix
			if regPrefix == "" {
				regPrefix = "docker.io"
			}
			var path string = fmt.Sprintf("/api/webhook/%s", regPrefix)
			sm.HandleFunc(path, webhookHandler)
		}
		errCh <- http.ListenAndServe(fmt.Sprintf(":%d", port), sm)
	}()
	return errCh
}

func getTagMetadata(regPrefix string, imageName string, tagStr string) (*tag.TagInfo, error) {
	rep, err := registry.GetRegistryEndpoint(regPrefix)
	if err != nil {
		log.Errorf("Could not get registry endpoint for %s", regPrefix)
		return nil, err
	}

	regClient, err := registry.NewClient(rep, rep.Username, rep.Password)
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
	rep, err := registry.GetRegistryEndpoint(regPrefix)
	if err != nil {
		log.Errorf("Could not get registry endpoint %s", regPrefix)
		return ""
	}
	return rep.HookSecret
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
	var webhookEv *WebhookEvent
	var err error

	parts := strings.Split(r.URL.Path, "/")
	regPrefix := parts[3]
	hookSecret := getWebhookSecretByPrefix(regPrefix)

	// TODO: support dockerhub, nexus for now. quay, gcr and ghcr are coming up next
	switch {
	case r.Header.Get("X-Docker-Event") != "":
		log.Debugf("Callback from Dockerhub, X-Docker-Event=%s", r.Header.Get("X-Docker-Event"))
		dockerWebhook := NewDockerWebhook("")
		webhookEv, err = dockerWebhook.Parse(r, (Event(docker.BuildEvent)))
		if err != nil {
			log.Errorf("Could not parse DockerHub payload %v", err)
		}
	case r.Header.Get("X-Nexus-Webhook-Id") != "":
		webhookID := r.Header.Get("X-Nexus-Webhook-Id")
		log.Debugf("Callback from Nexus, X-Nexus-Webhook-Id=%s", webhookID)
		if webhookID != string(RepositoryComponentEvent) {
			log.Debugf("Expecting X-Nexus-Webhook-Id header to be %s, got %s", RepositoryComponentEvent, webhookID)
			return
		}
		nexusHook := NewNexusWebhook(hookSecret)
		webhookEv, err = nexusHook.Parse(r, RepositoryComponentEvent)
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

	log.Debugf("Payload: %v", webhookEv)

	webhookEv.RegistryPrefix = regPrefix

	tagInfo, err := getTagMetadata(regPrefix, webhookEv.ImageName, webhookEv.TagName)
	if err != nil {
		log.Errorf("Could not get tag metadata for %s:%s. Stop updating.", webhookEv.ImageName, webhookEv.TagName)
		return
	}
	webhookEv.Digest = string(tagInfo.Digest[:])
	webhookEv.CreatedAt = tagInfo.CreatedAt

	log.Debugf("HandleEvent: imageName=%s, repoName=%s, tag id=%s", webhookEv.ImageName, webhookEv.RepoName, webhookEv.TagName)
	webhookEventCh <- *webhookEv
}
