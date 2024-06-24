package argocd

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/argo-cd/v2/util/cert"
	"github.com/argoproj/argo-cd/v2/util/db"
	"github.com/argoproj/argo-cd/v2/util/settings"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
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
	creds := GetGitCreds(repo, wbc.GitCreds)
	return creds, nil
}

// GetGitCreds returns the credentials from a repository configuration used to authenticate at a Git repository
// This is a slightly modified version of upstream's Repository.GetGitCreds method. We need it so it does not return the upstream type.
// TODO(jannfis): Can be removed once we have the change to the git client's getGitAskPassEnv upstream.
func GetGitCreds(repo *v1alpha1.Repository, store git.CredsStore) git.Creds {
	if repo == nil {
		return git.NopCreds{}
	}
	if repo.Password != "" {
		return git.NewHTTPSCreds(repo.Username, repo.Password, repo.TLSClientCertData, repo.TLSClientCertKey, repo.IsInsecure(), repo.Proxy, store, repo.ForceHttpBasicAuth)
	}
	if repo.SSHPrivateKey != "" {
		return git.NewSSHCreds(repo.SSHPrivateKey, getCAPath(repo.Repo), repo.IsInsecure(), store, repo.Proxy)
	}
	if repo.GithubAppPrivateKey != "" && repo.GithubAppId != 0 && repo.GithubAppInstallationId != 0 {
		return git.NewGitHubAppCreds(repo.GithubAppId, repo.GithubAppInstallationId, repo.GithubAppPrivateKey, repo.GitHubAppEnterpriseBaseURL, repo.Repo, repo.TLSClientCertData, repo.TLSClientCertKey, repo.IsInsecure(), repo.Proxy, store)
	}
	if repo.GCPServiceAccountKey != "" {
		return git.NewGoogleCloudCreds(repo.GCPServiceAccountKey, store)
	}
	return git.NopCreds{}
}

// Taken from upstream Argo CD.
// TODO(jannfis): Can be removed once we have the change to the git client's getGitAskPassEnv upstream.
func getCAPath(repoURL string) string {
	// For git ssh protocol url without ssh://, url.Parse() will fail to parse.
	// However, no warn log is output since ssh scheme url is a possible format.
	if ok, _ := git.IsSSHURL(repoURL); ok {
		return ""
	}

	hostname := ""
	// url.Parse() will happily parse most things thrown at it. When the URL
	// is either https or oci, we use the parsed hostname to retrieve the cert,
	// otherwise we'll use the parsed path (OCI repos are often specified as
	// hostname, without protocol).
	parsedURL, err := url.Parse(repoURL)
	if err != nil {
		log.Warnf("Could not parse repo URL '%s': %v", repoURL, err)
		return ""
	}
	if parsedURL.Scheme == "https" || parsedURL.Scheme == "oci" {
		hostname = parsedURL.Host
	} else if parsedURL.Scheme == "" {
		hostname = parsedURL.Path
	}

	if hostname == "" {
		log.Warnf("Could not get hostname for repository '%s'", repoURL)
		return ""
	}

	caPath, err := cert.GetCertBundlePathForRepository(hostname)
	if err != nil {
		log.Warnf("Could not get cert bundle path for repository '%s': %v", repoURL, err)
		return ""
	}

	return caPath
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
		return git.NewSSHCreds(string(sshPrivateKey), "", true, wbc.GitCreds, ""), nil
	} else if git.IsHTTPSURL(wbc.GitRepo) {
		var username, password, githubAppID, githubAppInstallationID, githubAppPrivateKey []byte
		if githubAppID, ok = credentials["githubAppID"]; ok {
			if githubAppInstallationID, ok = credentials["githubAppInstallationID"]; !ok {
				return nil, fmt.Errorf("invalid secret %s: does not contain field githubAppInstallationID", credentialsSecret)
			}
			if githubAppPrivateKey, ok = credentials["githubAppPrivateKey"]; !ok {
				return nil, fmt.Errorf("invalid secret %s: does not contain field githubAppPrivateKey", credentialsSecret)
			}
			// converting byte array to string and ultimately int64 for NewGitHubAppCreds
			intGithubAppID, err := strconv.ParseInt(string(githubAppID), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid value in field githubAppID: %w", err)
			}
			intGithubAppInstallationID, _ := strconv.ParseInt(string(githubAppInstallationID), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid value in field githubAppInstallationID: %w", err)
			}
			return git.NewGitHubAppCreds(intGithubAppID, intGithubAppInstallationID, string(githubAppPrivateKey), "", "", "", "", true, "", wbc.GitCreds), nil
		} else if username, ok = credentials["username"]; ok {
			if password, ok = credentials["password"]; !ok {
				return nil, fmt.Errorf("invalid secret %s: does not contain field password", credentialsSecret)
			}
			return git.NewHTTPSCreds(string(username), string(password), "", "", true, "", wbc.GitCreds, false), nil
		}
		return nil, fmt.Errorf("invalid repository credentials in secret %s: does not contain githubAppID or username", credentialsSecret)
	}
	return nil, fmt.Errorf("unknown repository type")
}
