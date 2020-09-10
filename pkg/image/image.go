package image

import (
	"strings"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/tag"
)

type ContainerImage struct {
	RegistryURL           string
	ImageName             string
	ImageTag              *tag.ImageTag
	ImageAlias            string
	HelmParamImageName    string
	HelmParamImageVersion string
	original              string
}

type ContainerImageList []*ContainerImage

// NewFromIdentifier parses an image identifier and returns a populated ContainerImage
func NewFromIdentifier(identifier string) *ContainerImage {
	img := ContainerImage{}
	img.RegistryURL = getRegistryFromIdentifier(identifier)
	img.ImageAlias, img.ImageName, img.ImageTag = getImageTagFromIdentifier(identifier)
	img.original = identifier
	return &img
}

// String returns the string representation of given ContainerImage
func (img *ContainerImage) String() string {
	str := ""
	if img.ImageAlias != "" {
		str += img.ImageAlias
		str += "="
	}
	if img.RegistryURL != "" {
		str += img.RegistryURL + "/"
	}
	str += img.ImageName
	if img.ImageTag != nil {
		str += ":"
		str += img.ImageTag.TagName
	}
	return str
}

func (img *ContainerImage) GetFullNameWithoutTag() string {
	str := ""
	if img.RegistryURL != "" {
		str += img.RegistryURL + "/"
	}
	str += img.ImageName
	return str
}

func (img *ContainerImage) GetFullNameWithTag() string {
	str := ""
	if img.RegistryURL != "" {
		str += img.RegistryURL + "/"
	}
	str += img.ImageName
	if img.ImageTag != nil {
		str += ":"
		str += img.ImageTag.TagName
	}
	return str
}

func (img *ContainerImage) Original() string {
	return img.original
}

// IsUpdatable checks whether the given image can be updated with newTag while
// taking tagSpec into account. tagSpec must be given as a semver compatible
// version spec, i.e. ^1.0 or ~2.1
func (img *ContainerImage) IsUpdatable(newTag, tagSpec string) bool {
	return false
}

// WithTag returns a copy of img with new tag information set
func (img *ContainerImage) WithTag(newTag *tag.ImageTag) *ContainerImage {
	nimg := &ContainerImage{}
	nimg.RegistryURL = img.RegistryURL
	nimg.ImageName = img.ImageName
	nimg.ImageTag = newTag
	nimg.ImageAlias = img.ImageAlias
	nimg.HelmParamImageName = img.HelmParamImageName
	nimg.HelmParamImageVersion = img.HelmParamImageVersion
	return nimg
}

// ContainsImage checks whether img is contained in a list of images
func (list *ContainerImageList) ContainsImage(img *ContainerImage, checkVersion bool) *ContainerImage {
	for _, image := range *list {
		if img.ImageName == image.ImageName && image.RegistryURL == img.RegistryURL {
			if !checkVersion || image.ImageTag.TagName == img.ImageTag.TagName {
				return image
			}
		}
	}
	return nil
}

// String Returns the name of all images as a string, separated using comma
func (list *ContainerImageList) String() string {
	imgNameList := make([]string, 0)
	for _, image := range *list {
		imgNameList = append(imgNameList, image.String())
	}
	return strings.Join(imgNameList, ",")
}

// Gets the registry URL from an image identifier
func getRegistryFromIdentifier(identifier string) string {
	var imageString string
	comp := strings.Split(identifier, "=")
	if len(comp) > 1 {
		imageString = comp[1]
	} else {
		imageString = identifier
	}
	comp = strings.Split(imageString, "/")
	if len(comp) > 1 && strings.Contains(comp[0], ".") {
		return comp[0]
	} else {
		return ""
	}
}

// Gets the image name and tag from an image identifier
func getImageTagFromIdentifier(identifier string) (string, string, *tag.ImageTag) {
	var imageString string
	var sourceName string

	// The original name is prepended to the image name, separated by =
	comp := strings.SplitN(identifier, "=", 2)
	if len(comp) == 2 {
		sourceName = comp[0]
		imageString = comp[1]
	} else {
		imageString = identifier
	}

	// Strip any repository identifier from the string
	comp = strings.Split(imageString, "/")
	if len(comp) > 1 && strings.Contains(comp[0], ".") {
		imageString = strings.Join(comp[1:], "/")
	}

	comp = strings.SplitN(imageString, ":", 2)
	if len(comp) != 2 {
		return sourceName, imageString, nil
	} else {
		return sourceName, comp[0], tag.NewImageTag(comp[1], time.Unix(0, 0))
	}
}
