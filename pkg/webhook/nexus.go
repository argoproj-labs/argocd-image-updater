package webhook

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
)

// parse errors
var (
	ErrEventNotFound                = errors.New("event not defined to be parsed")
	ErrEventNotSpecifiedToParse     = errors.New("no Event specified to parse")
	ErrHMACVerificationFailed       = errors.New("HMAC verification failed")
	ErrInvalidHTTPMethod            = errors.New("invalid HTTP Method")
	ErrMissingNexusWebhookIDHeader  = errors.New("missing X-Nexus-Webhook-Id Header")
	ErrMissingNexusWebhookSignature = errors.New("missing X-Nexus-Webhook-Signature Header")
	ErrMissingRegistrySecretConfig  = errors.New("missing registry hook secret config")
	ErrParsingPayload               = errors.New("error parsing payload")
)

// Nexus hook event types
const (
	RepositoryComponentEvent Event = "rm:repository:component"
	RepositoryAssetEvent     Event = "rm:repository:asset"
	RepositoryEvent          Event = "rm:repository"
	AuditEvent               Event = "rm:audit"
	// TODO: double check this AuditEvent string
)

// RepositoryComponentPayload a Nexus build notice
// https://help.sonatype.com/repomanager3/webhooks
type RepositoryComponentPayload struct {
	Timestamp      string `json:"timestamp"`
	NodeID         string `json:"nodeId"`
	Initiator      string `json:"initiator"`
	RepositoryName string `json:"repositoryName"`
	Action         string `json:"action"`
	Component      struct {
		ID          string `json:"id"`
		ComponentID string `json:"componentId"`
		Format      string `json:"format"`
		Name        string `json:"name"`
		Group       string `json:"group"`
		Version     string `json:"version"`
	} `json:"component"`
}

type RepositoryPayload struct {
	Timestamp  string `json:"timestamp"`
	NodeID     string `json:"nodeId"`
	Initiator  string `json:"initiator"`
	Action     string `json:"action"`
	Repository struct {
		Format string `json:"format"`
		Name   string `json:"name"`
		Type   string `json:"type"`
	} `json:"repository"`
}

type RepositoryAssetPayload struct {
	Timestamp      string `json:"timestamp"`
	NodeID         string `json:"nodeId"`
	Initiator      string `json:"initiator"`
	RepositoryName string `json:"repositoryName"`
	Action         string `json:"action"`
	Asset          struct {
		ID      string `json:"id"`
		AssetID string `json:"assetId"`
		Format  string `json:"format"`
		Name    string `json:"name"`
	} `json:"asset"`
}

type AuditPayload struct {
	NodeID    string `json:"nodeId"`
	Initiator string `json:"initiator"`
	Audit     struct {
		Domain     string `json:"domain"`
		Type       string `json:"type"`
		Context    string `json:"context"`
		Attributes struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Email  string `json:"email"`
			Source string `json:"source"`
			Status string `json:"status"`
			Roles  string `json:"roles"`
		} `json:"attributes"`
	} `json:"audit"`
}

type NexusWebhook struct {
	secret string
}

// NewNexusWebhook creates and returns a RegistryWebhook instance
func NewNexusWebhook(secret string) RegistryWebhook {
	hook := NexusWebhook{
		secret: secret,
	}
	return &hook
}

func (hook *NexusWebhook) New(secret string) (RegistryWebhook, error) {
	hook.secret = secret
	return hook, nil
}

func (hook *NexusWebhook) Parse(r *http.Request, events ...Event) (*WebhookEvent, error) {
	log.Debugf("nexus parse payload %v", r)
	defer func() {
		_, _ = io.Copy(ioutil.Discard, r.Body)
		_ = r.Body.Close()
	}()

	if len(events) == 0 {
		return nil, ErrEventNotSpecifiedToParse
	}

	if r.Method != http.MethodPost {
		return nil, ErrInvalidHTTPMethod
	}

	webhookID := r.Header.Get("X-Nexus-Webhook-Id")
	if webhookID == "" {
		return nil, ErrMissingNexusWebhookIDHeader
	}
	nexusEvent := Event(webhookID)

	var found bool
	for _, evt := range events {
		if evt == nexusEvent {
			found = true
			break
		}
	}
	// event not defined to be parsed
	if !found {
		return nil, ErrEventNotFound
	}

	payload, err := ioutil.ReadAll(r.Body)
	if err != nil || len(payload) == 0 {
		return nil, ErrParsingPayload
	}

	var pl RepositoryComponentPayload
	switch nexusEvent {
	case RepositoryComponentEvent:
		err = json.Unmarshal(payload, &pl)
		if err != nil {
			log.Errorf("Could not Unmarshal nexus payload %v", err)
		}
	case RepositoryAssetEvent:
	case RepositoryEvent:
	case AuditEvent:
	default:
		return nil, fmt.Errorf("expecting event %s, got %s", RepositoryComponentEvent, nexusEvent)
	}

	if len(hook.secret) > 0 {
		signature := r.Header.Get("X-Nexus-Webhook-Signature")
		if len(signature) == 0 {
			return nil, ErrMissingNexusWebhookSignature
		}
		mac := hmac.New(sha1.New, []byte(hook.secret))
		// We Unmarshal and then Marshal to mimic JSON.stringify(body) according to the example
		// here: https://help.sonatype.com/repomanager3/webhooks/working-with-hmac-payloads
		payloadMarshaled, _ := json.Marshal(pl)
		_, _ = mac.Write(payloadMarshaled)
		expectedMAC := hex.EncodeToString(mac.Sum(nil))

		log.Debugf("payload=%s, expected=%s, sig=%s", payload, expectedMAC, signature)
		if !hmac.Equal([]byte(signature[:]), []byte(expectedMAC)) {
			return nil, ErrHMACVerificationFailed
		}
	}

	webhookEvent := WebhookEvent{
		ImageName: pl.Component.Name,
		RepoName:  pl.RepositoryName,
		TagName:   pl.Component.Version,
	}

	return &webhookEvent, nil
}
