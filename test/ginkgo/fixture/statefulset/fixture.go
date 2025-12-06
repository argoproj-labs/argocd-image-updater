package statefulset

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

// Update will keep trying to update object until it succeeds, or times out.
func Update(obj *appsv1.StatefulSet, modify func(*appsv1.StatefulSet)) {
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

func HaveReplicas(replicas int) matcher.GomegaMatcher {
	return fetchStatefulSet(func(ss *appsv1.StatefulSet) bool {
		GinkgoWriter.Println("StatefulSet HaveReplicas:", "expected: ", replicas, "actual: ", ss.Status.Replicas)
		return int(ss.Status.Replicas) == replicas && ss.Generation == ss.Status.ObservedGeneration
	})
}

func HaveReadyReplicas(readyReplicas int) matcher.GomegaMatcher {
	return fetchStatefulSet(func(ss *appsv1.StatefulSet) bool {
		GinkgoWriter.Println("StatefulSet HaveReadyReplicas:", "expected: ", readyReplicas, "actual: ", ss.Status.ReadyReplicas)
		return int(ss.Status.ReadyReplicas) == readyReplicas && ss.Generation == ss.Status.ObservedGeneration
	})
}

func GetTemplateSpecInitContainerByName(name string, ss appsv1.StatefulSet) *corev1.Container {

	for idx := range ss.Spec.Template.Spec.InitContainers {

		container := ss.Spec.Template.Spec.InitContainers[idx]
		if container.Name == name {
			return &container
		}
	}

	return nil
}

func GetTemplateSpecContainerByName(name string, ss appsv1.StatefulSet) *corev1.Container {

	for idx := range ss.Spec.Template.Spec.Containers {

		container := ss.Spec.Template.Spec.Containers[idx]
		if container.Name == name {
			return &container
		}
	}

	return nil
}

func HaveTemplateSpecNodeSelector(nodeSelector map[string]string) matcher.GomegaMatcher {
	return fetchStatefulSet(func(ss *appsv1.StatefulSet) bool {

		templateSpec := ss.Spec.Template.Spec

		if templateSpec.NodeSelector == nil {
			GinkgoWriter.Println("HaveTemplateSpecNodeSelector - .spec.template.spec is nil")
			return false
		}

		GinkgoWriter.Println("HaveTemplateSpecNodeSelector - expected:", nodeSelector, "actual:", templateSpec.NodeSelector)
		return reflect.DeepEqual(nodeSelector, templateSpec.NodeSelector)
	})

}

func HaveTemplateLabelWithValue(key string, value string) matcher.GomegaMatcher {

	return fetchStatefulSet(func(ss *appsv1.StatefulSet) bool {
		k8sClient, _, err := utils.GetE2ETestKubeClientWithError()
		if err != nil {
			GinkgoWriter.Println(err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(ss), ss)
		if err != nil {
			GinkgoWriter.Println("HaveTemplateLabelWithValue:", err)
			return false
		}

		labels := ss.Spec.Template.Labels
		if labels == nil {
			GinkgoWriter.Println("HaveTemplateLabelWithValue - labels are nil")
			return false
		}

		GinkgoWriter.Println("HaveTemplateLabelWithValue - Key", key, "Expect:", value, "/ Have:", labels[key])

		return labels[key] == value

	})
}

func HaveTemplateAnnotationWithValue(key string, value string) matcher.GomegaMatcher {

	return fetchStatefulSet(func(ss *appsv1.StatefulSet) bool {
		k8sClient, _, err := utils.GetE2ETestKubeClientWithError()
		if err != nil {
			GinkgoWriter.Println(err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(ss), ss)
		if err != nil {
			GinkgoWriter.Println("HaveTemplateAnnotationWithValue:", err)
			return false
		}

		annotations := ss.Spec.Template.Annotations
		if annotations == nil {
			GinkgoWriter.Println("HaveTemplateAnnotationWithValue - annotations are nil")
			return false
		}

		GinkgoWriter.Println("HaveTemplateAnnotationWithValue - Key", key, "Expect:", value, "/ Have:", annotations[key])

		return annotations[key] == value

	})
}

func HaveTolerations(tolerations []corev1.Toleration) matcher.GomegaMatcher {
	return fetchStatefulSet(func(ss *appsv1.StatefulSet) bool {

		templateSpec := ss.Spec.Template.Spec
		GinkgoWriter.Println("HaveTolerations - expected:", tolerations, "actual:", templateSpec.Tolerations)

		return reflect.DeepEqual(templateSpec.Tolerations, tolerations)
	})
}

func HaveContainerImage(containerImage string, containerIndex int) matcher.GomegaMatcher {
	return fetchStatefulSet(func(ss *appsv1.StatefulSet) bool {

		containers := ss.Spec.Template.Spec.Containers

		if len(containers) <= containerIndex {
			GinkgoWriter.Println("current container slice has length", len(containers), "index is", containerIndex)
			return false
		}

		return containers[containerIndex].Image == containerImage

	})
}

func NotHaveContainerImage(containerImage string, containerIndex int) matcher.GomegaMatcher {
	return fetchStatefulSet(func(ss *appsv1.StatefulSet) bool {

		containers := ss.Spec.Template.Spec.Containers

		if len(containers) <= containerIndex {
			GinkgoWriter.Println("current container slice has length", len(containers), "index is", containerIndex)
			return false
		}

		GinkgoWriter.Println("NotHaveContainerImage - expected:", containerImage, "actual:", containers[containerIndex].Image)

		return containers[containerIndex].Image != containerImage

	})
}

func HaveContainerCommandSubstring(expectedCommandSubstring string, containerIndex int) matcher.GomegaMatcher {
	return fetchStatefulSet(func(ss *appsv1.StatefulSet) bool {

		containers := ss.Spec.Template.Spec.Containers

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

func HaveContainerWithEnvVar(envKey string, envValue string, containerIndex int) matcher.GomegaMatcher {
	return fetchStatefulSet(func(ss *appsv1.StatefulSet) bool {

		containers := ss.Spec.Template.Spec.Containers

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

// This is intentionally NOT exported, for now. Create another function in this file/package that calls this function, and export that.
func fetchStatefulSet(f func(*appsv1.StatefulSet) bool) matcher.GomegaMatcher {

	return WithTransform(func(ss *appsv1.StatefulSet) bool {

		k8sClient, _, err := utils.GetE2ETestKubeClientWithError()
		if err != nil {
			GinkgoWriter.Println("fetchStatefulSet:", err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(ss), ss)
		if err != nil {
			GinkgoWriter.Println("fetchStatefulSet:", err)
			return false
		}

		return f(ss)

	}, BeTrue())

}
