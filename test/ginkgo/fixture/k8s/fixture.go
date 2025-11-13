package k8s

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/client-go/util/retry"

	//lint:ignore ST1001 "This is a common practice in Gomega tests for readability."
	. "github.com/onsi/ginkgo/v2" //nolint:all
	//lint:ignore ST1001 "This is a common practice in Gomega tests for readability."
	. "github.com/onsi/gomega" //nolint:all

	matcher "github.com/onsi/gomega/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/utils"
)

func HaveAnnotationWithValue(key string, value string) matcher.GomegaMatcher {

	return WithTransform(func(k8sObject client.Object) bool {
		k8sClient, _, err := utils.GetE2ETestKubeClientWithError()
		if err != nil {
			GinkgoWriter.Println(err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(k8sObject), k8sObject)
		if err != nil {
			GinkgoWriter.Println("HasAnnotationWithValue:", err)
			return false
		}

		annotations := k8sObject.GetAnnotations()
		if annotations == nil {
			return false
		}

		return annotations[key] == value

	}, BeTrue())
}

func HaveLabelWithValue(key string, value string) matcher.GomegaMatcher {

	return WithTransform(func(k8sObject client.Object) bool {
		k8sClient, _, err := utils.GetE2ETestKubeClientWithError()
		if err != nil {
			GinkgoWriter.Println(err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(k8sObject), k8sObject)
		if err != nil {
			GinkgoWriter.Println("HaveLabelWithValue:", err)
			return false
		}

		labels := k8sObject.GetLabels()
		if labels == nil {
			GinkgoWriter.Println("HaveLabelWithValue - labels are nil")
			return false
		}

		GinkgoWriter.Println("HaveLabelWithValue - Key", key, "Expect:", value, "/ Have:", labels[key])

		return labels[key] == value

	}, BeTrue())
}

func NotHaveLabelWithValue(key string, value string) matcher.GomegaMatcher {

	return WithTransform(func(k8sObject client.Object) bool {
		k8sClient, _, err := utils.GetE2ETestKubeClientWithError()
		if err != nil {
			GinkgoWriter.Println(err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(k8sObject), k8sObject)
		if err != nil {
			GinkgoWriter.Println("DoesNotHaveLabelWithValue:", err)
			return false
		}

		labels := k8sObject.GetLabels()
		if labels == nil {
			return true
		}

		return labels[key] != value

	}, BeTrue())
}

// ExistByName checks if the given k8s resource exists, when retrieving it by name/namespace.
// - It does NOT check if the resource content matches. It only checks that a resource of that type and name exists.
func ExistByName() matcher.GomegaMatcher {

	return WithTransform(func(k8sObject client.Object) bool {
		k8sClient, _, err := utils.GetE2ETestKubeClientWithError()
		if err != nil {
			GinkgoWriter.Println(err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(k8sObject), k8sObject)
		if err != nil {
			GinkgoWriter.Println("Object does not exist in ExistByName:", k8sObject.GetName(), err)
		} else {
			GinkgoWriter.Println("Object exists in ExistByName:", k8sObject.GetName())
		}
		return err == nil
	}, BeTrue())
}

// NotExistByName checks if the given resource does not exist, when retrieving it by name/namespace.
// Does NOT check if the resource content matches.
func NotExistByName() matcher.GomegaMatcher {

	return WithTransform(func(k8sObject client.Object) bool {
		k8sClient, _, err := utils.GetE2ETestKubeClientWithError()
		if err != nil {
			GinkgoWriter.Println(err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(k8sObject), k8sObject)
		if apierrors.IsNotFound(err) {
			return true
		} else {
			if err != nil {
				GinkgoWriter.Println(err)
			}
			return false
		}
	}, BeTrue())
}

// Update will keep trying to update object until it succeeds, or times out.
func Update(obj client.Object, modify func(client.Object)) {
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
