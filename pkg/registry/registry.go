package registry

// Package registry implements functions for retrieving data from container
// registries.
//
// TODO: Refactor this package and provide mocks for better testing.

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/client"
	"github.com/argoproj-labs/argocd-image-updater/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/pkg/tag"
)

// GetTags returns a list of available tags for the given image
func (endpoint *RegistryEndpoint) GetTags(img *image.ContainerImage, regClient RegistryClient, vc *image.VersionConstraint) (*tag.ImageTagList, error) {
	var tagList *tag.ImageTagList = tag.NewImageTagList()
	var imgTag *tag.ImageTag
	var err error

	// DockerHub has a special namespace 'library', that is hidden from the user
	// FIXME: How do other registries handle this?
	var nameInRegistry string
	if len := len(strings.Split(img.ImageName, "/")); len == 1 {
		nameInRegistry = "library/" + img.ImageName
	} else {
		nameInRegistry = img.ImageName
	}
	tTags, err := regClient.Tags(nameInRegistry)
	if err != nil {
		return nil, err
	}

	tags := []string{}

	// Loop through tags, removing those we do not want
	if vc.MatchFunc != nil {
		for _, t := range tTags {
			if !vc.MatchFunc(t, vc.MatchArgs) {
				log.Tracef("Removing tag %s because of tag match options", t)
			} else {
				tags = append(tags, t)
			}
		}
	}

	// If we don't want to fetch meta data, just build the taglist and return it
	// with dummy meta data.
	if vc.SortMode != image.VersionSortLatest {
		for _, tagStr := range tags {
			imgTag = tag.NewImageTag(tagStr, time.Unix(0, 0))
			tagList.Add(imgTag)
		}
		return tagList, nil
	}

	// Fetch the manifest for the tag -- we need v1, because it contains history
	// information that we require.
	for _, tagStr := range tags {

		// Look into the cache first and re-use any found item. If GetTag() returns
		// an error, we treat it as a cache miss and just go ahead to invalidate
		// the entry.
		imgTag, err = endpoint.Cache.GetTag(nameInRegistry, tagStr)
		if err != nil {
			log.Warnf("invalid entry for %s:%s in cache, invalidating.", nameInRegistry, imgTag.TagName)
		} else if imgTag != nil {
			log.Debugf("Cache hit for %s:%s", nameInRegistry, imgTag.TagName)
			tagList.Add(imgTag)
			continue
		}

		ml, err := regClient.ManifestV1(nameInRegistry, tagStr)
		if err != nil {
			return nil, err
		}

		if len(ml.History) < 1 {
			log.Warnf("Could not get creation date for %s: History information missing", img.GetFullNameWithTag())
			continue
		}

		var histInfo map[string]interface{}
		err = json.Unmarshal([]byte(ml.History[0].V1Compatibility), &histInfo)
		if err != nil {
			log.Warnf("Could not unmarshal history info for %s: %v", img.GetFullNameWithTag(), err)
			continue
		}

		crIf, ok := histInfo["created"]
		if !ok {
			log.Warnf("Incomplete history information for %s: no creation timestamp found", img.GetFullNameWithTag())
			continue
		}

		crStr, ok := crIf.(string)
		if !ok {
			log.Warnf("Creation timestamp for %s has wrong type - need string, is %T", img.GetFullNameWithTag(), crIf)
			continue
		}

		// Creation date is stored as RFC3339 timestamp with nanoseconds, i.e. like
		// this: 2017-12-01T23:06:12.607835588Z
		log.Tracef("Found origin creation date for %s: %s", tagStr, crStr)
		crDate, err := time.Parse(time.RFC3339Nano, crStr)
		if err != nil {
			log.Warnf("Could not parse creation timestamp for %s (%s): %v", img.GetFullNameWithTag(), crStr, err)
			continue
		}
		imgTag = tag.NewImageTag(tagStr, crDate)
		tagList.Add(imgTag)
		endpoint.Cache.SetTag(nameInRegistry, imgTag)
	}

	return tagList, err
}

// Sets endpoint credentials for this registry from a reference to a K8s secret
func (clientInfo *RegistryEndpoint) SetEndpointCredentials(kubeClient *client.KubernetesClient) error {
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
