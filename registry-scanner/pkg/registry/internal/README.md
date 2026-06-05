# registry/internal

This directory is a verbatim copy of
[`github.com/distribution/distribution/internal`](https://github.com/distribution/distribution/tree/main/internal).

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

When upgrading `github.com/distribution/distribution/v3`, compare the
upstream [`internal/`](https://github.com/distribution/distribution/tree/main/internal)
directory against this copy and apply any relevant changes.
