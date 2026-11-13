package argocd

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	jsonpatch "github.com/evanphx/json-patch/v5"
)

// ErrConcurrentSpecChange is returned by UpdateSpec when the live Application
// was changed by another client, since the baseline spec was read, in a field
// that the pending update also changes. Applying the update in that case would
// silently overwrite the other client's change, so the update is not written.
var ErrConcurrentSpecChange = errors.New("application spec was changed concurrently in a field that is being updated")

// computeSpecChanges returns the JSON encoding of original and a JSON merge
// patch holding the changes made in updated relative to original. If original
// is nil, no patch is computed.
func computeSpecChanges(original *v1alpha1.ApplicationSpec, updated *v1alpha1.ApplicationSpec) (originalJSON []byte, specPatch []byte, err error) {
	if original == nil {
		return nil, nil, nil
	}
	originalJSON, err = json.Marshal(original)
	if err != nil {
		return nil, nil, fmt.Errorf("could not marshal original spec: %w", err)
	}
	updatedJSON, err := json.Marshal(updated)
	if err != nil {
		return nil, nil, fmt.Errorf("could not marshal updated spec: %w", err)
	}
	specPatch, err = jsonpatch.CreateMergePatch(originalJSON, updatedJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("could not compute spec changes: %w", err)
	}
	return originalJSON, specPatch, nil
}

// rebaseSpecChanges applies specPatch, a JSON merge patch holding the changes
// made relative to the spec in originalJSON, on top of the live spec. It fails
// with ErrConcurrentSpecChange if the live spec differs from the original in
// any field that specPatch also changes.
func rebaseSpecChanges(originalJSON []byte, specPatch []byte, live *v1alpha1.ApplicationSpec) (*v1alpha1.ApplicationSpec, error) {
	liveJSON, err := json.Marshal(live)
	if err != nil {
		return nil, fmt.Errorf("could not marshal live spec: %w", err)
	}
	concurrentPatch, err := jsonpatch.CreateMergePatch(originalJSON, liveJSON)
	if err != nil {
		return nil, fmt.Errorf("could not compute concurrent spec changes: %w", err)
	}

	var ours, theirs map[string]any
	if err := json.Unmarshal(specPatch, &ours); err != nil {
		return nil, fmt.Errorf("could not unmarshal spec changes: %w", err)
	}
	if err := json.Unmarshal(concurrentPatch, &theirs); err != nil {
		return nil, fmt.Errorf("could not unmarshal concurrent spec changes: %w", err)
	}
	if path, conflict := mergePatchesConflict(ours, theirs, "spec"); conflict {
		return nil, fmt.Errorf("%w: %s", ErrConcurrentSpecChange, path)
	}

	patchedJSON, err := jsonpatch.MergePatch(liveJSON, specPatch)
	if err != nil {
		return nil, fmt.Errorf("could not apply spec changes: %w", err)
	}
	rebased := &v1alpha1.ApplicationSpec{}
	if err := json.Unmarshal(patchedJSON, rebased); err != nil {
		return nil, fmt.Errorf("could not unmarshal updated spec: %w", err)
	}
	return rebased, nil
}

// mergePatchesConflict reports whether two JSON merge patches computed against
// the same document change the same field to different values, and returns
// the path of the first such field. Arrays are replaced as a whole by a merge
// patch, so changes anywhere in the same array conflict.
func mergePatchesConflict(ours, theirs map[string]any, path string) (string, bool) {
	for key, ourValue := range ours {
		theirValue, ok := theirs[key]
		if !ok {
			continue
		}
		fieldPath := path + "." + key
		ourMap, ourIsMap := ourValue.(map[string]any)
		theirMap, theirIsMap := theirValue.(map[string]any)
		if ourIsMap && theirIsMap {
			if p, conflict := mergePatchesConflict(ourMap, theirMap, fieldPath); conflict {
				return p, true
			}
			continue
		}
		if !reflect.DeepEqual(ourValue, theirValue) {
			return fieldPath, true
		}
	}
	return "", false
}
