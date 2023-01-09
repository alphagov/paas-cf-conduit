package util

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"strings"
)

// CipherSuiteNamesToIDs converts a list of cipher suite names to a list of cipher suite IDs
func CipherSuiteNamesToIDs(cipherSuites []string) (tlsCipherSuites []uint16, err error) {
	if len(cipherSuites) == 0 {
		// If no cipher suites are specified, use the default list
		cipherSuites = []string{
			"TLS_RSA_WITH_AES_256_CBC_SHA",
			"TLS_RSA_WITH_AES_256_GCM_SHA384",
			"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
			"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
		}
	}

	cipherSuiteMap := make(map[string]uint16)

	for _, cipherSuite := range tls.CipherSuites() {
		cipherSuiteMap[cipherSuite.Name] = cipherSuite.ID
	}

	for _, cipherSuite := range tls.InsecureCipherSuites() {
		cipherSuiteMap[cipherSuite.Name] = cipherSuite.ID
	}
	for _, cipherSuite := range cipherSuites {
		cs, ok := cipherSuiteMap[cipherSuite]
		if !ok {
			return nil, fmt.Errorf("invalid cipher suite: %s, valid names include %s", cipherSuite, strings.Join(func() []string {
				keys := make([]string, 0, len(cipherSuiteMap))
				for key := range cipherSuiteMap {
					keys = append(keys, key)
				}
				return keys
			}(), ","))
		}
		tlsCipherSuites = append(tlsCipherSuites, cs)
	}
	return tlsCipherSuites, nil
}

// TLSVersionToID converts a TLS version name to a TLS version ID
func TLSVersionToID(version string) (uint16, error) {
	versions := map[string]uint16{
		"SSL30": tls.VersionSSL30,
		"TLS10": tls.VersionTLS10,
		"TLS11": tls.VersionTLS11,
		"TLS12": tls.VersionTLS12,
		"TLS13": tls.VersionTLS13,
	}

	if _, ok := versions[version]; !ok {
		return 0, fmt.Errorf("invalid minimum TLS version: %s, valid names include %s", version, strings.Join(func() []string {
			keys := make([]string, 0, len(versions))
			for key := range versions {
				keys = append(keys, key)
			}
			return keys
		}(), ","))
	}
	return versions[version], nil
}

// GetRootCAs reads PEM-encoded certificate data from a file or input argument, parses them into x509 certificates,
// and adds them to a new certificate pool. It returns the certificate pool and an error if any occurred.
// If the input argument is nil, it reads the PEM-encoded certificates from the file "/etc/ssl/cert.pem".
//
//
// This is a workaround for the following issue:
//   https://github.com/golang/go/issues/51991
//
// In summary on the latest version of Mac OS X and golang 1.18+, the MAC OS has started enforcing the use of
// SCTs (Signed Certificate Timestamp) in TLS certificates. This is a good thing, but it breaks with
// aws internal services like elasticache, which do not use SCTs.
//
// The workaround is to import all the system root CAs into a new certificate pool. This will then use
// golangs certifcate verification logic to check the certificate chain, but will not enforce the SCTs.
//
// Hopefully aws will find a solution to this issue and this code can be removed.

func GetRootCAs(pemDataIn []byte) (*x509.CertPool, error) {

	var pemData []byte
	var err error
	if pemDataIn == nil {
		pemData, err = ReadLocalSSLFile()
		if err != nil {
			return nil, err
		}
	} else {
		pemData = pemDataIn
	}
	rootCAs := x509.NewCertPool()

	for len(pemData) > 0 {
		var block *pem.Block
		block, pemData = pem.Decode(pemData)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			fmt.Println("Error parsing certificate:", err)
			continue
		}
		rootCAs.AddCert(cert)
	}

	return rootCAs, nil
}

func ReadLocalSSLFile() ([]byte, error) {
	pemData, err := ioutil.ReadFile("/etc/ssl/cert.pem")
	if err != nil {
		return nil, err
	}
	return pemData, nil
}
