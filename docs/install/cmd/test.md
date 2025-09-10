## Synopsis

argocd-image-updater test IMAGE [flags]

## Description

The test command lets you test the behaviour of argocd-image-updater before
configuring annotations on your Argo CD Applications.

Its main use case is to tell you to which tag a given image would be updated
to using the given parametrization. Command line switches can be used as a
way to supply the required parameters.
  
## Flags

**--allow-tags *match function***            
      
Only consider tags in registry that satisfy the match function
      
**--credentials *defintion***            

The credentials definition for the test (overrides registry config)

**-h, --help**

Help for test
      
**--ignore-tags *string array***

Ignore tags in registry that match given glob pattern

**--kubeconfig *path***            

Path to your Kubernetes client configuration
      
**--loglevel *level***

Log level to use (one of trace, debug, info, warn, error) (default "debug")
      
**--platforms *platforms***   

Limit images to given platforms (default [darwin/arm64])

**--rate-limit *limit***    

Specific registry rate limit (overrides registry.conf) (default 20)

**--registries-conf-path *path***

Path to registries configuration
 
**--semver-constraint *constraint***  

Only consider tags matching semantic version constraint

**--update-strategy *strategy***

Update strategy to use (one of semver, newest-build, alphabetical, digest) (default "semver")

## Examples

In the most simple form, check for the latest available (semver) version of
an image in the registry

`argocd-image-updater test nginx`

Check to which version the nginx image within the 1.17 branch would be
updated to, using the default semver strategy

`argocd-image-updater test nginx --semver-constraint v1.17.x`

Check for the latest built image for a tag that matches a pattern

`argocd-image-updater test nginx --allow-tags '^1.19.\d+(\-.*)*$' --update-strategy newest-build`
