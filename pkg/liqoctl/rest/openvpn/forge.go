// Copyright 2019-2026 The Liqo Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package openvpn

import (
	"encoding/base64"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ForgeServerSecret creates a Kubernetes Secret containing server OpenVPN certificates.
func ForgeServerSecret(namespace, name string, bundle *CertificateBundle) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"ca.crt":     bundle.CACert,
			"server.crt": bundle.ServerCert,
			"server.key": bundle.ServerKey,
			"dh.pem":     bundle.DHParams,
			"ta.key":     bundle.TLSAuthKey,
		},
	}
}

// ForgeClientSecret creates a Kubernetes Secret containing client OpenVPN certificates.
func ForgeClientSecret(namespace, name string, bundle *CertificateBundle) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"ca.crt":     bundle.CACert,
			"client.crt": bundle.ClientCert,
			"client.key": bundle.ClientKey,
			"ta.key":     bundle.TLSAuthKey,
		},
	}
}

// SecretDataBase64 returns the secret data with all values base64 encoded.
// This is useful for displaying secrets in manifests.
func SecretDataBase64(data map[string][]byte) map[string]string {
	encoded := make(map[string]string)
	for k, v := range data {
		encoded[k] = base64.StdEncoding.EncodeToString(v)
	}
	return encoded
}
