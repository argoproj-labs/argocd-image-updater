package argocd

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"text/template"

	"sigs.k8s.io/kustomize/api/konfig"
	"sigs.k8s.io/kustomize/api/types"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/argoproj-labs/argocd-image-updater/pkg/image"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/pkg/log"

	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
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

// TemplateBranchName parses a string to a template, and returns a
// branch name from that new template. If a branch name can not be
// rendered, it returns an empty value.
func TemplateBranchName(branchName string, changeList []ChangeEntry) string {
	var cmBuf bytes.Buffer

	tpl, err1 := template.New("branchName").Parse(branchName)

	if err1 != nil {
		log.Errorf("could not create template for Git branch name: %v", err1)
		return ""
	}

	type imageChange struct {
		Name   string
		Alias  string
		OldTag string
		NewTag string
	}

	type branchNameTemplate struct {
		Images []imageChange
		SHA1   string
	}

	// Let's add a unique hash to the template
	hasher := sha1.New()

	// We need to transform the change list into something more viable for the
	// writer of a template.
	changes := make([]imageChange, 0)
	for _, c := range changeList {
		changes = append(changes, imageChange{c.Image.ImageName, c.Image.ImageAlias, c.OldTag.String(), c.NewTag.String()})
		id := fmt.Sprintf("%v-%v-%v,", c.Image.ImageName, c.OldTag.String(), c.NewTag.String())
		_, hasherErr := hasher.Write([]byte(id))
		if hasherErr != nil {
			log.Errorf("could not write image string to hasher: %v", hasherErr)
			return ""
		}
	}

	tplData := branchNameTemplate{
		Images: changes,
		SHA1:   hex.EncodeToString(hasher.Sum(nil)),
	}

	err2 := tpl.Execute(&cmBuf, tplData)
	if err2 != nil {
		log.Errorf("could not execute template for Git branch name: %v", err2)
		return ""
	}

	toReturn := cmBuf.String()

	if len(toReturn) > 255 {
		return toReturn[:255]
	} else {
		return toReturn
	}
}

type changeWriter func(app *v1alpha1.Application, wbc *WriteBackConfig, gitC git.Client) (err error, skip bool)

// commitChanges commits any changes required for updating one or more images
// after the UpdateApplication cycle has finished.
func commitChangesGit(app *v1alpha1.Application, wbc *WriteBackConfig, changeList []ChangeEntry, write changeWriter) error {
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
		gitC, err = git.NewClientExt(app.Spec.Source.RepoURL, tempRoot, creds, false, false, "")
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
	err = gitC.Fetch("")
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

	// The push branch is by default the same as the checkout branch, unless
	// specified with the git-write-branch annotation, in which case a new
	// branch will be made following a template that can use the list of
	// changed images.
	pushBranch := checkOutBranch

	if wbc.GitWriteBranch != "" {
		log.Debugf("Using branch template: %s", wbc.GitWriteBranch)
		pushBranch = TemplateBranchName(wbc.GitWriteBranch, changeList)
		if pushBranch != "" {
			log.Debugf("Creating branch '%s' and using that for push operations", pushBranch)
			err = gitC.Branch(checkOutBranch, pushBranch)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("Git branch name could not be created from the template: %s", wbc.GitWriteBranch)
		}
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
	err = gitC.Push("origin", pushBranch, false)
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

	kustFile := findKustomization(base)
	if kustFile == "" {
		return fmt.Errorf("could not find kustomization in %s", base), false
	}

	filterFunc, err := imagesFilter(app.Spec.Source.Kustomize.Images)
	if err != nil {
		return err, false
	}
	err = kyaml.UpdateFile(filterFunc, kustFile)
	if err != nil {
		return err, false
	}

	return nil, false
}

func imagesFilter(images v1alpha1.KustomizeImages) (kyaml.Filter, error) {
	var overrides []kyaml.Filter
	for _, img := range images {
		override, err := imageFilter(parseImageOverride(img))
		if err != nil {
			return nil, err
		}
		overrides = append(overrides, override)
	}

	return kyaml.FilterFunc(func(object *kyaml.RNode) (*kyaml.RNode, error) {
		err := object.PipeE(append([]kyaml.Filter{kyaml.LookupCreate(
			kyaml.SequenceNode, "images",
		)}, overrides...)...)
		return object, err
	}), nil
}

func imageFilter(imgSet types.Image) (kyaml.Filter, error) {
	data, err := kyaml.Marshal(imgSet)
	if err != nil {
		return nil, err
	}
	update, err := kyaml.Parse(string(data))
	if err != nil {
		return nil, err
	}
	setter := kyaml.ElementSetter{
		Element: update.YNode(),
		Keys:    []string{"name"},
		Values:  []string{imgSet.Name},
	}
	return kyaml.FilterFunc(func(object *kyaml.RNode) (*kyaml.RNode, error) {
		return object, object.PipeE(setter)
	}), nil
}

func findKustomization(base string) string {
	for _, f := range konfig.RecognizedKustomizationFileNames() {
		kustFile := path.Join(base, f)
		if stat, err := os.Stat(kustFile); err == nil && !stat.IsDir() {
			return kustFile
		}
	}
	return ""
}

func parseImageOverride(str v1alpha1.KustomizeImage) types.Image {
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
	return types.Image{
		Name:    img.ImageAlias,
		NewName: img.ImageName,
		NewTag:  tagName,
		Digest:  tagDigest,
	}
}

var _ changeWriter = writeKustomization
