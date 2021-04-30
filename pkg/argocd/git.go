package argocd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"text/template"

	"github.com/argoproj-labs/argocd-image-updater/pkg/image"
	"sigs.k8s.io/kustomize/pkg/commands/kustfile"
	"sigs.k8s.io/kustomize/pkg/fs"
	image2 "sigs.k8s.io/kustomize/pkg/image"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/pkg/log"

	"github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
)

// templateCommitMessage renders a commit message template and returns it as
// as a string. If the template could not be rendered, returns a default
// message.
func TemplateCommitMessage(tpl *template.Template, appName string, changeList []ChangeEntry) string {
	var cmBuf bytes.Buffer

	type commitMessageChange struct {
		Image  string
		OldTag string
		NewTag string
	}

	type commitMessageTemplate struct {
		AppName    string
		AppChanges []commitMessageChange
	}

	// We need to transform the change list into something more viable for the
	// writer of a template.
	changes := make([]commitMessageChange, 0)
	for _, c := range changeList {
		changes = append(changes, commitMessageChange{c.Image.ImageName, c.OldTag.String(), c.NewTag.String()})
	}

	tplData := commitMessageTemplate{
		AppName:    appName,
		AppChanges: changes,
	}
	err := tpl.Execute(&cmBuf, tplData)
	if err != nil {
		log.Errorf("could not execute template for Git commit message: %v", err)
		return "build: update of application " + appName
	}

	return cmBuf.String()
}

type changeWriter func(app *v1alpha1.Application, wbc *WriteBackConfig, gitC git.Client) (err error, skip bool)

// commitChanges commits any changes required for updating one or more images
// after the UpdateApplication cycle has finished.
func commitChangesGit(app *v1alpha1.Application, wbc *WriteBackConfig, write changeWriter) error {
	creds, err := wbc.GetCreds(app)
	if err != nil {
		return fmt.Errorf("could not get creds for repo '%s': %v", app.Spec.Source.RepoURL, err)
	}
	var gitC git.Client
	if wbc.GitClient == nil {
		tempRoot, err := ioutil.TempDir(os.TempDir(), fmt.Sprintf("git-%s", app.Name))
		if err != nil {
			return err
		}
		defer func() {
			err := os.RemoveAll(tempRoot)
			if err != nil {
				log.Errorf("could not remove temp dir: %v", err)
			}
		}()
		gitC, err = git.NewClientExt(app.Spec.Source.RepoURL, tempRoot, creds, false, false)
		if err != nil {
			return err
		}
	} else {
		gitC = wbc.GitClient
	}
	err = gitC.Init()
	if err != nil {
		return err
	}
	err = gitC.Fetch()
	if err != nil {
		return err
	}

	// Set username and e-mail address used to identify the commiter
	if wbc.GitCommitUser != "" && wbc.GitCommitEmail != "" {
		err = gitC.Config(wbc.GitCommitUser, wbc.GitCommitEmail)
		if err != nil {
			return err
		}
	}

	// The branch to checkout is either a configured branch in the write-back
	// config, or taken from the application spec's targetRevision. If the
	// target revision is set to the special value HEAD, or is the empty
	// string, we'll try to resolve it to a branch name.
	checkOutBranch := app.Spec.Source.TargetRevision
	if wbc.GitBranch != "" {
		checkOutBranch = wbc.GitBranch
	}
	log.Tracef("targetRevision for update is '%s'", checkOutBranch)
	if checkOutBranch == "" || checkOutBranch == "HEAD" {
		checkOutBranch, err = gitC.SymRefToBranch(checkOutBranch)
		log.Infof("resolved remote default branch to '%s' and using that for operations", checkOutBranch)
		if err != nil {
			return err
		}
	}

	err = gitC.Checkout(checkOutBranch)
	if err != nil {
		return err
	}

	if err, skip := write(app, wbc, gitC); err != nil {
		return err
	} else if skip {
		return nil
	}

	commitOpts := &git.CommitOptions{}
	if wbc.GitCommitMessage != "" {
		cm, err := ioutil.TempFile("", "image-updater-commit-msg")
		if err != nil {
			return fmt.Errorf("cold not create temp file: %v", err)
		}
		log.Debugf("Writing commit message to %s", cm.Name())
		err = ioutil.WriteFile(cm.Name(), []byte(wbc.GitCommitMessage), 0600)
		if err != nil {
			_ = cm.Close()
			return fmt.Errorf("could not write commit message to %s: %v", cm.Name(), err)
		}
		commitOpts.CommitMessagePath = cm.Name()
		_ = cm.Close()
		defer os.Remove(cm.Name())
	}

	err = gitC.Commit("", commitOpts)
	if err != nil {
		return err
	}
	err = gitC.Push("origin", checkOutBranch, false)
	if err != nil {
		return err
	}

	return nil
}

func writeOverrides(app *v1alpha1.Application, _ *WriteBackConfig, gitC git.Client) (err error, skip bool) {
	targetExists := true
	targetFile := path.Join(gitC.Root(), app.Spec.Source.Path, fmt.Sprintf(".argocd-source-%s.yaml", app.Name))
	_, err = os.Stat(targetFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return
		} else {
			targetExists = false
		}
	}

	override, err := marshalParamsOverride(app)
	if err != nil {
		return
	}

	// If the target file already exist in the repository, we will check whether
	// our generated new file is the same as the existing one, and if yes, we
	// don't proceed further for commit.
	if targetExists {
		data, err := ioutil.ReadFile(targetFile)
		if err != nil {
			return err, false
		}
		if string(data) == string(override) {
			log.Debugf("target parameter file and marshaled data are the same, skipping commit.")
			return nil, true
		}
	}

	err = ioutil.WriteFile(targetFile, override, 0600)
	if err != nil {
		return
	}

	if !targetExists {
		err = gitC.Add(targetFile)
	}
	return
}

var _ changeWriter = writeOverrides

// writeKustomization writes any changes required for updating one or more images to a kustomization.yml
func writeKustomization(app *v1alpha1.Application, wbc *WriteBackConfig, gitC git.Client) (err error, skip bool) {
	if oldDir, err := os.Getwd(); err != nil {
		return err, false
	} else {
		defer func() {
			_ = os.Chdir(oldDir)
		}()
	}

	base := filepath.Join(gitC.Root(), wbc.KustomizeBase)
	if err := os.Chdir(base); err != nil {
		return err, false
	}

	log.Infof("updating base %s", base)

	kf, err := kustfile.NewKustomizationFile(fs.MakeRealFS())
	if err != nil {
		return
	}
	kustomization, err := kf.Read()
	if err != nil {
		return
	}

Images:
	for _, img := range app.Spec.Source.Kustomize.Images {
		override := parseImageOverride(img)
		for i, imgSet := range kustomization.Images {
			if imgSet.Name == override.Name {
				kustomization.Images[i] = override
				continue Images
			}
		}
		// wasn't an existing override, add one
		kustomization.Images = append(kustomization.Images, override)
	}

	if err := kf.Write(kustomization); err != nil {
		return err, false
	}

	return
}

func parseImageOverride(str v1alpha1.KustomizeImage) image2.Image {
	// TODO is this a valid use? format could diverge
	img := image.NewFromIdentifier(string(str))
	tagName := ""
	tagDigest := ""
	if img.ImageTag != nil {
		tagName = img.ImageTag.TagName
		tagDigest = img.ImageTag.TagDigest
	}
	if img.RegistryURL != "" {
		// NewFromIdentifier strips off the registry
		img.ImageName = img.RegistryURL + "/" + img.ImageName
	}
	if img.ImageAlias == "" {
		img.ImageAlias = img.ImageName
		img.ImageName = "" // inside baseball (see return): name isn't changing, just tag, so don't write newName
	}
	return image2.Image{
		Name:    img.ImageAlias,
		NewName: img.ImageName,
		NewTag:  tagName,
		Digest:  tagDigest,
	}
}

var _ changeWriter = writeKustomization
