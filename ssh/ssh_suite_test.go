package ssh

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSsh(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ssh Suite")
}
