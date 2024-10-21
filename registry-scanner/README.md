# Registry Scanner

## Introduction

ArgoCD Image Updater provides functionalities that can be reused by other projects, most notably the feature to inspect OCI and Docker registries' contents and pick an image based on some constraints.

The registry-scanner is a reusable library for registry inspections and fetching images based on the configured strategy. Having registry-scanner as a separate library allows users to integrate just the registry-scanner.


## Current status

Registry Scanner is under active development. We would not recommend it
yet for *critical* production workloads, but feel free to give it a spin.

We're very interested in feedback on usability and the user experience as well
as in bug discoveries and enhancement requests.

**Important note:** Until the first stable version (i.e. `v1.0`) is released,
breaking changes between the releases must be expected. We will do our best
to indicate all breaking changes (and how to un-break them) in the
[Changelog](CHANGELOG.md)

## Contributing

You are welcome to contribute to this project by means of raising issues for
bugs, sending & discussing enhancement ideas or by contributing code via pull
requests.

In any case, please be sure that you have read & understood the currently known
design limitations before raising issues.

Also, if you want to contribute code, please make sure that your code

* has its functionality covered by unit tests (coverage goal is 80%),
* is correctly linted,
* is well commented,
* and last but not least is compatible with our license and CLA

Please note that in the current early phase of development, the code base is
a fast moving target and lots of refactoring will happen constantly.

## License

`registry-scanner` is open source software, released under the
[Apache 2.0 license](https://www.apache.org/licenses/LICENSE-2.0)

