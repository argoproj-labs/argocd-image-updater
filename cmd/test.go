package main

import (
	"context"

	"github.com/argoproj-labs/argocd-image-updater/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/pkg/registry"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/ratelimit"
)

func newTestCommand() *cobra.Command {
	var (
		semverConstraint   string
		strategy           string
		registriesConfPath string
		logLevel           string
		allowTags          string
		credentials        string
		kubeConfig         string
		disableKubernetes  bool
		ignoreTags         []string
		disableKubeEvents  bool
		rateLimit          int
	)
	var runCmd = &cobra.Command{
		Use:   "test IMAGE",
		Short: "Test the behaviour of argocd-image-updater",
		Long: `
The test command lets you test the behaviour of argocd-image-updater before
configuring annotations on your Argo CD Applications.

Its main use case is to tell you to which tag a given image would be updated
to using the given parametrization. Command line switches can be used as a
way to supply the required parameters.
`,
		Example: `
# In the most simple form, check for the latest available (semver) version of
# an image in the registry
argocd-image-updater test nginx

# Check to which version the nginx image within the 1.17 branch would be
# updated to, using the default semver strategy
argocd-image-updater test nginx --semver-constraint v1.17.x

# Check for the latest built image for a tag that matches a pattern
argocd-image-updater test nginx --allow-tags '^1.19.\d+(\-.*)*$' --update-strategy latest
`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				cmd.HelpFunc()(cmd, args)
				log.Fatalf("image needs to be specified")
			}

			if err := log.SetLogLevel(logLevel); err != nil {
				log.Fatalf("could not set log level to %s: %v", logLevel, err)
			}

			var kubeClient *kube.KubernetesClient
			var err error
			if !disableKubernetes {
				ctx := context.Background()
				kubeClient, err = getKubeConfig(ctx, "", kubeConfig)
				if err != nil {
					log.Fatalf("could not create K8s client: %v", err)
				}
			}

			vc := &image.VersionConstraint{
				Constraint: semverConstraint,
				SortMode:   image.VersionSortSemVer,
			}

			vc.SortMode = image.ParseUpdateStrategy(strategy)

			if allowTags != "" {
				vc.MatchFunc, vc.MatchArgs = image.ParseMatchfunc(allowTags)
			}

			vc.IgnoreList = ignoreTags

			img := image.NewFromIdentifier(args[0])
			log.WithContext().
				AddField("registry", img.RegistryURL).
				AddField("image_name", img.ImageName).
				Infof("getting image")

			if registriesConfPath != "" {
				if err := registry.LoadRegistryConfiguration(registriesConfPath, false); err != nil {
					log.Fatalf("could not load registries configuration: %v", err)
				}
			}

			ep, err := registry.GetRegistryEndpoint(img.RegistryURL)
			if err != nil {
				log.Fatalf("could not get registry endpoint: %v", err)
			}

			if err := ep.SetEndpointCredentials(kubeClient); err != nil {
				log.Fatalf("could not set registry credentials: %v", err)
			}

			checkFlag := func(f *pflag.Flag) {
				if f.Name == "rate-limit" {
					log.Infof("Overriding registry rate-limit to %d requests per second", rateLimit)
					ep.Limiter = ratelimit.New(rateLimit)
				}
			}

			cmd.Flags().Visit(checkFlag)

			var creds *image.Credential
			var username, password string
			if credentials != "" {
				credSrc, err := image.ParseCredentialSource(credentials, false)
				if err != nil {
					log.Fatalf("could not parse credential definition '%s': %v", credentials, err)
				}
				creds, err = credSrc.FetchCredentials(ep.RegistryAPI, kubeClient)
				if err != nil {
					log.Fatalf("could not fetch credentials: %v", err)
				}
				username = creds.Username
				password = creds.Password
			}

			regClient, err := registry.NewClient(ep, username, password)
			if err != nil {
				log.Fatalf("could not create registry client: %v", err)
			}

			log.WithContext().
				AddField("image_name", img.ImageName).
				Infof("Fetching available tags and metadata from registry")

			tags, err := ep.GetTags(img, regClient, vc)
			if err != nil {
				log.Fatalf("could not get tags: %v", err)
			}

			log.WithContext().
				AddField("image_name", img.ImageName).
				Infof("Found %d tags in registry", len(tags.Tags()))

			upImg, err := img.GetNewestVersionFromTags(vc, tags)
			if err != nil {
				log.Fatalf("could not get updateable image from tags: %v", err)
			}
			if upImg == nil {
				log.Infof("no newer version of image found")
				return
			}

			log.Infof("latest image according to constraint is %s", img.WithTag(upImg))
		},
	}

	runCmd.Flags().StringVar(&semverConstraint, "semver-constraint", "", "only consider tags matching semantic version constraint")
	runCmd.Flags().StringVar(&allowTags, "allow-tags", "", "only consider tags in registry that satisfy the match function")
	runCmd.Flags().StringArrayVar(&ignoreTags, "ignore-tags", nil, "ignore tags in registry that match given glob pattern")
	runCmd.Flags().StringVar(&strategy, "update-strategy", "semver", "update strategy to use, one of: semver, latest)")
	runCmd.Flags().StringVar(&registriesConfPath, "registries-conf-path", "", "path to registries configuration")
	runCmd.Flags().StringVar(&logLevel, "loglevel", "debug", "log level to use (one of trace, debug, info, warn, error)")
	runCmd.Flags().BoolVar(&disableKubernetes, "disable-kubernetes", false, "whether to disable the Kubernetes client")
	runCmd.Flags().StringVar(&kubeConfig, "kubeconfig", "", "path to your Kubernetes client configuration")
	runCmd.Flags().StringVar(&credentials, "credentials", "", "the credentials definition for the test (overrides registry config)")
	runCmd.Flags().BoolVar(&disableKubeEvents, "disable-kubernetes-events", false, "Disable kubernetes events")
	runCmd.Flags().IntVar(&rateLimit, "rate-limit", 20, "specificy registry rate limit (overrides registry.conf)")
	return runCmd
}
