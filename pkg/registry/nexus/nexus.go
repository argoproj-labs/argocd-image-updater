package nexus

// this package receives the Nexus webhook
// https://help.sonatype.com/repomanager3/webhooks
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
	ErrParsingPayload               = errors.New("error parsing payload")
)

// Event defines a Nexus hook event type
type Event string

// Nexus hook event types
const (
	RepositoryComponentEvent Event = "rm:repository:component"
	RepositoryAssetEvent     Event = "rm:repository:asset"
	RepositoryEvent          Event = "rm:repository"
	AuditEvent               Event = "rm:audit" // TODO: double check this
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

// Option is a configuration option for the webhook
type Option func(*Webhook) error

// Options is a namespace var for configuration options
var Options = WebhookOptions{}

// WebhookOptions is a namespace for configuration option methods
type WebhookOptions struct{}

// Secret registers the Nexus secret
func (WebhookOptions) Secret(secret string) Option {
	return func(hook *Webhook) error {
		hook.secret = secret
		return nil
	}
}

// Webhook instance contains all methods needed to process events
type Webhook struct {
	secret string
}

// New creates and returns a WebHook instance denoted by the Provider type
func New(options ...Option) (*Webhook, error) {
	hook := new(Webhook)
	for _, opt := range options {
		if err := opt(hook); err != nil {
			return nil, errors.New("Error applying Option")
		}
	}
	return hook, nil
}

// Parse verifies and parses the events specified and returns the payload object or an error
func (hook Webhook) Parse(r *http.Request, events ...Event) (interface{}, error) {
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

	event := r.Header.Get("X-Nexus-Webhook-Id")
	if event == "" {
		return nil, ErrMissingNexusWebhookIDHeader
	}
	nexusEvent := Event(event)

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

	var pl interface{}
	switch nexusEvent {
	case RepositoryComponentEvent:
		pl = pl.(RepositoryComponentPayload)
		err = json.Unmarshal(payload, &pl)
		break
	case RepositoryAssetEvent:
		pl = pl.(RepositoryAssetPayload)
		err = json.Unmarshal(payload, &pl)
		break
	case RepositoryEvent:
		pl = pl.(RepositoryPayload)
		err = json.Unmarshal(payload, &pl)
		break
	case AuditEvent:
		pl = pl.(AuditPayload)
		err = json.Unmarshal(payload, &pl)
		break
	default:
		return nil, fmt.Errorf("unknown event %s", nexusEvent)
	}

	// TODO: should we also reject all request with component.format other than docker?
	// If we have a Secret set, we should check the MAC
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

	return pl, nil
}
