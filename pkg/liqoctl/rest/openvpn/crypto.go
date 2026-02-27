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
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"encoding/asn1"
	"fmt"
	"math/big"
	"net"
	"time"
)

// CertificateBundle contains all the OpenVPN certificates and keys.
type CertificateBundle struct {
	CAKey      []byte
	CACert     []byte
	ServerKey  []byte
	ServerCert []byte
	ClientKey  []byte
	ClientCert []byte
	DHParams   []byte
	TLSAuthKey []byte
}

const rsaKeySize = 2048

// GenerateCertificateBundle generates all necessary certificates and keys for OpenVPN.
func GenerateCertificateBundle() (*CertificateBundle, error) {
	bundle := &CertificateBundle{}

	// Generate CA key and cert
	caKey, caCert, err := generateCA()
	if err != nil {
		return nil, fmt.Errorf("failed to generate CA: %w", err)
	}
	bundle.CAKey = caKey
	bundle.CACert = caCert

	// Generate server key and cert
	serverKey, serverCert, err := generateCertificate("server", caKey, caCert)
	if err != nil {
		return nil, fmt.Errorf("failed to generate server certificate: %w", err)
	}
	bundle.ServerKey = serverKey
	bundle.ServerCert = serverCert

	// Generate client key and cert
	clientKey, clientCert, err := generateCertificate("client", caKey, caCert)
	if err != nil {
		return nil, fmt.Errorf("failed to generate client certificate: %w", err)
	}
	bundle.ClientKey = clientKey
	bundle.ClientCert = clientCert

	// Generate DH parameters
	dhParams, err := generateDHParameters()
	if err != nil {
		return nil, fmt.Errorf("failed to generate DH parameters: %w", err)
	}
	bundle.DHParams = dhParams

	// Generate TLS auth key
	tlsAuthKey, err := generateTLSAuthKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate TLS auth key: %w", err)
	}
	bundle.TLSAuthKey = tlsAuthKey

	return bundle, nil
}

// generateCA generates a self-signed CA certificate and key.
func generateCA() (keyPEM, certPEM []byte, err error) {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, rsaKeySize)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Country:      []string{"US"},
			Organization: []string{"Liqo"},
			CommonName:   "liqo-ca",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0), // 10 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// Create certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode to PEM
	keyPEM = encodePrivateKeyPEM(privateKey)
	certPEM = encodeCertPEM(certBytes)

	return keyPEM, certPEM, nil
}

// generateCertificate generates a certificate signed by the CA.
func generateCertificate(commonName string, caKeyPEM, caCertPEM []byte) (keyPEM, certPEM []byte, err error) {
	// Parse CA key and cert
	caKey, err := parsePrivateKeyPEM(caKeyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA key: %w", err)
	}

	caCert, err := parseCertPEM(caCertPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA cert: %w", err)
	}

	// Generate private key for the certificate
	privateKey, err := rsa.GenerateKey(rand.Reader, rsaKeySize)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(int64(time.Now().Unix())),
		Subject: pkix.Name{
			Country:      []string{"US"},
			Organization: []string{"Liqo"},
			CommonName:   commonName,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(1, 0, 0), // 1 year
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	// Create certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, &privateKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode to PEM
	keyPEM = encodePrivateKeyPEM(privateKey)
	certPEM = encodeCertPEM(certBytes)

	return keyPEM, certPEM, nil
}

// generateTLSAuthKey generates a static key for TLS authentication.
func generateTLSAuthKey() ([]byte, error) {
	keySize := 2048
	keyBytes := make([]byte, keySize)
	_, err := rand.Read(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// OpenVPN TLS auth key format: header, key bytes in hex, footer
	header := []byte("-----BEGIN OpenVPN Static key V1-----\n")
	footer := []byte("-----END OpenVPN Static key V1-----\n")

	// Format key as hex chunks (16 bytes per line, 2 hex chars per byte)
	var buf bytes.Buffer
	buf.Write(header)
	for i := 0; i < len(keyBytes); i += 16 {
		end := i + 16
		if end > len(keyBytes) {
			end = len(keyBytes)
		}
		fmt.Fprintf(&buf, "%X\n", keyBytes[i:end])
	}
	buf.Write(footer)

	return buf.Bytes(), nil
}

// generateDHParameters generates Diffie-Hellman parameters.
// It uses RFC 3526 Group 14 (2048-bit MODP group) which is a safe prime group.
func generateDHParameters() ([]byte, error) {
	prime := new(big.Int)
	
	// RFC 3526 Group 14: 2048-bit MODP Group (512 hex characters)
	prime.SetString(
		"FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1"+
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD"+
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245"+
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED"+
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D"+
		"C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F"+
		"83655D23DCA3AD961C62F356208552BB9ED529077096966D"+
		"670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B"+
		"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9"+
		"DE2BCBF6955817183995497CEA956AE515D2261898FA0510"+
		"15728E5A8AACAA68FFFFFFFFFFFFFFFF",
		16,
	)

	generator := big.NewInt(2)

	der := encodeDHParams(prime, generator)

	encoded := pem.EncodeToMemory(&pem.Block{
		Type:  "DH PARAMETERS",
		Bytes: der,
	})

	return encoded, nil
}


type dhParams struct {
	P *big.Int
	G *big.Int
}
// encodeDHParams encodes DH parameters (prime and generator) in SEQUENCE format
func encodeDHParams(prime, generator *big.Int) []byte {
	params := dhParams{
		P: prime,
		G: generator,
	}

	derBytes, err := asn1.Marshal(params)
	if err != nil {
		panic("Error during the ASN.1 marshaling of DH parameters: " + err.Error())
	}

	return derBytes
}


func encodePrivateKeyPEM(key *rsa.PrivateKey) []byte {
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(key)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})
	return privateKeyPEM
}

func encodeCertPEM(certBytes []byte) []byte {
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})
	return certPEM
}

func parsePrivateKeyPEM(keyPEM []byte) (crypto.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return privateKey, nil
}

func parseCertPEM(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	return cert, nil
}
