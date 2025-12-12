package configmap

import (
	"context"
	"strings"

	"github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/utils"
	//lint:ignore ST1001 "This is a common practice in Gomega tests for readability."
	. "github.com/onsi/ginkgo/v2" //nolint:all
	//lint:ignore ST1001 "This is a common practice in Gomega tests for readability."
	. "github.com/onsi/gomega" //nolint:all

	matcher "github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Update will keep trying to update object until it succeeds, or times out.
func Update(obj *corev1.ConfigMap, modify func(*corev1.ConfigMap)) {
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

// HaveStringDataKeyValue returns true if ConfigMap has 'key' field under .data map, and the value of that field is equal to 'value'
// e.g.
//
// kind: ConfigMap
// metadata:
//
//	(...)
//
// data:
//
//	"key": "value"
func HaveStringDataKeyValue(key string, value string) matcher.GomegaMatcher {
	return fetchConfigMap(func(cm *corev1.ConfigMap) bool {
		a, exists := cm.Data[key]
		if !exists {
			GinkgoWriter.Println("HaveStringDataKeyValue: ConfigMap key", key, "does not exist.")
			return false
		}

		// Remove leading and trailing whitespace
		a = strings.TrimSpace(a)
		value = strings.TrimSpace(value)

		if strings.Contains(value, "\n") {
			GinkgoWriter.Println("HaveStringDataKeyValue: ConfigMag key", key)
			GinkgoWriter.Println("Have:")
			GinkgoWriter.Println("|" + a + "|")
			GinkgoWriter.Println("Expected:")
			GinkgoWriter.Println("|" + value + "|")
		} else {
			GinkgoWriter.Println("HaveStringDataKeyValue: ConfigMag key", key, "Have:", a, "Expected:", value)
		}

		return a == value
	})

}

// NotHaveStringDataKey returns true if ConfigMap's .data 'key' does not exist, false otherwise
func NotHaveStringDataKey(key string) matcher.GomegaMatcher {
	return fetchConfigMap(func(cm *corev1.ConfigMap) bool {
		_, exists := cm.Data[key]
		return !exists
	})

}

// HaveNonEmptyDataKey returns true if ConfigMap has 'key' field under .data map, and the value of that field is equal to 'value'
func HaveNonEmptyDataKey(key string) matcher.GomegaMatcher {
	return fetchConfigMap(func(cm *corev1.ConfigMap) bool {
		a, exists := cm.Data[key]
		if !exists {
			GinkgoWriter.Println("HaveStringDataKeyValue: ConfigMap key", key, "does not exist.")
			return false
		}
		return len(strings.TrimSpace(a)) > 0
	})

}

// HaveStringDataKeyValueContainsSubstring returns true if ConfigMap has 'key' field under .data map, and the value of that field contains a specific substring
func HaveStringDataKeyValueContainsSubstring(key string, substring string) matcher.GomegaMatcher {
	return fetchConfigMap(func(cm *corev1.ConfigMap) bool {
		a, exists := cm.Data[key]
		if !exists {
			GinkgoWriter.Println("HaveStringDataKeyValueContainsSubstring: ConfigMap key", key, "does not exist.")
			return false
		}

		// Remove leading and trailing whitespace
		a = strings.TrimSpace(a)
		substring = strings.TrimSpace(substring)

		if strings.Contains(substring, "\n") {
			GinkgoWriter.Println("HaveStringDataKeyValue: ConfigMag key", key)
			GinkgoWriter.Println("Value:")
			GinkgoWriter.Println("|" + a + "|")
			GinkgoWriter.Println("Expected:")
			GinkgoWriter.Println("|" + substring + "|")
		} else {
			GinkgoWriter.Println("HaveStringDataKeyValue: ConfigMag key", key, "Value:", a, "Expected:", substring)
		}

		return strings.Contains(a, substring)
	})

}

// This is intentionally NOT exported, for now. Create another function in this file/package that calls this function, and export that.
func fetchConfigMap(f func(*corev1.ConfigMap) bool) matcher.GomegaMatcher {

	return WithTransform(func(configMap *corev1.ConfigMap) bool {

		k8sClient, _, err := utils.GetE2ETestKubeClientWithError()
		if err != nil {
			GinkgoWriter.Println(err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(configMap), configMap)
		if err != nil {
			GinkgoWriter.Println(err)
			return false
		}

		return f(configMap)

	}, BeTrue())

}
