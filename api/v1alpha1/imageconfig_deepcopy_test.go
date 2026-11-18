/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import "testing"

// A copy of an ImageConfig must not share DeployedImageName with the original,
// so that changing one does not change the other.
func TestImageConfigDeepCopy_DeployedImageName(t *testing.T) {
	name := "mirror.example/team/app"
	in := &ImageConfig{Alias: "app", ImageName: "team/app", DeployedImageName: &name}

	out := in.DeepCopy()

	if out.DeployedImageName == nil || *out.DeployedImageName != name {
		t.Fatalf("DeployedImageName = %v, want %q", out.DeployedImageName, name)
	}
	if out.DeployedImageName == in.DeployedImageName {
		t.Fatal("DeployedImageName is shared between the copy and the original")
	}
}
