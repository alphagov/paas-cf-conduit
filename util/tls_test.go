package util_test

import (
	"crypto/tls"

	_ "embed"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/alphagov/paas-cf-conduit/util"
)

//go:embed testdata/certs.pem
var pemData []byte

var _ = Describe("util", func() {
	Describe("CipherSuiteNamesToIDs", func() {
		It("should return a list of uint16 IDs for the given cipher suite names", func() {
			cipherSuites := []string{
				"TLS_RSA_WITH_AES_256_CBC_SHA",
				"TLS_RSA_WITH_AES_256_GCM_SHA384",
			}
			ids, err := CipherSuiteNamesToIDs(cipherSuites)
			Expect(err).NotTo(HaveOccurred())
			Expect(ids).To(ContainElement(tls.TLS_RSA_WITH_AES_256_CBC_SHA))
			Expect(ids).To(ContainElement(tls.TLS_RSA_WITH_AES_256_GCM_SHA384))
		})

		It("should return an error if an invalid cipher suite name is provided", func() {
			cipherSuites := []string{
				"TLS_RSA_WITH_AES_256_CBC_SHA",
				"TLS_RSA_WITH_INVALID_CIPHER",
			}
			_, err := CipherSuiteNamesToIDs(cipherSuites)
			Expect(err).To(HaveOccurred())
		})

		It("should return the default cipher suites if none are provided", func() {
			ids, err := CipherSuiteNamesToIDs(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(ids).To(ContainElement(tls.TLS_RSA_WITH_AES_256_CBC_SHA))
			Expect(ids).To(ContainElement(tls.TLS_RSA_WITH_AES_256_GCM_SHA384))
			Expect(ids).To(ContainElement(tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384))
			Expect(ids).To(ContainElement(tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384))
		})
	})

	Describe("TLSVersionToID", func() {
		It("should return the int ID for a valid TLS version", func() {
			version := "TLS12"
			id, err := TLSVersionToID(version)
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(uint16(tls.VersionTLS12)))
		})

		It("should return an error if an invalid TLS version is provided", func() {
			version := "INVALID_VERSION"
			_, err := TLSVersionToID(version)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetRootCAs", func() {

		It("should return CertPool with two subjects", func() {
			certPool, err := GetRootCAs(pemData)
			Expect(err).ToNot(HaveOccurred())
			Expect(certPool.Subjects()).To(HaveLen(2))
		})

		It("should return 0 certs if passed invalid data", func() {
			certPool, err := GetRootCAs([]byte("invalid data"))
			Expect(err).ToNot(HaveOccurred())
			Expect(certPool.Subjects()).To(HaveLen(0))
		})
	})
})
