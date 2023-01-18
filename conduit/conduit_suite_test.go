package conduit_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConduit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Conduit Suite")
}
