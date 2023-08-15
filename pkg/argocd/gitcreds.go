package argocd

import (
	"context"
	"fmt"
	"strings"

	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/argo-cd/v2/util/db"
	"github.com/argoproj/argo-cd/v2/util/settings"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
)

// getGitCredsSource returns git credentials source that loads credentials from the secret or from Argo CD settings
func getGitCredsSource(creds string, kubeClient *kube.KubernetesClient, wbc *WriteBackConfig) (GitCredsSource, error) {
	switch {
	case creds == "repocreds":
		return func(app *v1alpha1.Application) (git.Creds, error) {
			return getCredsFromArgoCD(wbc, kubeClient)
		}, nil
	case strings.HasPrefix(creds, "secret:"):
		return func(app *v1alpha1.Application) (git.Creds, error) {
			return getCredsFromSecret(wbc, creds[len("secret:"):], kubeClient)
		}, nil
	}
	return nil, fmt.Errorf("unexpected credentials format. Expected 'repocreds' or 'secret:<namespace>/<secret>' but got '%s'", creds)
}

// getCredsFromArgoCD loads repository credentials from Argo CD settings
func getCredsFromArgoCD(wbc *WriteBackConfig, kubeClient *kube.KubernetesClient) (git.Creds, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settingsMgr := settings.NewSettingsManager(ctx, kubeClient.Clientset, kubeClient.Namespace)
	argocdDB := db.NewDB(kubeClient.Namespace, settingsMgr, kubeClient.Clientset)
	repo, err := argocdDB.GetRepository(ctx, wbc.GitRepo)
	if err != nil {
		return nil, err
	}
	if !repo.HasCredentials() {
		return nil, fmt.Errorf("credentials for '%s' are not configured in Argo CD settings", wbc.GitRepo)
	}
	return repo.GetGitCreds(nil), nil
}

// getCredsFromSecret loads repository credentials from secret
func getCredsFromSecret(wbc *WriteBackConfig, credentialsSecret string, kubeClient *kube.KubernetesClient) (git.Creds, error) {
	var credentials map[string][]byte
	var err error
	s := strings.SplitN(credentialsSecret, "/", 2)
	if len(s) == 2 {
		credentials, err = kubeClient.GetSecretData(s[0], s[1])
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("secret ref must be in format 'namespace/name', but is '%s'", credentialsSecret)
	}

	if ok, _ := git.IsSSHURL(wbc.GitRepo); ok {
		var sshPrivateKey []byte
		if sshPrivateKey, ok = credentials["sshPrivateKey"]; !ok {
			return nil, fmt.Errorf("invalid secret %s: does not contain field sshPrivateKey", credentialsSecret)
		}
		return git.NewSSHCreds(string(sshPrivateKey), "", true), nil
	} else if git.IsHTTPSURL(wbc.GitRepo) {
		var username, password []byte
		if username, ok = credentials["username"]; !ok {
			return nil, fmt.Errorf("invalid secret %s: does not contain field username", credentialsSecret)
		}
		if password, ok = credentials["password"]; !ok {
			return nil, fmt.Errorf("invalid secret %s: does not contain field password", credentialsSecret)
		}
		return git.NewHTTPSCreds(string(username), string(password), "", "", true, ""), nil
	}
	return nil, fmt.Errorf("unknown repository type")
}
