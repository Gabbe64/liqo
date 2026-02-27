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
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// EnsureGatewaySecrets creates the OpenVPN server and client secrets if both are missing.
// If only one secret exists, it returns an error to avoid mismatched credentials.
func EnsureGatewaySecrets(ctx context.Context,
	serverClient, clientClient kubernetes.Interface,
	serverNamespace, clientNamespace string,
	serverSecretName, clientSecretName string,
) error {
	if serverNamespace == "" || clientNamespace == "" {
		return fmt.Errorf("openvpn secret namespaces must be set")
	}
	if serverSecretName == "" || clientSecretName == "" {
		return fmt.Errorf("openvpn secret names must be set")
	}

	serverExists, err := secretExists(ctx, serverClient, serverNamespace, serverSecretName)
	if err != nil {
		return fmt.Errorf("unable to check server secret: %w", err)
	}
	clientExists, err := secretExists(ctx, clientClient, clientNamespace, clientSecretName)
	if err != nil {
		return fmt.Errorf("unable to check client secret: %w", err)
	}

	if serverExists && clientExists {
		return nil
	}
	if serverExists || clientExists {
		return fmt.Errorf("openvpn secrets must be created together; delete existing secret(s) and retry")
	}

	bundle, err := GenerateCertificateBundle()
	if err != nil {
		return fmt.Errorf("failed to generate openvpn certificate bundle: %w", err)
	}

	serverSecret := ForgeServerSecret(serverNamespace, serverSecretName, bundle)
	if _, err := serverClient.CoreV1().Secrets(serverNamespace).Create(ctx, serverSecret, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create openvpn server secret: %w", err)
	}

	clientSecret := ForgeClientSecret(clientNamespace, clientSecretName, bundle)
	if _, err := clientClient.CoreV1().Secrets(clientNamespace).Create(ctx, clientSecret, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create openvpn client secret: %w", err)
	}

	return nil
}

// SetSecretOwnerReference sets an owner reference on an existing OpenVPN secret.
// This enables automatic garbage collection when the owner resource (GatewayClient or GatewayServer) is deleted.
func SetSecretOwnerReference(ctx context.Context,
	kubeClient kubernetes.Interface,
	scheme *runtime.Scheme,
	namespace string,
	secretName string,
	owner client.Object,
) error {
	if namespace == "" {
		return fmt.Errorf("openvpn secret namespace must be set")
	}
	if secretName == "" {
		return fmt.Errorf("openvpn secret name must be set")
	}
	if scheme == nil {
		return fmt.Errorf("openvpn scheme must be set")
	}
	if owner == nil {
		return fmt.Errorf("openvpn owner object must be set")
	}

	secret, err := kubeClient.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get secret: %w", err)
	}

	if err := controllerutil.SetOwnerReference(owner, secret, scheme); err != nil {
		return fmt.Errorf("failed to set secret owner reference: %w", err)
	}

	if _, err := kubeClient.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update secret owner reference: %w", err)
	}

	return nil
}

func secretExists(ctx context.Context, client kubernetes.Interface, namespace, name string) (bool, error) {
	_, err := client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
