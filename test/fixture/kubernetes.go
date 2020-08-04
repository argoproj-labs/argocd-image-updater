package fixture

import (
	"encoding/json"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewSecret(namespace, name string, entries map[string][]byte) *v1.Secret {
	secret := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Data: entries,
	}
	return &secret
}

func MustCreateSecretFromFile(filepath string) *v1.Secret {
	jsonData := MustReadFile(filepath)
	return MustCreateSecretFromJson(jsonData)
}

func MustCreateSecretFromJson(jsonData string) *v1.Secret {
	var s v1.Secret
	err := json.Unmarshal([]byte(jsonData), &s)
	if err != nil {
		panic(err)
	}
	return &s
}
