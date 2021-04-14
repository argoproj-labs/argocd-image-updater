package argocd

import (
	"testing"
	"text/template"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/pkg/tag"

	"github.com/stretchr/testify/assert"
)

func Test_TemplateCommitMessage(t *testing.T) {
	t.Run("Template default commit message", func(t *testing.T) {
		exp := `build: automatic update of foobar

updates image foo/bar tag '1.0' to '1.1'
updates image bar/baz tag '2.0' to '2.1'
`
		tpl := template.Must(template.New("sometemplate").Parse(common.DefaultGitCommitMessage))
		cl := []ChangeEntry{
			{
				Image:  image.NewFromIdentifier("foo/bar"),
				OldTag: tag.NewImageTag("1.0", time.Now(), ""),
				NewTag: tag.NewImageTag("1.1", time.Now(), ""),
			},
			{
				Image:  image.NewFromIdentifier("bar/baz"),
				OldTag: tag.NewImageTag("2.0", time.Now(), ""),
				NewTag: tag.NewImageTag("2.1", time.Now(), ""),
			},
		}
		r := TemplateCommitMessage(tpl, "foobar", cl)
		assert.NotEmpty(t, r)
		assert.Equal(t, exp, r)
	})
}
