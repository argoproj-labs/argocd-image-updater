package argocd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"sigs.k8s.io/kustomize/api/konfig"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/order"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
)

// TemplateCommitMessage renders a commit message template and returns it
// as a string, including image labels that can be used to add custom information to commit messages.
// If the template could not be rendered, returns a default
// message.
func TemplateCommitMessage(ctx context.Context, tpl *template.Template, appName string, changeList []ChangeEntry) string {
	log := log.LoggerFromContext(ctx)
	var cmBuf bytes.Buffer

	type commitMessageChange struct {
		Image  string
		OldTag string
		NewTag string
		Labels map[string]string
	}

	type commitMessageTemplate struct {
		AppName    string
		AppChanges []commitMessageChange
	}

	// We need to transform the change list into something more viable for the
	// writer of a template.
	changes := make([]commitMessageChange, 0)
	for _, c := range changeList {
		changes = append(changes, commitMessageChange{
			Image:  c.Image.ImageName,
			OldTag: c.OldTag.String(),
			NewTag: c.NewTag.String(),
			Labels: c.NewTag.Labels,
		})
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
func TemplateBranchName(ctx context.Context, branchName, appNamespace, appName string, changeList []ChangeEntry) string {
	log := log.LoggerFromContext(ctx)
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
		AppNamespace string
		AppName      string
		Images       []imageChange
		SHA256       string
	}

	// Let's add a unique hash to the template
	hasher := sha256.New()

	// We need to transform the change list into something more viable for the
	// writer of a template.
	changes := make([]imageChange, 0)
	for _, c := range changeList {
		changes = append(changes, imageChange{c.Image.ImageName, c.Image.ImageAlias, c.OldTag.String(), c.NewTag.String()})
		id := fmt.Sprintf("%v-%v-%v,", c.Image.ImageName, c.OldTag.String(), c.NewTag.String())
		_, hasherErr := hasher.Write([]byte(id))
		log.Infof("writing to hasher %v", id)
		if hasherErr != nil {
			log.Errorf("could not write image string to hasher: %v", hasherErr)
			return ""
		}
	}

	tplData := branchNameTemplate{
		AppNamespace: appNamespace,
		AppName:      appName,
		Images:       changes,
		SHA256:       hex.EncodeToString(hasher.Sum(nil)),
	}

	err2 := tpl.Execute(&cmBuf, tplData)
	if err2 != nil {
		log.Errorf("could not execute template for Git branch name: %v", err2)
		return ""
	}

	toReturn := cmBuf.String()

	if len(toReturn) > 255 {
		trunc := toReturn[:255]
		log.Warnf("write-branch name %v exceeded 255 characters and was truncated to %v", toReturn, trunc)
		return trunc
	} else {
		return toReturn
	}
}

type changeWriter func(ctx context.Context, applicationImages *ApplicationImages, gitC git.Client) (err error, skip bool)

// getWriteBackBranch returns the branch to use for write-back operations.
// It first checks for a branch specified in annotations, then uses the
// targetRevision from the matching git source, falling back to getApplicationSource.
func getWriteBackBranch(ctx context.Context, app *v1alpha1.Application, wbc *WriteBackConfig) string {
	log := log.LoggerFromContext(ctx)
	if app == nil {
		return ""
	}
	// If git repository is specified, find matching source
	if wbc != nil && wbc.GitRepo != "" {
		gitRepo := wbc.GitRepo
		if app.Spec.HasMultipleSources() {
			for _, s := range app.Spec.Sources {
				if s.RepoURL == gitRepo {
					log.Debugf("Using target revision '%s' from matching source '%s'", s.TargetRevision, gitRepo)
					return s.TargetRevision
				}
			}
			log.Debugf("No matching source found for git repository %s, falling back to primary source", gitRepo)
		}
	}
	// Fall back to getApplicationSource's targetRevision
	// This maintains consistency with how other parts of the code select the source
	return getApplicationSource(ctx, app, wbc).TargetRevision
}

// commitChangesGit commits any changes required for updating one or more images
// after the UpdateApplication cycle has finished.
func commitChangesGit(ctx context.Context, applicationImages *ApplicationImages, changeList []ChangeEntry, write changeWriter) error {
	logCtx := log.LoggerFromContext(ctx)

	app := applicationImages.Application
	wbc := applicationImages.WriteBackConfig
	creds, err := wbc.GetCreds(&app)
	if err != nil {
		return fmt.Errorf("could not get creds for repo '%s': %v", wbc.GitRepo, err)
	}
	var gitC git.Client
	if wbc.GitClient == nil {
		tempRoot, err := os.MkdirTemp(os.TempDir(), fmt.Sprintf("git-%s", app.Name))
		if err != nil {
			return err
		}
		defer func() {
			err := os.RemoveAll(tempRoot)
			if err != nil {
				logCtx.Errorf("could not remove temp dir: %v", err)
			}
		}()
		gitC, err = git.NewClientExt(wbc.GitRepo, tempRoot, creds, false, false, "")
		if err != nil {
			return err
		}
	} else {
		gitC = wbc.GitClient
	}
	err = gitC.Init(ctx)
	if err != nil {
		return err
	}

	// The branch to checkout is either a configured branch in the write-back
	// config, or taken from the application spec's targetRevision. If the
	// target revision is set to the special value HEAD, or is the empty
	// string, we'll try to resolve it to a branch name.
	var checkOutBranch string
	if wbc.GitBranch != "" {
		checkOutBranch = wbc.GitBranch
	} else {
		checkOutBranch = getWriteBackBranch(ctx, &app, wbc)
	}
	logCtx.Tracef("targetRevision for update is '%s'", checkOutBranch)
	if checkOutBranch == "" || checkOutBranch == "HEAD" {
		checkOutBranch, err = gitC.SymRefToBranch(ctx, checkOutBranch)
		logCtx.Infof("resolved remote default branch to '%s' and using that for operations", checkOutBranch)
		if err != nil {
			return err
		}
	}

	// The push branch is by default the same as the checkout branch, unless
	// specified after a : separator git-branch annotation, in which case a
	// new branch will be made following a template that can use the list of
	// changed images.
	pushBranch := checkOutBranch

	// Set custom pushBranch name for PR/MR mode
	if wbc.PRProvider > 0 {
		// The default template produces a stable branch name per app and new image tag.
		customTemplate := "image-updater-{{.AppNamespace}}-{{.AppName}}-{{.SHA256}}"
		logCtx.Tracef("setting git push branch for PR/MR mode using custom template '%s'", customTemplate)
		pushBranch = TemplateBranchName(ctx, customTemplate, app.Namespace, app.Name, changeList)
		if pushBranch == "" {
			return fmt.Errorf("git branch name could not be created from the template: %s", customTemplate)
		}
		wbc.PullRequest, err = buildPullRequest(ctx, wbc, app.Namespace, app.Name, checkOutBranch, pushBranch)
		if err != nil {
			return err
		}
	} else if wbc.GitWriteBranch != "" {
		// use GitWriteBranch for git mode without PR
		logCtx.Debugf("Using branch template: %s", wbc.GitWriteBranch)
		pushBranch = TemplateBranchName(ctx, wbc.GitWriteBranch, "", "", changeList)
		if pushBranch == "" {
			return fmt.Errorf("git branch name could not be created from the template: %s", wbc.GitWriteBranch)
		}
	}

	// If the pushBranch already exists in the remote origin, directly use it.
	// Otherwise, create the new pushBranch from checkoutBranch
	if checkOutBranch != pushBranch {
		fetchErr := gitC.ShallowFetch(ctx, pushBranch, 1)
		if fetchErr != nil {
			err = gitC.ShallowFetch(ctx, checkOutBranch, 1)
			if err != nil {
				return err
			}
			logCtx.Debugf("Creating branch '%s' and using that for push operations", pushBranch)
			err = gitC.Branch(ctx, checkOutBranch, pushBranch)
			if err != nil {
				return err
			}
		}
	} else {
		err = gitC.ShallowFetch(ctx, checkOutBranch, 1)
		if err != nil {
			return err
		}
	}

	err = gitC.Checkout(ctx, pushBranch, false)
	if err != nil {
		return err
	}

	if err, skip := write(ctx, applicationImages, gitC); err != nil {
		return err
	} else if skip {
		return nil
	}

	commitOpts := &git.CommitOptions{}
	if wbc.GitCommitMessage != "" {
		cm, err := os.CreateTemp("", "image-updater-commit-msg")
		if err != nil {
			return fmt.Errorf("could not create temp file: %v", err)
		}
		logCtx.Debugf("Writing commit message to %s", cm.Name())
		err = os.WriteFile(cm.Name(), []byte(wbc.GitCommitMessage), 0600)
		if err != nil {
			_ = cm.Close()
			return fmt.Errorf("could not write commit message to %s: %v", cm.Name(), err)
		}
		commitOpts.CommitMessagePath = cm.Name()
		_ = cm.Close()
		defer os.Remove(cm.Name())
	}

	// Set username and e-mail address used to identify the commiter
	if wbc.GitCommitUser != "" && wbc.GitCommitEmail != "" {
		err = gitC.Config(ctx, wbc.GitCommitUser, wbc.GitCommitEmail)
		if err != nil {
			return err
		}
	}

	if wbc.GitCommitSigningKey != "" {
		commitOpts.SigningKey = wbc.GitCommitSigningKey
	}

	commitOpts.SigningMethod = wbc.GitCommitSigningMethod
	commitOpts.SignOff = wbc.GitCommitSignOff

	err = gitC.Commit(ctx, "", commitOpts)
	if err != nil {
		return err
	}
	err = gitC.Push(ctx, "origin", pushBranch, pushBranch != checkOutBranch)
	if err != nil {
		return err
	}

	return nil
}

// BatchCommitChangesGit performs git write-back for multiple applications in a single
// git transaction. Instead of each app doing its own clone→fetch→checkout→write→commit→push
// cycle, it does a single clone→fetch→checkout, writes all files, then one commit→push.
// For N apps sharing the same repo+branch, this reduces N round-trips to 1.
//
// All pending writes must target the same git repository and branch.
// Returns a map of app names to errors for any individual write failures.
func BatchCommitChangesGit(ctx context.Context, pendingWrites []*PendingWrite, state *SyncIterationState) map[string]error {
	logCtx := log.LoggerFromContext(ctx)
	errors := make(map[string]error)

	if len(pendingWrites) == 0 {
		return errors
	}

	// Use the first write's config as representative for repo/branch/creds.
	// All writes in this batch must share the same repo+branch.
	firstWBC := pendingWrites[0].App.WriteBackConfig
	firstApp := pendingWrites[0].App.Application

	// Acquire the repo lock once for the entire batch
	if firstWBC.RequiresLocking() {
		lock := state.GetRepositoryLock(firstWBC.GitRepo)
		lock.Lock()
		defer lock.Unlock()
	}

	creds, err := firstWBC.GetCreds(&firstApp)
	if err != nil {
		for _, pw := range pendingWrites {
			errors[pw.AppName] = fmt.Errorf("could not get creds for repo '%s': %v", firstWBC.GitRepo, err)
		}
		return errors
	}

	// Create a single temp directory and git client for the entire batch
	var gitC git.Client
	if firstWBC.GitClient != nil {
		// Use injected git client (e.g. for testing)
		gitC = firstWBC.GitClient
	} else {
		tempRoot, err := os.MkdirTemp(os.TempDir(), "git-batch")
		if err != nil {
			for _, pw := range pendingWrites {
				errors[pw.AppName] = err
			}
			return errors
		}
		defer func() {
			if removeErr := os.RemoveAll(tempRoot); removeErr != nil {
				logCtx.Errorf("could not remove temp dir: %v", removeErr)
			}
		}()

		gitC, err = git.NewClientExt(firstWBC.GitRepo, tempRoot, creds, false, false, "")
		if err != nil {
			for _, pw := range pendingWrites {
				errors[pw.AppName] = err
			}
			return errors
		}
	}
	if err = gitC.Init(ctx); err != nil {
		for _, pw := range pendingWrites {
			errors[pw.AppName] = err
		}
		return errors
	}

	// Determine the checkout branch from the first app's config
	var checkOutBranch string
	if firstWBC.GitBranch != "" {
		checkOutBranch = firstWBC.GitBranch
	} else {
		checkOutBranch = getWriteBackBranch(ctx, &firstApp, firstWBC)
	}
	if checkOutBranch == "" || checkOutBranch == "HEAD" {
		checkOutBranch, err = gitC.SymRefToBranch(ctx, checkOutBranch)
		logCtx.Infof("resolved remote default branch to '%s' and using that for operations", checkOutBranch)
		if err != nil {
			for _, pw := range pendingWrites {
				errors[pw.AppName] = err
			}
			return errors
		}
	}

	// Single fetch + checkout for the entire batch
	if err = gitC.ShallowFetch(ctx, checkOutBranch, 1); err != nil {
		for _, pw := range pendingWrites {
			errors[pw.AppName] = err
		}
		return errors
	}
	if err = gitC.Checkout(ctx, checkOutBranch, false); err != nil {
		for _, pw := range pendingWrites {
			errors[pw.AppName] = err
		}
		return errors
	}

	logCtx.Infof("Batch writing %d application(s) in a single git transaction", len(pendingWrites))

	// Phase: Write all files. Track which apps had actual changes.
	var appsWithChanges []*PendingWrite
	for _, pw := range pendingWrites {
		wbc := pw.App.WriteBackConfig
		var write changeWriter
		if wbc.KustomizeBase != "" {
			write = writeKustomization
		} else {
			write = writeOverrides
		}

		appCtx := log.ContextWithLogger(ctx, logCtx.WithField("application", pw.AppName))
		if writeErr, skip := write(appCtx, pw.App, gitC); writeErr != nil {
			logCtx.Errorf("Error writing changes for app %s: %v", pw.AppName, writeErr)
			errors[pw.AppName] = writeErr
		} else if !skip {
			appsWithChanges = append(appsWithChanges, pw)
		} else {
			logCtx.Debugf("App %s: file already up-to-date, skipping", pw.AppName)
		}
	}

	if len(appsWithChanges) == 0 {
		logCtx.Infof("Batch: all %d app file(s) already up-to-date, nothing to commit", len(pendingWrites))
		return errors
	}

	// Build a combined commit message
	commitMsg := buildBatchCommitMessage(appsWithChanges)

	// Use git config from the first app (they should all share the same user/email)
	if firstWBC.GitCommitUser != "" && firstWBC.GitCommitEmail != "" {
		if err = gitC.Config(ctx, firstWBC.GitCommitUser, firstWBC.GitCommitEmail); err != nil {
			for _, pw := range appsWithChanges {
				errors[pw.AppName] = err
			}
			return errors
		}
	}

	commitOpts := &git.CommitOptions{}

	// Write commit message to temp file
	cm, err := os.CreateTemp("", "image-updater-batch-commit-msg")
	if err != nil {
		for _, pw := range appsWithChanges {
			errors[pw.AppName] = fmt.Errorf("could not create temp file: %v", err)
		}
		return errors
	}
	if err = os.WriteFile(cm.Name(), []byte(commitMsg), 0600); err != nil {
		_ = cm.Close()
		for _, pw := range appsWithChanges {
			errors[pw.AppName] = fmt.Errorf("could not write commit message: %v", err)
		}
		return errors
	}
	commitOpts.CommitMessagePath = cm.Name()
	_ = cm.Close()
	defer os.Remove(cm.Name())

	if firstWBC.GitCommitSigningKey != "" {
		commitOpts.SigningKey = firstWBC.GitCommitSigningKey
	}
	commitOpts.SigningMethod = firstWBC.GitCommitSigningMethod
	commitOpts.SignOff = firstWBC.GitCommitSignOff

	// Single commit + push for all changes
	if err = gitC.Commit(ctx, "", commitOpts); err != nil {
		for _, pw := range appsWithChanges {
			errors[pw.AppName] = fmt.Errorf("git commit failed: %v", err)
		}
		return errors
	}
	if err = gitC.Push(ctx, "origin", checkOutBranch, false); err != nil {
		for _, pw := range appsWithChanges {
			errors[pw.AppName] = fmt.Errorf("git push failed: %v", err)
		}
		return errors
	}

	logCtx.Infof("Batch: successfully committed and pushed %d app update(s) in a single transaction", len(appsWithChanges))
	return errors
}

// buildBatchCommitMessage creates a commit message summarizing all batched updates.
func buildBatchCommitMessage(writes []*PendingWrite) string {
	if len(writes) == 1 {
		// For a single app, use the app's own commit message if available
		if msg := writes[0].App.WriteBackConfig.GitCommitMessage; msg != "" {
			return msg
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("build: batch update of %d application(s)\n\n", len(writes)))
	for _, pw := range writes {
		b.WriteString(fmt.Sprintf("- %s:", pw.AppName))
		for _, c := range pw.ChangeList {
			b.WriteString(fmt.Sprintf(" %s %s→%s", c.Image.ImageName, c.OldTag.String(), c.NewTag.String()))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func writeOverrides(ctx context.Context, applicationImages *ApplicationImages, gitC git.Client) (err error, skip bool) {
	logCtx := log.LoggerFromContext(ctx)
	wbc := applicationImages.WriteBackConfig
	targetExists := true
	targetFile := path.Join(gitC.Root(), wbc.Target)
	_, err = os.Stat(targetFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return
		} else {
			targetExists = false
		}
	}

	// If the target file already exist in the repository, we will check whether
	// our generated new file is the same as the existing one, and if yes, we
	// don't proceed further for commit.
	var override []byte
	var originalData []byte
	if targetExists {
		originalData, err = os.ReadFile(targetFile)
		if err != nil {
			return err, false
		}
		override, err = marshalParamsOverride(ctx, applicationImages, originalData)
		if err != nil {
			return
		}
		if string(originalData) == string(override) {
			logCtx.Debugf("target parameter file and marshaled data are the same, skipping commit.")
			return nil, true
		}
	} else {
		override, err = marshalParamsOverride(ctx, applicationImages, nil)
		if err != nil {
			return
		}
	}

	dir := filepath.Dir(targetFile)
	err = os.MkdirAll(dir, 0700)
	if err != nil {
		return
	}

	err = os.WriteFile(targetFile, override, 0600)
	if err != nil {
		return
	}

	if !targetExists {
		err = gitC.Add(ctx, targetFile)
	}
	return
}

var _ changeWriter = writeOverrides

// writeKustomization writes any changes required for updating one or more images to a kustomization.yml
func writeKustomization(ctx context.Context, applicationImages *ApplicationImages, gitC git.Client) (err error, skip bool) {
	app := applicationImages.Application
	wbc := applicationImages.WriteBackConfig
	logCtx := log.LoggerFromContext(ctx)

	base := filepath.Join(gitC.Root(), wbc.KustomizeBase)

	logCtx.Infof("updating base %s", base)

	kustFile := findKustomization(base)
	if kustFile == "" {
		return fmt.Errorf("could not find kustomization in %s", base), false
	}
	source := getApplicationSource(ctx, &app, wbc)
	if source == nil {
		return fmt.Errorf("failed to find source for kustomization in %s", base), false
	}

	kustomize := source.Kustomize
	images := v1alpha1.KustomizeImages{}
	if kustomize != nil {
		images = kustomize.Images
	}

	filterFunc, err := imagesFilter(images)
	if err != nil {
		return err, false
	}

	return updateKustomizeFile(ctx, filterFunc, kustFile)
}

// updateKustomizeFile reads the kustomization file at path, applies the filter to it, and writes the result back
// to the file. This is the same behavior as kyaml.UpdateFile, but it preserves the original order of YAML fields
// and indentation of YAML sequences to minimize git diffs.
func updateKustomizeFile(ctx context.Context, filter kyaml.Filter, path string) (error, bool) {
	log := log.LoggerFromContext(ctx)

	// Open the input file for read
	yRaw, err := os.ReadFile(path)
	if err != nil {
		return err, false
	}

	// Read the yaml document from bytes
	originalYSlice, err := kio.FromBytes(yRaw)
	if err != nil {
		return err, false
	}

	// Check that we are dealing with a single document
	if len(originalYSlice) != 1 {
		return errors.New("target parameter file should contain a single YAML document"), false
	}
	originalY := originalYSlice[0]

	// Get the (parsed) original document
	originalData, err := originalY.String()
	if err != nil {
		return err, false
	}

	// Create a reader, preserving indentation of sequences
	var out bytes.Buffer
	rw := &kio.ByteReadWriter{
		Reader:            bytes.NewBuffer(yRaw),
		Writer:            &out,
		PreserveSeqIndent: true,
	}

	// Read from input buffer
	newYSlice, err := rw.Read()
	if err != nil {
		return err, false
	}
	// We can safely assume we have a single document from the previous check
	newY := newYSlice[0]

	// Update the yaml
	if err := newY.PipeE(filter); err != nil {
		return err, false
	}

	// Preserve the original order of fields
	if err := order.SyncOrder(originalY, newY); err != nil {
		return err, false
	}

	// Write the yaml document to the output buffer
	if err = rw.Write([]*kyaml.RNode{newY}); err != nil {
		return err, false
	}

	// newY contains metadata used by kio to preserve sequence indentation,
	// hence we need to parse the output buffer instead
	newParsedY, err := kyaml.Parse(out.String())
	if err != nil {
		return err, false
	}
	newData, err := newParsedY.String()
	if err != nil {
		return err, false
	}

	// Compare the updated document with the original document
	if originalData == newData {
		log.Debugf("target parameter file and marshaled data are the same, skipping commit.")
		return nil, true
	}

	// Write to file the changes
	if err := os.WriteFile(path, out.Bytes(), 0600); err != nil {
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
