# Update strategies

## <a name="supported-strategies"></a>Supported update strategies

An update strategy defines how Argo CD Image Updater will find new versions of
an image that is to be updated.

Argo CD Image Updater supports different update strategies for the images that
are configured to be tracked and updated.

You can configure the update strategy to be used at multiple levels in the
`ImageUpdater` custom resource, with the default being the `semver` strategy:

* **Global level**: In `spec.commonUpdateSettings` - applies to all applications
  unless overridden at a more specific level
* **Per application level**: In `spec.applicationRefs[].commonUpdateSettings` -
  overrides the global configuration for specific applications
* **Per image level**: In `spec.applicationRefs[].images[].commonUpdateSettings` -
  overrides both global and application-level configuration for specific images

Image-level configuration takes precedence over application-level configuration,
which takes precedence over global configuration.

The following update strategies are currently supported:

* [semver](#strategy-semver) - Update to the latest version of an image
  considering semantic versioning constraints
* [newest-build](#strategy-latest) - Update to the most recently built image found in a registry (deprecated alias: `latest` — still accepted but may be removed in a future release)
* [digest](#strategy-digest) - Update to the latest version of a given version (tag), using the tag's SHA digest
* [alphabetical](#strategy-name) - Sorts tags alphabetically and update to the one with the highest cardinality (deprecated alias: `name` — still accepted but may be removed in a future release)
* [calver](#strategy-calver) - Update to the newest tag according to a [calendar versioning](https://calver.org) scheme

!!!warning "Renamed image update strategies"
    The `latest` strategy has been renamed to `newest-build`, and `name` strategy has been renamed to `alphabetical`. 
    Please switch to the new convention as support for the old naming convention will be removed in future releases.

Some of the strategies will require additional configuration, or can be tweaked
with additional parameters. Please have a look at the
[image configuration](../configuration/images.md)
documentation for more details.

## <a name="mutable-immutable"></a>Mutable vs immutable tags

Please note that all update strategies except `digest` assume tags to be
*immutable* and that new images will be pushed with a new, unique tag. If
you want to update to *mutable* tags (e.g. the commonly used `latest` tag),
you should use the `digest` strategy.

## Update strategies in detail
### <a name="strategy-semver"></a>semver - Update to semantic versions

This is the default strategy.

Strategy name: `semver`

Basic configuration:

```yaml
images:
  - alias: "someImage"
    imageName: "some/image[:<version_constraint>]"
    commonUpdateSettings:
      # Specifying update-strategy is optional, because semver is the default
      updateStrategy: "semver"
```

The `semver` strategy allows you to track & update images which use tags that
follow the
[semantic versioning scheme](https://semver.org). Tag names must contain semver
compatible identifiers in the format `X.Y.Z`, where `X`, `Y` and `Z` must be
whole numbers. An optional prefix of `v`, e.g. `vX.Y.Z` is allowed, and both
variants are treated equal (so, a constraint of `v1.x` would match a tag `1.0`
and a constraint of `1.x` also matches a tag `v1.0`).

Updating to pre-release versions (e.g. `-rc1`) is supported, but must be 
explicitly allowed (see below).

This will allow you to update to the latest version of an image within a given
patch branch or minor release, or just to the latest version that has is tagged
with a valid semantic version identifier.

To tell Argo CD Image Updater which versions are allowed, simply specify a semver
version constraint in the `imageName` field. For example,
to allow updates to the latest patch release within the `1.2` minor release
branch, use

```yaml
images:
  - alias: "someImage"
    imageName: "some/image:1.2.x"
```

The above example would update to any new tag pushed to the registry matching
this constraint, e.g. `1.2.5`, `1.2.12` etc., but not to a new minor version
(e.g. `1.3`).

!!!note "A note on the current image tag"
    The current application tag does not need to follow semver. The updater will
    find the newest tag from the registry that matches the version constraint,
    regardless of the format of the currently running tag. For example, an
    application running the `latest` tag can be updated to a semver-compatible
    version using a constraint like `1.x`.

Likewise, to allow updates to any minor release within the major version `1`,
use

```yaml
images:
  - alias: "someImage"
    imageName: "some/image:1.x"
```

The above example would update to any new tag pushed to the registry matching
this constraint, e.g. `1.2.12`, `1.3.0`, `1.15.2` etc., but not to a new major
version (e.g. `2.0`).

If you also want to allow updates to pre-release versions (e.g. `v2.0-rc1`),
you need to append the suffix `-0` to the constraint, for example

```yaml
images:
  - alias: "someImage"
    imageName: "some/image:2.x-0"
```

If no version constraint is specified in the list of allowed images, Argo CD
Image Updater will pick the highest version number found in the registry.

Argo CD Image Updater will omit any tags from your registry that do not match 
a semantic version when using the `semver` update strategy.



### <a name="strategy-latest"></a>newest-build - Update to the most recently built image


!!!warning "Renamed image update strategies"
    The `latest` strategy has been renamed to `newest-build`.
    Please switch to the new convention as support for the old naming convention will be removed in future releases.
    Detected usage of `latest` will result in a warning message within the image-updater controller logs.

!!!warning
    As of November 2020, Docker Hub has introduced pull limits for accounts on
    the free plan and unauthenticated requests. The `latest` or `newest-build` update strategy
    will perform manifest pulls for determining the most recently pushed tags,
    and these will count into your pull limits. So unless you are not affected
    by these pull limits, it is **not recommended** to use the `latest` or `newest-build` update
    strategy with images hosted on Docker Hub.

!!!note
    If you are using *reproducible builds* for your container images (e.g. if
    your build pipeline always sets the creation date of the image to the same
    value), the `latest` or `newest-build` strategy will not be able to determine which tag to
    update to.

Strategy name: `newest-build` (deprecated alias: `latest`)

Basic configuration:

```yaml
images:
  - alias: "alias"
    imageName: "some/image"
    commonUpdateSettings:
      updateStrategy: "newest-build"
```

Argo CD Image Updater can update to the image that has the most recent build
date, and is tagged with an arbitrary name (e.g. a Git commit SHA, or even a
random string). 

It is important to understand, that this strategy will consider the build date
of the image, and not the date of when the image was tagged or pushed to the
registry. If you are tagging the same image with multiple tags, these tags
will have the same build date. In this case, Argo CD Image Updater will sort
the tag names lexically descending and pick the last tag name of that list.
For example, consider an image that was tagged with the `f33bacd`, `dev`
and `latest` tags. You might want to have the `f33bacd` tag set for your
application, but Image Updater will pick the `latest` tag name. In order to
prevent such a situation, you need to further restrict the tags that Image
Updater will inspect, see below.

By default, this update strategy will inspect all the tags it found in the
image's repository. If you wish to allow only certain tags to be considered
for update, you will need additional configuration. For example,

```yaml
images:
  - alias: "myimage"
    imageName: "some/image"
    commonUpdateSettings:
      updateStrategy: "newest-build"
      allowTags: "regexp:^[0-9a-f]{7}$"
```

would only consider tags that match a given regular expression for update. In
this case, the regular expression matches a 7-digit hexadecimal string that
could represent the short version of a Git commit SHA, so it would match tags
like `a5fb3d3` or `f7bb2e3`, but not `latest` or `master`.

Likewise, you can ignore a certain list of tags from your repository:

```yaml
images:
  - alias: "myimage"
    imageName: "some/image"
    commonUpdateSettings:
      updateStrategy: "newest-build"
      ignoreTags: [ "latest", "master" ]
```

This would allow for considering all tags found but `latest` and `master`. You
can read more about filtering tags
[here](../configuration/images.md#filtering-tags).

### <a name="strategy-digest"></a>digest - Update to the most recent pushed version of a given tag

Strategy name: `digest`

Basic configuration:

```yaml
images:
  - alias: "alias"
    imageName: "some/image:<tag_name>"
    commonUpdateSettings:
      updateStrategy: "digest"
```

This update strategy inspects a single tag in the registry for changes, and
updates the image on any change to the previous state. The tag name to be
inspected must be specified as a version constraint in the image list.

Use this update strategy if you want to follow a *mutable* tag, such as the
commonly used `latest` tag, or when your CI system produces a tag named as
the environment it is intended for, e.g. `dev` or `stage` or `prod`.

Argo CD Image Updater will then update the image when either

* The currently running image has a non-digest specification (e.g. uses a tag),
  or
* the currently used digest differs from what is found in the registry

For example, the following specification would always update the image for an
application on each new push of the image `some/image` with the tag `latest`:

```yaml
images:
  - alias: "myimage"
    imageName: "some/image:latest"
    commonUpdateSettings:
      updateStrategy: "digest"
```

### <a name="strategy-name"></a>Update according to lexical sort

!!!warning "Renamed image update strategies"
    The `name` strategy has been renamed to `alphabetical`.
    Please switch to the new convention as support for the old naming convention will be removed in future releases.
    Detected usage of `name` will result in a warning message within the image-updater controller logs.


Strategy name: `alphabetical` (deprecated alias: `name`)

Basic configuration:

```yaml
images:
  - alias: "alias"
    imageName: "some/image"
    commonUpdateSettings:
      updateStrategy: "alphabetical"
```

This update strategy sorts the tags returned by the registry in a lexical way
(by name, in descending order) and picks the last tag in the list for update.

!!!warning "Lexical sorting requires fixed width fields"
    Tags are compared byte by byte, so every numeric field must be zero padded to a constant width. With tags such as `2026-9-01` and `2026-10-03`, the unpadded `9` sorts above `10` and the image stays pinned to the older tag without any error being logged. For calendar versioned tags, prefer the [calver](#strategy-calver) strategy, which compares the fields numerically.

By default, this update strategy will inspect all of the tags it found in the
image's repository. If you wish to allow only certain tags to be considered
for update, you will need additional configuration. For example,


```yaml
images:
  - alias: "myimage"
    imageName: "some/image"
    commonUpdateSettings:
      updateStrategy: "alphabetical"
      allowTags: "regexp:^[0-9]{4}-[0-9]{2}-[0-9]{2}$"
```

would only consider tags that match a given regular expression for update. In
this case, only tags matching a date specification of `YYYY-MM-DD` would be
considered for update.

### <a name="strategy-calver"></a>calver - Update to the newest calendar version

Strategy name: `calver`

Basic configuration:

```yaml
images:
  - alias: "myimage"
    imageName: "some/image"
    commonUpdateSettings:
      updateStrategy: "calver"
```

This update strategy decodes each tag into the segments of a [calendar versioning](https://calver.org) scheme and picks the newest one.
Unlike [alphabetical](#strategy-name), the segments are compared as numbers, so `2026-9-01` correctly sorts below `2026-10-03` and a micro segment `10` sorts above `2`.

Tags that do not match the configured scheme are ignored, in the same way the `semver` strategy ignores tags that are not valid semantic versions. A repository that also holds a `latest` tag therefore needs no further configuration.

#### Tag layout

The scheme is given as the tag part of the image name and defaults to `YYYY.0M.0D`, the canonical scheme of calver.org. The default is zero padded on purpose: under a lenient layout, `2026.1.3` and `2026.01.03` decode to the same version, and a repository carrying both spellings would see its image rewritten back and forth between them. Any other scheme has to be spelled out:

```yaml
images:
  - alias: "myimage"
    imageName: "some/image:vYYYY-0M-0D"
    commonUpdateSettings:
      updateStrategy: "calver"
```

!!!warning "The tag is the layout, not the current version"
    Under this strategy the tag position holds the layout, so writing a concrete tag such as `some/image:v2026-01-30` there is a configuration error. Write `some/image:vYYYY-0M-0D` instead. Layouts are checked when the configuration is read, so a mistake is reported once, at startup, and the image is skipped rather than being looked up in the registry every cycle.

The tokens are the ones described at [calver.org](https://calver.org).
Everything else in the layout, such as the `v` prefix and the separators, has to appear in the tag verbatim.

| Token      | Matches                                                         |
| ---------- | --------------------------------------------------------------- |
| `YYYY`     | Full year, e.g. `2026`                                          |
| `YY`       | Short year counted from 2000, e.g. `6`, `26`, `106`             |
| `0Y`       | Zero padded short year, e.g. `06`, `26`, `106`                  |
| `MM`       | Short month, e.g. `1` ... `12`                                  |
| `0M`       | Zero padded month, e.g. `01` ... `12`                           |
| `WW`       | Short week of the year, e.g. `0` ... `53`                       |
| `0W`       | Zero padded week of the year, e.g. `00` ... `53`                |
| `DD`       | Short day, e.g. `1` ... `31`                                    |
| `0D`       | Zero padded day, e.g. `01` ... `31`                             |
| `MAJOR`    | Numeric major segment                                           |
| `MINOR`    | Numeric minor segment                                           |
| `MICRO`    | Numeric micro segment                                           |
| `MODIFIER` | Text segment, e.g. `rc1`, `alpha`, `beta`                       |

The zero padded tokens require the padding to be present, so `0M` matches `01` but not `1`. The short tokens are lenient and accept a field written either way, which is what you want for a repository whose tags are not padded consistently.

Short years count from the year 2000, so `6` is 2006, `26` is 2026 and `106` is 2106. Weeks follow the `%W` and `%U` conventions of `date`, which count the days before the first week day of the year as week `00`, so a tag produced by `date +%Y.%W` in early January is matched rather than dropped. A date that does not exist, such as `2026-02-31`, is rejected.

Because the layout is written where a tag would be, it also has to be a syntactically valid image tag: it may contain letters, digits, underscores, periods and dashes, and may not start with a period or a dash. Every layout shown here satisfies that.

#### How tags are ordered

Segments are compared in the order the layout writes them, so the layout decides what outranks what. The date counts as a single segment, placed where the layout first mentions it and compared by significance rather than by the order it is spelled in - a date means the same thing whether it is written `YYYY-0M-0D` or `0D-0M-YYYY`.

A scheme that leads with `MAJOR` therefore ranks the major segment above the date, exactly as calver.org describes it:

```
MAJOR.YY.0M    1.26.03  <  1.26.11  <  2.25.01
YY.0M.MAJOR    25.01.2  <  26.03.1  <  26.11.1
```

Because the date is compared as one unit, its fields have to be written next to each other. A layout such as `YY.MINOR.0M` is rejected, since there is no way to say whether the minor segment outranks the month or the other way round.

The freedom is in where the numbered segments sit relative to the date, not in their order among themselves: `MAJOR` always outranks `MINOR`, which always outranks `MICRO`, so a layout writing them the other way round is turned away as a typo.

A missing numeric segment counts as `0`, so `v26.4` ranks below `v26.4.1` under `vYY.MINOR.MICRO`.

#### Optional trailing segments

`MINOR`, `MICRO` and `MODIFIER` may be left out of a tag when the layout writes them at its very end, each separated from what comes before by a literal. This is what lets a dated tag and its rebuilds coexist:

```yaml
images:
  - alias: "myimage"
    imageName: "some/image:vYYYY-0M-0D-MICRO"
    commonUpdateSettings:
      updateStrategy: "calver"
```

```
v2026-01-30 < v2026-01-30-2 < v2026-01-30-10 < v2026-02-01
```

The same layout written without `MICRO` matches only `v2026-01-30`, and `v2026-01-30-2` is left out of consideration. Nothing is ever picked up that the layout does not mention.

Micro segments are compared numerically, so they do not need to be zero padded, and `v2026-01-30-1` and `v2026-01-30-01` denote the same version. Should a repository contain both spellings, the one sorting last by name is picked, so that the result stays stable across runs.

The same holds for the short tokens: under `MM`, the tags `2026-1-30` and `2026-01-30` decode to the same version. If a repository carries both, the updater may rewrite a running `2026-1-30` to `2026-01-30` even though the two denote the same day. Use the zero padded tokens, or `ignoreTags`, if you would rather that never happen.

#### Pre-releases

A tag carrying a `MODIFIER` is treated as a pre-release of the tag that does not, so it never outranks the plain release. Modifiers are compared in natural order among themselves, which puts `rc10` above `rc2`:

```
v2026-02-01-alpha < v2026-02-01-rc2 < v2026-02-01-rc10 < v2026-02-01
```

A modifier is only ever matched when the layout spells `MODIFIER` out, which mirrors how the `semver` strategy excludes pre-releases unless they are asked for. Under `vYYYY-0M-0D`, the tag `v2026-02-01-rc1` simply does not match and is never considered.

`MODIFIER` matches the remainder of the tag, so it has to come last in the layout.

!!!warning "MODIFIER is a pre-release marker, not a variant marker"
    Registries also use that same trailing position for image *variants*, as in `2026-01-30-alpine`, `-slim` or `-debian`. `MODIFIER` cannot tell those apart from `-rc1`, and since a plain release outranks anything carrying a modifier, a layout of `vYYYY-0M-0D-MODIFIER` will move an application running `v2026-01-30-alpine` onto the non-alpine `v2026-02-01`.

    To track a variant, write it into the layout as a literal and leave `MODIFIER` out:

    ```yaml
    images:
      - alias: "myimage"
        imageName: "some/image:vYYYY-0M-0D-alpine"
        commonUpdateSettings:
          updateStrategy: "calver"
    ```

    The `-alpine` suffix is part of the layout, so a tag without it does not match and is never a candidate. No `allowTags` is needed.

#### Tags without separators

When two fields are written next to each other, nothing marks where the first one ends, so only the fixed width tokens may be used in that position. The layout `YYYY0M0D` matches `20260130`, while `YYYYMMDD` is rejected because `MM` could consume one digit or two.

A token in that position also has to commit to a width, which makes `0Y` two digits rather than the two-or-three it accepts elsewhere. So `0Y0M0D` matches `260130` but not the year 2106's `1060130`, even though `0Y` on its own does match `106`. Use `YYYY0M0D` for a scheme that has to outlive 2099.

#### Prefixes

A layout that starts with a token accepts an optional `v` or `V` in front of the tag, so the default layout matches both `2026.01.30` and `v2026.01.30`.
Spelling the prefix out in the layout makes it mandatory, which is what you want when a repository holds more than one series of tags. A prefix spelled out in the layout is matched literally, so a layout of `vYYYY-0M-0D` matches `v2026-01-30` but not `V2026-01-30`.

#### Rejected layouts

Layouts are validated when they are read, and one that cannot be matched or cannot be ordered is reported as an error rather than silently matching
nothing. A layout is rejected when:

* it contains no year field, or defines the same field twice;
* it contains a day without a month, or combines a week with a month or a day;
* it splits the date around another segment, as in `YY.MINOR.0M`;
* it writes `MAJOR`, `MINOR` and `MICRO` out of order, as in `YYYY.MICRO.MINOR` - where they sit relative to the date is your choice, but they are names for the first, second and third number of a scheme, so their order among themselves is not;
* it places `MODIFIER` anywhere but last;
* it places a variable width token directly before another token;
* it contains an uppercase `Y`, `M`, `W`, `D` or a digit outside of a token, so that a mistyped `vYYYY-0M-D` is reported instead of being read as a literal `D`.

#### Testing a layout

The `test` command reads the layout out of the image name, exactly as the controller does, so a line can be copied straight out of the image list:

```
argocd-image-updater test some/image:vYYYY-0M-0D-MICRO --update-strategy calver
```

`--calver-layout` overrides it, which is handy for trying a layout against an image without editing the reference:

```
argocd-image-updater test some/image --update-strategy calver --calver-layout vYYYY-0M-0D-MICRO
```
