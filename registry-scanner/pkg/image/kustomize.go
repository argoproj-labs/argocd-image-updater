package image

import (
	"strings"
)

// Shamelessly ripped from ArgoCD CLI code

type KustomizeImage string

func (i KustomizeImage) delim() string {
	for _, d := range []string{"=", ":", "@"} {
		if strings.Contains(string(i), d) {
			return d
		}
	}
	return ":"
}

// if the image name matches (i.e. up to the first delimiter)
func (i KustomizeImage) Match(j KustomizeImage) bool {
	delim := j.delim()
	imageName, _, _ := strings.Cut(string(i), delim)
	otherImageName, _, _ := strings.Cut(string(j), delim)
	return imageName == otherImageName
}

type KustomizeImages []KustomizeImage

// find the image or -1
func (images KustomizeImages) Find(image KustomizeImage) int {
	for i, a := range images {
		if a.Match(image) {
			return i
		}
	}
	return -1
}
