package cache

import (
	"fmt"

	"github.com/argoproj-labs/argocd-image-updater/pkg/tag"

	memcache "github.com/patrickmn/go-cache"
)

type MemCache struct {
	cache *memcache.Cache
}

// NewMemCache returns a new instance of MemCache
func NewMemCache() ImageTagCache {
	mc := MemCache{}
	c := memcache.New(0, 0)
	mc.cache = c
	return &mc
}

// HasTag returns true if cache has entry for given tag, false if not
func (mc *MemCache) HasTag(imageName string, tagName string) bool {
	tag, err := mc.GetTag(imageName, tagName)
	if err != nil || tag == nil {
		return false
	} else {
		return true
	}
}

func (mc *MemCache) SetTag(imageName string, imgTag *tag.ImageTag) {
	mc.cache.Set(cacheKey(imageName, imgTag.TagName), *imgTag, -1)
}

func (mc *MemCache) GetTag(imageName string, tagName string) (*tag.ImageTag, error) {
	var imgTag tag.ImageTag
	e, ok := mc.cache.Get(cacheKey(imageName, tagName))
	if !ok {
		return nil, nil
	}
	imgTag, ok = e.(tag.ImageTag)
	if !ok {
		return nil, fmt.Errorf("")
	}
	return &imgTag, nil
}

func cacheKey(imageName, imageTag string) string {
	return fmt.Sprintf("%s:%s", imageName, imageTag)
}
