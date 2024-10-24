# ApplicationSet considerations

If using an `ApplicationSet` instead of `Application`, there are a few things to consider. Take this `ApplicationSet` manifest as example:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: vitrine
  namespace: argocd
spec:
  ignoreApplicationDifferences:
    - jsonPointers:
      - /spec/source/helm/parameters
  goTemplate: true
  goTemplateOptions: ["missingkey=error"]
  generators:
  - matrix:
      generators:
      - list:
          elements:
          - name: vitrine
      - list:
          elements:
          - branch: main
          - branch: develop
  template:
    metadata:
      name: "{{ printf `%s-%s` `{{ .name }}` `{{ .branch }}` }}"
      namespace: argocd
      annotations:
        #
        argocd-image-updater.argoproj.io/image-list: "{{ `{{ .name }}` }}=docker.io/myuser/{{ `{{ .name }}` }}:{{ `{{ .branch }}` }}"
        argocd-image-updater.argoproj.io/{{ `{{ .name }}` }}.helm.image-name: config.image.name
        argocd-image-updater.argoproj.io/{{ `{{ .name }}` }}.helm.image-tag: config.image.tag
        argocd-image-updater.argoproj.io/{{ `{{ .name }}` }}.update-strategy: digest
    spec:
      destination:
        server: {{ quote .Values.spec.destination.server }}
        namespace: "{{ printf `%s-%s` .Values.sharedConfig.namespaceBase `{{ .branch }}` }}"
      project: default
      syncPolicy:
        automated: {}
        syncOptions:
        - CreateNamespace=true
      source:
        path: "{{ printf `%s/%s` .Values.spec.source.repo.paths.charts `{{ .name }}` }}"
        repoURL: {{ include "repoNameFullName" . }}
        targetRevision: HEAD
        helm:
          valueFiles:
          - values.yaml
          valuesObject:
            config:
              image:
                name: "docker.io/myuser//{{ `{{ .name }}` }}"
                tag: "{{ `{{ .branch }}` }}"
```

## Automatic patching of children `Application`s

By default, `ApplicationSet` prevents any manual modification of spawned `Application`. For example, if you try to update a child `Application` through the UI, the modification will be instantly reverted; this is an expected behavior. 
Theses rejected manual modifications sadly include Image Updater's `spec.template.spec.source.helm.parameters` `Application` automatic patching, which will prevent `argocd-image-updater.argoproj.io/write-back-method: argocd` implicit behavior to function properly.

In this case, you will still receive `ImagesUpdated` events on your affected `Application`, but Image Updater will continously retry again, to no avail.

To allow Image Updater to work here, you need to add this to your `ApplicationSet`:

```yaml
spec:
  ignoreApplicationDifferences:
    - jsonPointers:
      - /spec/source/helm/parameters
```

This will prevent automatic revert behavior of `ApplicationSet` if any of the defined `jsonPointers` subjacent values are altered on children `Application`.

Please mind there are also a few caveats to consider. (see https://argo-cd.readthedocs.io/en/stable/operator-manual/applicationset/Controlling-Resource-Modification/#limitations-of-ignoreapplicationdifferences)
