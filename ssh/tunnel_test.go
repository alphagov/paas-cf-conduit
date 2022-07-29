package ssh

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	"golang.org/x/crypto/ssh"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Fingerprint checking", func() {
	// RSA public key pulled from development environment
	// `cat private-key | openssl rsa -pubout`
	const (
		rsaPubKeyPEM = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAq2r6Lc5LsBDl8IB0riyr
/vTOTu9BnWAueBOqYmCuK7JrmhXpiw6oFJdvgD7sd4Nnr0+Nv7rJZZ45PJW5wlcP
joVkMLJQROEW7G3H2yonfaKTlQ71Xh0UObIB3CsQ920EHOfRnNTw4jXDONUIDFM8
SfNslfSnRmiOX2O6oTbYoDzJiBRx4obtZtOWhX6kcoFkubOg5W4oP77CE8dSNFtZ
JdwgL8l1TITjsP9sIve4oy1IHbAa8s6j9U1J5gRWRnhys3w26XZw1u6YMmM9GPFQ
8r7fPYp8NU0CrEJ5TP/CmtokHyyEJ1L3Hec18r0wh9nYPQfbY9Ah2IEd0+9/b2kk
9wIDAQAB
-----END PUBLIC KEY-----`

		correctSHA256Sum        = "YM/+fDBqPFHaaWhdRb2Y7uvQFBXGs9BCUbe6zXzegtc"
		correctMD5Sum           = "999d2afa3d599197d603d87edd45e759"
		correctMD5SumWithColons = "99:9d:2a:fa:3d:59:91:97:d6:03:d8:7e:dd:45:e7:59"

		incorrectSHA256Sum = "AM/+fDBqPFHaaWhdRb2Y7uvQFBXGs9BCUbe6zXzegtc"
		incorrectMD5Sum    = "899d2afa3d599197d603d87edd45e759"
	)

	var sshPubKey ssh.PublicKey

	BeforeEach(func() {
		loadedPEM, restPEM := pem.Decode([]byte(rsaPubKeyPEM))
		Expect(restPEM).To(HaveLen(0), "PEM should decode fully")
		Expect(loadedPEM.Type).To(Equal("PUBLIC KEY"), "PEM should be public key")

		rsaPubKeyPrecast, err := x509.ParsePKIXPublicKey(loadedPEM.Bytes)
		Expect(err).NotTo(HaveOccurred(), "RSA public key should parse")
		Expect(rsaPubKeyPrecast).NotTo(BeNil(), "RSA public key should parse")

		rsaPubKey, ok := rsaPubKeyPrecast.(*rsa.PublicKey)
		Expect(ok).To(Equal(true), "RSA public key should cast")
		Expect(rsaPubKey).NotTo(BeNil(), "RSA public key should cast")

		sshPubKey, err = ssh.NewPublicKey(rsaPubKey)
		Expect(err).NotTo(HaveOccurred(), "SSH public key should create")
	})

	It("Should accept valid sha256 fingerprints", func() {
		compatible, _ := checkSSHFingerprint(sshPubKey, correctSHA256Sum)

		Expect(compatible).To(Equal(true))
	})

	It("Should accept valid MD5 fingerprints", func() {
		compatible, _ := checkSSHFingerprint(sshPubKey, correctMD5Sum)

		Expect(compatible).To(Equal(true))
	})

	It("Should accept valid MD5 fingerprints with colons", func() {
		compatible, _ := checkSSHFingerprint(sshPubKey, correctMD5SumWithColons)

		Expect(compatible).To(Equal(true))
	})

	It("Should return useful info about expected fingerprints", func() {
		_, expected := checkSSHFingerprint(sshPubKey, "")

		Expect(expected).To(HaveLen(2))
		Expect(expected).To(ContainElement(correctSHA256Sum))
		Expect(expected).To(ContainElement(correctMD5Sum))
	})

	It("Should reject valid but incorrect SHA256 fingerprints", func() {
		compatible, _ := checkSSHFingerprint(sshPubKey, incorrectSHA256Sum)

		Expect(compatible).To(Equal(false))
	})

	It("Should reject valid but incorrect MD5 fingerprints", func() {
		compatible, _ := checkSSHFingerprint(sshPubKey, incorrectMD5Sum)

		Expect(compatible).To(Equal(false))
	})

	It("Should reject the empty string", func() {
		compatible, _ := checkSSHFingerprint(sshPubKey, "")

		Expect(compatible).To(Equal(false))
	})
})
