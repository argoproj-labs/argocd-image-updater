package registry

// Package registry implements functions for retrieving data from container
// registries.
//
// TODO: Refactor this package and provide mocks for better testing.

import (
	"fmt"
	"strings"
	"time"

	"github.com/docker/distribution"

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

	// Some registries have a default namespace that is used when the image name
	// doesn't specify one. For example at Docker Hub, this is 'library'.
	var nameInRegistry string
	if len := len(strings.Split(img.ImageName, "/")); len == 1 && endpoint.DefaultNS != "" {
		nameInRegistry = endpoint.DefaultNS + "/" + img.ImageName
		log.Debugf("Using canonical image name '%s' for image '%s'", nameInRegistry, img.ImageName)
	} else {
		nameInRegistry = img.ImageName
	}
	tTags, err := regClient.Tags(nameInRegistry)
	if err != nil {
		return nil, err
	}

	tags := []string{}

	// Loop through tags, removing those we do not want
	if vc.MatchFunc != nil || len(vc.IgnoreList) > 0 {
		for _, t := range tTags {
			if (vc.MatchFunc != nil && !vc.MatchFunc(t, vc.MatchArgs)) || vc.IsTagIgnored(t) {
				log.Tracef("Removing tag %s because it either didn't match defined pattern or is ignored", t)
			} else {
				tags = append(tags, t)
			}
		}
	} else {
		tags = tTags
	}

	// In some cases, we don't need to fetch the metadata to get the creation time
	// stamp of from the image's meta data:
	//
	// - We do not have sort mode == latest
	// - The registry doesn't provide meta data and has tags sorted already
	//
	// We just create a dummy time stamp according to the registry's sort mode, if
	// set.
	if vc.SortMode != image.VersionSortLatest || endpoint.TagListSort.IsTimeSorted() {
		for i, tagStr := range tags {
			var ts int
			if endpoint.TagListSort == SortLatestFirst {
				ts = len(tags) - i
			} else if endpoint.TagListSort == SortLatestLast {
				ts = i
			}
			imgTag = tag.NewImageTag(tagStr, time.Unix(int64(ts), 0))
			tagList.Add(imgTag)
		}
		return tagList, nil
	}

	// Fetch the manifest for the tag -- we need v1, because it contains history
	// information that we require.
	i := 0
	for _, tagStr := range tags {
		i += 1
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

		log.Tracef("Getting manifest for image %s:%s (operation %d/%d)", nameInRegistry, tagStr, i, len(tags))

		var ml distribution.Manifest
		var err error

		// We first try to fetch a V2 manifest, and if that's not available we fall
		// back to fetching V1 manifest. If that fails also, we just skip this tag.
		if ml, err = regClient.ManifestV2(nameInRegistry, tagStr); err != nil {
			log.Debugf("No V2 manifest for %s:%s, fetching V1 (%v)", nameInRegistry, tagStr, err)
			if ml, err = regClient.ManifestV1(nameInRegistry, tagStr); err != nil {
				log.Errorf("Error fetching metadata for %s:%s - neither V1 or V2 manifest returned by registry: %v", nameInRegistry, tagStr, err)
				continue
			}
		}

		// Parse required meta data from the manifest. The metadata contains all
		// information needed to decide whether to consider this tag or not.
		ti, err := regClient.TagMetadata(nameInRegistry, ml)
		if err != nil {
			return nil, err
		}
		if ti == nil {
			log.Debugf("No metadata found for %s:%s", nameInRegistry, tagStr)
			continue
		}

		log.Tracef("Found date %s", ti.CreatedAt.String())

		imgTag = tag.NewImageTag(tagStr, ti.CreatedAt)
		tagList.Add(imgTag)
		endpoint.Cache.SetTag(nameInRegistry, imgTag)
	}

	return tagList, err
}

// Sets endpoint credentials for this registry from a reference to a K8s secret
func (ep *RegistryEndpoint) SetEndpointCredentials(kubeClient *client.KubernetesClient) error {
	if ep.Username == "" && ep.Password == "" && ep.Credentials != "" {
		credSrc, err := image.ParseCredentialSource(ep.Credentials, false)
		if err != nil {
			return err
		}

		// For fetching credentials, we must have working Kubernetes client.
		if (credSrc.Type == image.CredentialSourcePullSecret || credSrc.Type == image.CredentialSourceSecret) && kubeClient == nil {
			log.WithContext().
				AddField("registry", ep.RegistryAPI).
				Warnf("cannot user K8s credentials without Kubernetes client")
			return fmt.Errorf("could not fetch image tags")
		}

		creds, err := credSrc.FetchCredentials(ep.RegistryAPI, kubeClient)
		if err != nil {
			return err
		}

		ep.Username = creds.Username
		ep.Password = creds.Password
	}

	return nil
}
