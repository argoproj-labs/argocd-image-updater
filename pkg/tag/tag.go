package tag

import (
	"sort"
	"sync"
	"time"
)

// ImageTag is a representation of an image tag with metadata
// Use NewImageTag to to initialize a new object.
type ImageTag struct {
	TagName string
	TagDate *time.Time
}

// ImageTagList is a collection of ImageTag objects.
// Use NewImageTagList to to initialize a new object.
type ImageTagList struct {
	items map[string]*ImageTag
	lock  *sync.RWMutex
}

// SortableImageTagList is just that - a sortable list of ImageTag entries
type SortableImageTagList []*ImageTag

// Len returns the length of an SortableImageList
func (il SortableImageTagList) Len() int {
	return len(il)
}

// Swap swaps two entries in the SortableImageList
func (il SortableImageTagList) Swap(i, j int) {
	il[i], il[j] = il[j], il[i]
}

// NewImageTag initializes an ImageTag object and returns it
func NewImageTag(tagName string, tagDate time.Time) *ImageTag {
	tag := &ImageTag{}
	tag.TagName = tagName
	tag.TagDate = &tagDate
	return tag
}

// NewImageTagList initializes an ImageTagList object and returns it
func NewImageTagList() *ImageTagList {
	itl := ImageTagList{}
	itl.items = make(map[string]*ImageTag)
	itl.lock = &sync.RWMutex{}
	return &itl
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

// String returns the tag name of the ImageTag
func (tag *ImageTag) String() string {
	return tag.TagName
}

// Checks whether given tag is contained in tag list in O(n) time
func (il ImageTagList) Contains(tag *ImageTag) bool {
	il.lock.RLock()
	defer il.lock.RUnlock()
	return il.unlockedContains(tag)
}

// Add adds an ImageTag to an ImageTagList, ensuring this will not result in
// an double entry
func (il ImageTagList) Add(tag *ImageTag) {
	il.lock.Lock()
	defer il.lock.Unlock()
	il.items[tag.TagName] = tag
}

// SortByName returns an array of ImageTag objects, sorted by the tag's name
func (il ImageTagList) SortByName() SortableImageTagList {
	sil := SortableImageTagList{}
	for _, v := range il.items {
		sil = append(sil, v)
	}
	sort.Slice(sil, func(i, j int) bool {
		return sil[i].TagName < sil[j].TagName
	})
	return sil
}

// Should only be used in a method that holds a lock on the ImageTagList
func (il ImageTagList) unlockedContains(tag *ImageTag) bool {
	if _, ok := il.items[tag.TagName]; ok {
		return true
	}
	return false
}
