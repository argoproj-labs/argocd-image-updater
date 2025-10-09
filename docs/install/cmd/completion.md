## Synopsis

`argocd-image-updater completion [command] [flags]`

## Description

Generates the autocompletion script for argocd-image-updater for the specified shell.
See each sub-command's help for details on how to use the generated script.

## Flags

**-h, --help**

Help for completion

## Sub-Command bash

### Synopsis

`argocd-image-updater completion bash [flags]`

### Description

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(argocd-image-updater completion bash)

To load completions for every new session, execute once:

**Linux**

	argocd-image-updater completion bash > /etc/bash_completion.d/argocd-image-updater

**macOS**

	argocd-image-updater completion bash > $(brew --prefix)/etc/bash_completion.d/argocd-image-updater

You will need to start a new shell for this setup to take effect.

### Flags

**-h, --help**

Help for bash.

**--no-descriptions**

Disable completion descriptions.

## Sub-Command fish

### Synopsis

`argocd-image-updater completion fish [flags]`

### Description

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	argocd-image-updater completion fish | source

To load completions for every new session, execute once:

	argocd-image-updater completion fish > ~/.config/fish/completions/argocd-image-updater.fish

You will need to start a new shell for this setup to take effect.

### Flags

**-h, --help**

Help for fish.

**--no-descriptions**

Disable completion descriptions.

## Sub-Command powershell

### Synopsis

`argocd-image-updater completion powershell [flags]`

### Description

Generate the autocompletion script for powershell.

To load completions in your current shell session:

	argocd-image-updater completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.

### Flags

**-h, --help**

Help for powershell
 
**--no-descriptions**

Disable completion descriptions

## Sub-Command zsh

### Synopsis

argocd-image-updater completion zsh [flags]

### Description

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(argocd-image-updater completion zsh)

To load completions for every new session, execute once:

**Linux:**

	argocd-image-updater completion zsh > "${fpath[1]}/_argocd-image-updater"

**macOS:**

	argocd-image-updater completion zsh > $(brew --prefix)/share/zsh/site-functions/_argocd-image-updater

You will need to start a new shell for this setup to take effect.
  
### Flags

**-h, --help**

help for zsh

**--no-descriptions**

disable completion descriptions
