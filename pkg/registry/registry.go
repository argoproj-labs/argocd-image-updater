package registry

import (
	"fmt"
	"strings"

	"github.com/argoproj-labs/argocd-image-updater/pkg/client"
	"github.com/argoproj-labs/argocd-image-updater/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/pkg/log"

	"github.com/nokia/docker-registry-client/registry"
)

// GetTags returns a list of available tags for the given image
func (clientInfo *RegistryEndpoint) GetTags(img *image.ContainerImage, kubeClient *client.KubernetesClient) ([]string, error) {
	err := clientInfo.setEndpointCredentials(kubeClient)
	if err != nil {
		return nil, err
	}
	client, err := registry.NewCustom(clientInfo.RegistryAPI, registry.Options{
		DoInitialPing: clientInfo.Ping,
		Logf:          registry.Quiet,
		Username:      clientInfo.Username,
		Password:      clientInfo.Password,
	})
	if err != nil {
		return nil, err
	}

	// DockerHub has a special namespace 'library', that is hidden from the user
	var nameInRegistry string
	if len := len(strings.Split(img.ImageName, "/")); len == 1 {
		nameInRegistry = "library/" + img.ImageName
	} else {
		nameInRegistry = img.ImageName
	}
	tags, err := client.Tags(nameInRegistry)
	if err != nil {
		return nil, err
	}
	return tags, err
}

func (clientInfo *RegistryEndpoint) setEndpointCredentials(kubeClient *client.KubernetesClient) error {
	if clientInfo.Username == "" && clientInfo.Password == "" && clientInfo.Credentials != "" {
		credSrc, err := image.ParseCredentialSource(clientInfo.Credentials, false)
		if err != nil {
			return err
		}

		// For fetching credentials, we must have working Kubernetes client.
		if (credSrc.Type == image.CredentialSourcePullSecret || credSrc.Type == image.CredentialSourceSecret) && kubeClient == nil {
			log.WithContext().
				AddField("registry", clientInfo.RegistryAPI).
				Warnf("cannot user K8s credentials without Kubernetes client")
			return fmt.Errorf("could not fetch image tags")
		}

		creds, err := credSrc.FetchCredentials(clientInfo.RegistryAPI, kubeClient)
		if err != nil {
			return err
		}

		clientInfo.Username = creds.Username
		clientInfo.Password = creds.Password
	}

	return nil
}
