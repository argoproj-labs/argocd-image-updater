# registry/internal

This directory is a verbatim copy of the `internal/` packages from
`github.com/distribution/distribution/v3` pinned at the same version as
`registry-scanner/go.mod`.

## Why it exists

Prior to `distribution/distribution` v3.0.0, the following packages were
publicly importable:

```
github.com/distribution/distribution/v3/registry/client
github.com/distribution/distribution/v3/registry/client/auth
github.com/distribution/distribution/v3/registry/client/auth/challenge
github.com/distribution/distribution/v3/registry/client/transport
```

Starting with **v3.0.0** these packages were moved into the module's own
`internal/` tree, making them unexportable by design. This local copy is
the only way to continue using the same functionality without forking the
entire `distribution` module.

## Maintenance

When upgrading `github.com/distribution/distribution/v3`, refresh this copy
from the new tag and review the diff before committing:

```bash
make get-distribution-internal DISTRIBUTION_VERSION=v<new-version>
git diff registry-scanner/pkg/registry/internal/
```

!!!warning "Import paths must be fixed after every refresh"
    The copied files contain upstream import paths that are always wrong and
    must be reverted manually. After every refresh, ensure all imports inside
    this directory use the local module path:

    ```
    # wrong (upstream path — revert these):
    github.com/distribution/distribution/v3/internal/...

    # correct (our module path — keep these):
    github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry/internal/...
    ```
