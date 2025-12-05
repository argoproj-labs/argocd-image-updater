package deployment

import (
	"context"
	"reflect"
	"strings"

	//lint:ignore ST1001 "This is a common practice in Gomega tests for readability."
	. "github.com/onsi/ginkgo/v2" //nolint:all
	//lint:ignore ST1001 "This is a common practice in Gomega tests for readability."
	. "github.com/onsi/gomega" //nolint:all

	matcher "github.com/onsi/gomega/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/retry"
)

func GetEnv(d *appsv1.Deployment, key string) (*string, error) {

	k8sClient, _, err := utils.GetE2ETestKubeClientWithError()
	if err != nil {
		return nil, err
	}

	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(d), d); err != nil {
		return nil, err
	}

	containers := d.Spec.Template.Spec.Containers

	Expect(containers).Should(HaveLen(1))

	for idx := range containers[0].Env {

		currEnv := containers[0].Env[idx]

		if currEnv.Name == key {
			return &currEnv.Value, nil
		}
	}

	return nil, nil

}

func SetEnv(depl *appsv1.Deployment, key string, value string) {

	Update(depl, func(d *appsv1.Deployment) {
		containers := d.Spec.Template.Spec.Containers

		Expect(containers).Should(HaveLen(1))

		newEnvVars := []corev1.EnvVar{}

		match := false
		for idx := range containers[0].Env {

			currEnv := containers[0].Env[idx]

			if currEnv.Name == key {
				// replace with the value from the param
				newEnvVars = append(newEnvVars, corev1.EnvVar{Name: key, Value: value})
				match = true
			} else {
				newEnvVars = append(newEnvVars, currEnv)
			}
		}

		if !match {
			newEnvVars = append(newEnvVars, corev1.EnvVar{Name: key, Value: value})
		}

		containers[0].Env = newEnvVars

	})

}

func RemoveEnv(depl *appsv1.Deployment, key string) {

	Update(depl, func(d *appsv1.Deployment) {
		containers := d.Spec.Template.Spec.Containers

		Expect(containers).Should(HaveLen(1))

		newEnvVars := []corev1.EnvVar{}

		for idx := range containers[0].Env {

			currEnv := containers[0].Env[idx]

			if currEnv.Name == key {
				// don't add, thus causing it to be removed
			} else {
				newEnvVars = append(newEnvVars, currEnv)
			}
		}

		containers[0].Env = newEnvVars

	})

}

// Update will keep trying to update object until it succeeds, or times out.
func Update(obj *appsv1.Deployment, modify func(*appsv1.Deployment)) {
	k8sClient, _ := utils.GetE2ETestKubeClient()

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version of the object
		err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)
		if err != nil {
			return err
		}

		modify(obj)

		// Attempt to update the object
		return k8sClient.Update(context.Background(), obj)
	})
	Expect(err).ToNot(HaveOccurred())
}

func GetTemplateSpecInitContainerByName(name string, depl appsv1.Deployment) *corev1.Container {

	for idx := range depl.Spec.Template.Spec.InitContainers {

		container := depl.Spec.Template.Spec.InitContainers[idx]
		if container.Name == name {
			return &container
		}
	}

	return nil
}

func GetTemplateSpecContainerByName(name string, depl appsv1.Deployment) *corev1.Container {

	for idx := range depl.Spec.Template.Spec.Containers {

		container := depl.Spec.Template.Spec.Containers[idx]
		if container.Name == name {
			return &container
		}
	}

	return nil
}

func HaveTemplateSpec(podSpec corev1.PodSpec) matcher.GomegaMatcher {
	return fetchDeployment(func(depl *appsv1.Deployment) bool {
		templateSpec := depl.Spec.Template.Spec
		if templateSpec.NodeSelector == nil {
			GinkgoWriter.Println("HaveTemplateSpec - .spec.template.spec.NodeSelector is nil")
			return false
		}
		GinkgoWriter.Println("HaveTemplateSpec - expected:", podSpec, "actual:", templateSpec)
		return reflect.DeepEqual(podSpec, templateSpec)
	})
}

func HaveTemplateSpecNodeSelector(nodeSelector map[string]string) matcher.GomegaMatcher {
	return fetchDeployment(func(depl *appsv1.Deployment) bool {

		templateSpec := depl.Spec.Template.Spec

		if templateSpec.NodeSelector == nil {
			GinkgoWriter.Println("HaveTemplateSpecNodeSelector - .spec.template.spec.NodeSelector is nil")
			return false
		}

		GinkgoWriter.Println("HaveTemplateSpecNodeSelector - expected:", nodeSelector, "actual:", templateSpec.NodeSelector)
		return reflect.DeepEqual(nodeSelector, templateSpec.NodeSelector)
	})

}

func HaveTemplateLabelWithValue(key string, value string) matcher.GomegaMatcher {

	return WithTransform(func(depl *appsv1.Deployment) bool {
		k8sClient, _, err := utils.GetE2ETestKubeClientWithError()
		if err != nil {
			GinkgoWriter.Println(err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(depl), depl)
		if err != nil {
			GinkgoWriter.Println("HaveTemplateLabelWithValue:", err)
			return false
		}

		labels := depl.Spec.Template.Labels
		if labels == nil {
			GinkgoWriter.Println("HaveTemplateLabelWithValue - labels are nil")
			return false
		}

		GinkgoWriter.Println("HaveTemplateLabelWithValue - Key", key, "Expect:", value, "/ Have:", labels[key])

		return labels[key] == value

	}, BeTrue())
}

func HaveTemplateAnnotationWithValue(key string, value string) matcher.GomegaMatcher {

	return WithTransform(func(depl *appsv1.Deployment) bool {
		k8sClient, _, err := utils.GetE2ETestKubeClientWithError()
		if err != nil {
			GinkgoWriter.Println(err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(depl), depl)
		if err != nil {
			GinkgoWriter.Println("HaveTemplateAnnotationWithValue:", err)
			return false
		}

		annotations := depl.Spec.Template.Annotations
		if annotations == nil {
			GinkgoWriter.Println("HaveTemplateAnnotationWithValue - annotations are nil")
			return false
		}

		GinkgoWriter.Println("HaveTemplateAnnotationWithValue - Key", key, "Expect:", value, "/ Have:", annotations[key])

		return annotations[key] == value

	}, BeTrue())
}

func HaveTolerations(tolerations []corev1.Toleration) matcher.GomegaMatcher {
	return fetchDeployment(func(depl *appsv1.Deployment) bool {

		templateSpec := depl.Spec.Template.Spec

		GinkgoWriter.Println("HaveTolerations - expected:", tolerations, "actual:", templateSpec.Tolerations)

		return reflect.DeepEqual(templateSpec.Tolerations, tolerations)
	})

}

func HaveObservedGeneration(observedGeneration int) matcher.GomegaMatcher {
	return fetchDeployment(func(depl *appsv1.Deployment) bool {
		GinkgoWriter.Println("Deployment HaveObservedGeneration:", "expected: ", observedGeneration, "actual: ", depl.Status.ObservedGeneration)
		return int64(observedGeneration) == depl.Status.ObservedGeneration
	})
}

func HaveReplicas(replicas int) matcher.GomegaMatcher {
	return fetchDeployment(func(depl *appsv1.Deployment) bool {
		GinkgoWriter.Println("Deployment", depl.Name, "- HaveReplicas:", "expected: ", replicas, "actual: ", depl.Status.Replicas)
		return int(depl.Status.Replicas) == replicas && depl.Generation == depl.Status.ObservedGeneration
	})
}

func HaveReadyReplicas(readyReplicas int) matcher.GomegaMatcher {
	return fetchDeployment(func(depl *appsv1.Deployment) bool {
		GinkgoWriter.Println("Deployment ", depl.Name, "- HaveReadyReplicas:", "expected: ", readyReplicas, "actual: ", depl.Status.ReadyReplicas)
		return int(depl.Status.ReadyReplicas) == readyReplicas && depl.Generation == depl.Status.ObservedGeneration
	})
}

func HaveUpdatedReplicas(updatedReplicas int) matcher.GomegaMatcher {
	return fetchDeployment(func(depl *appsv1.Deployment) bool {
		GinkgoWriter.Println("Deployment HaveUpdatedReplicas:", "expected: ", updatedReplicas, "actual: ", depl.Status.UpdatedReplicas)
		return int(depl.Status.UpdatedReplicas) == updatedReplicas && depl.Generation == depl.Status.ObservedGeneration
	})
}

func HaveAvailableReplicas(availableReplicas int) matcher.GomegaMatcher {
	return fetchDeployment(func(depl *appsv1.Deployment) bool {
		GinkgoWriter.Println("Deployment HaveAvailableReplicas:", "expected: ", availableReplicas, "actual: ", depl.Status.AvailableReplicas)
		return int(depl.Status.AvailableReplicas) == availableReplicas && depl.Generation == depl.Status.ObservedGeneration
	})
}

func HaveContainerCommandSubstring(expectedCommandSubstring string, containerIndex int) matcher.GomegaMatcher {
	return fetchDeployment(func(depl *appsv1.Deployment) bool {

		containers := depl.Spec.Template.Spec.Containers

		if len(containers) <= containerIndex {
			GinkgoWriter.Println("current container slice has length", len(containers), "index is", containerIndex)
			return false
		}

		// Combine Command and Args, adding spaces (' ') between the args
		var cmdLine string

		for _, val := range containers[containerIndex].Command {
			if val == "" {
				cmdLine += "\"\"" + " "
			} else {
				cmdLine += val + " "
			}
		}
		cmdLine = strings.TrimSpace(cmdLine)

		for _, val := range containers[containerIndex].Args {
			if val == "" {
				cmdLine += "\"\"" + " "
			} else {
				cmdLine += val + " "
			}
		}
		cmdLine = strings.TrimSpace(cmdLine)

		GinkgoWriter.Println("HaveContainerCommandSubstring: Have:")
		GinkgoWriter.Println(cmdLine)
		GinkgoWriter.Println("HaveContainerCommandSubstring: Expect:")
		GinkgoWriter.Println(expectedCommandSubstring)

		return strings.Contains(cmdLine, expectedCommandSubstring)

	})
}

// HaveContainerWithEnvVar matchs when a container exists under .spec.template.spec.containers, at position 'containerIndex' in the container array, with env var key/value
func HaveContainerWithEnvVar(envKey string, envValue string, containerIndex int) matcher.GomegaMatcher {
	return fetchDeployment(func(depl *appsv1.Deployment) bool {

		containers := depl.Spec.Template.Spec.Containers

		if len(containers) <= containerIndex {
			GinkgoWriter.Println("current container slice has length", len(containers), "index is", containerIndex)
			return false
		}

		container := containers[containerIndex]

		for _, env := range container.Env {
			if env.Name == envKey {
				GinkgoWriter.Println("HaveContainerWithEnvVar - Key ", envKey, " Expected:", envValue, "Actual:", env.Value)
				if env.Value == envValue {
					return true
				}
			}
		}

		return false
	})
}

func HaveSpecTemplateSpecVolume(volumeParam corev1.Volume) matcher.GomegaMatcher {
	return fetchDeployment(func(depl *appsv1.Deployment) bool {

		GinkgoWriter.Println("HaveSpecTemplateSpecVolume - Volumes:")
		for _, volume := range depl.Spec.Template.Spec.Volumes {
			GinkgoWriter.Println("-", volume)

			if reflect.DeepEqual(volumeParam, volume) {
				return true
			}
		}

		return false
	})

}

func HaveConditionTypeStatus(expectedConditionType appsv1.DeploymentConditionType, expectedConditionStatus corev1.ConditionStatus) matcher.GomegaMatcher {
	return fetchDeployment(func(depl *appsv1.Deployment) bool {

		GinkgoWriter.Println("Conditions:")
		for _, condition := range depl.Status.Conditions {
			GinkgoWriter.Println("-", condition.Type, condition.Status)
			if condition.Type == expectedConditionType && condition.Status == expectedConditionStatus {
				return true
			}
		}

		return false
	})
}

func HaveServiceAccountName(expectedServiceAccountName string) matcher.GomegaMatcher {
	return fetchDeployment(func(depl *appsv1.Deployment) bool {

		GinkgoWriter.Println("HaveServiceAccountName - Expected:", expectedServiceAccountName, "Actual:", depl.Spec.Template.Spec.ServiceAccountName, "/", depl.Spec.Template.Spec.DeprecatedServiceAccount)

		// We check both the deprecated and non-deprecated names, as these can POTENTIALLY be different values.
		// - They should both have the same value 'expectedServiceAccountName'

		return depl.Spec.Template.Spec.ServiceAccountName == expectedServiceAccountName && depl.Spec.Template.Spec.DeprecatedServiceAccount == expectedServiceAccountName
	})
}

// This is intentionally NOT exported, for now. Create another function in this file/package that calls this function, and export that.
func fetchDeployment(f func(*appsv1.Deployment) bool) matcher.GomegaMatcher {

	return WithTransform(func(depl *appsv1.Deployment) bool {

		k8sClient, _, err := utils.GetE2ETestKubeClientWithError()
		if err != nil {
			GinkgoWriter.Println(err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(depl), depl)
		if err != nil {
			GinkgoWriter.Println(err)
			return false
		}

		return f(depl)

	}, BeTrue())

}
