package tag

import (
	"context"
	"encoding/hex"
	"sort"
	"sync"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"

	"github.com/Masterminds/semver/v3"
)

// ImageTag is a representation of an image tag with metadata
// Use NewImageTag to initialize a new object.
type ImageTag struct {
	TagName   string
	TagDate   *time.Time
	TagDigest string
	Labels    map[string]string
}

// ImageTagList is a collection of ImageTag objects.
// Use NewImageTagList to initialize a new object.
type ImageTagList struct {
	items map[string]*ImageTag
	lock  *sync.RWMutex
}

// TagInfo contains information for a tag
type TagInfo struct {
	CreatedAt time.Time
	Digest    [32]byte
	Labels    map[string]string
}

// SortableImageTagList is just that - a sortable list of ImageTag entries
type SortableImageTagList []*ImageTag

// NewImageTag initializes an ImageTag object and returns it
func NewImageTag(tagName string, tagDate time.Time, tagDigest string) *ImageTag {
	tag := &ImageTag{}
	tag.TagName = tagName
	tag.TagDate = &tagDate
	tag.TagDigest = tagDigest
	tag.Labels = make(map[string]string)
	return tag
}

// NewImageTagWithLabels initializes an ImageTag object with labels and returns it
func NewImageTagWithLabels(tagName string, tagDate time.Time, tagDigest string, labels map[string]string) *ImageTag {
	tag := &ImageTag{}
	tag.TagName = tagName
	tag.TagDate = &tagDate
	tag.TagDigest = tagDigest
	if labels != nil {
		tag.Labels = labels
	} else {
		tag.Labels = make(map[string]string)
	}
	return tag
}

// NewImageTagList initializes an ImageTagList object and returns it
func NewImageTagList() *ImageTagList {
	itl := ImageTagList{}
	itl.items = make(map[string]*ImageTag)
	itl.lock = &sync.RWMutex{}
	return &itl
}

// Len returns the length of an SortableImageList
func (sitl SortableImageTagList) Len() int {
	return len(sitl)
}

// Swap swaps two entries in the SortableImageList
func (sitl SortableImageTagList) Swap(i, j int) {
	sitl[i], sitl[j] = sitl[j], sitl[i]
}

// Tags returns a list of verbatim tag names as string slice
func (sitl SortableImageTagList) Tags() []string {
	tagList := []string{}
	for _, t := range sitl {
		tagList = append(tagList, t.TagName)
	}
	return tagList
}

// String returns the tag name of the ImageTag, possibly with a digest appended
// to its name.
func (tag *ImageTag) String() string {
	if tag.TagDigest != "" {
		return tag.TagDigest
	} else {
		return tag.TagName
	}
}

// IsDigest returns true if the tag has a digest
func (tag *ImageTag) IsDigest() bool {
	return tag.TagDigest != ""
}

// Equals checks whether two tags are equal. Will consider any digest set for
// either tag with precedence, otherwise uses the tag names.
func (tag *ImageTag) Equals(aTag *ImageTag) bool {
	// If either tag has a digest, compare by digest
	if tag.IsDigest() || aTag.IsDigest() {
		return tag.TagDigest == aTag.TagDigest
	}
	// Otherwise compare by tag name
	return tag.TagName == aTag.TagName
}

// Tags returns a list of verbatim tag names as string slice
func (il *ImageTagList) Tags() []string {
	il.lock.RLock()
	defer il.lock.RUnlock()
	tagList := []string{}
	for k := range il.items {
		tagList = append(tagList, k)
	}
	return tagList
}

// Contains checks whether given tag is contained in tag list in O(n) time
func (il *ImageTagList) Contains(tag *ImageTag) bool {
	il.lock.RLock()
	defer il.lock.RUnlock()
	return il.unlockedContains(tag)
}

// Add adds an ImageTag to an ImageTagList, ensuring this will not result in
// a double entry
func (il *ImageTagList) Add(tag *ImageTag) {
	il.lock.Lock()
	defer il.lock.Unlock()
	il.items[tag.TagName] = tag
}

// SortAlphabetically returns an array of ImageTag objects, sorted by the tag's name
func (il *ImageTagList) SortAlphabetically() SortableImageTagList {
	sil := make(SortableImageTagList, 0, len(il.items))
	for _, v := range il.items {
		sil = append(sil, v)
	}
	sort.Slice(sil, func(i, j int) bool {
		return sil[i].TagName < sil[j].TagName
	})
	return sil
}

// SortByDate returns a SortableImageTagList, sorted by the tag's date
func (il *ImageTagList) SortByDate() SortableImageTagList {
	sil := make(SortableImageTagList, 0, len(il.items))
	for _, v := range il.items {
		sil = append(sil, v)
	}
	sort.Slice(sil, func(i, j int) bool {
		if sil[i].TagDate.Equal(*sil[j].TagDate) {
			// if an image has two tags, return the same consistently
			return sil[i].TagName < sil[j].TagName
		}
		return sil[i].TagDate.Before(*sil[j].TagDate)
	})
	return sil
}

func (il *ImageTagList) SortBySemVer(ctx context.Context) SortableImageTagList {
	log := log.LoggerFromContext(ctx)
	// We need a read lock, because we access the items hash after sorting
	il.lock.RLock()
	defer il.lock.RUnlock()

	sil := SortableImageTagList{}
	svl := make([]*semver.Version, 0)
	for _, v := range il.items {
		svi, err := semver.NewVersion(v.TagName)
		if err != nil {
			log.Debugf("could not parse input tag %s as semver: %v", v.TagName, err)
			continue
		}
		svl = append(svl, svi)
	}
	sort.Sort(semverCollection(svl))
	for _, svi := range svl {
		originalTag := il.items[svi.Original()]
		sil = append(sil, NewImageTagWithLabels(svi.Original(), *originalTag.TagDate, originalTag.TagDigest, originalTag.Labels))
	}
	return sil
}

// Should only be used in a method that holds a lock on the ImageTagList
func (il *ImageTagList) unlockedContains(tag *ImageTag) bool {
	if _, ok := il.items[tag.TagName]; ok {
		return true
	}
	return false
}

func (ti *TagInfo) EncodedDigest() string {
	return "sha256:" + hex.EncodeToString(ti.Digest[:])
}
