package util_test

import (
	"fmt"
	"net"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/alphagov/paas-cf-conduit/util"
)

var _ = Describe("PortIsInUse", func() {
	var (
		err      error
		port     int
		listener net.Listener
	)

	BeforeEach(func() {
		port, err = util.GetRandomPort()
		Expect(err).NotTo(HaveOccurred())
	})

	Context("when the port is in use", func() {
		BeforeEach(func() {
			listener, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			listener.Close()
		})

		It("should return true", func() {
			Expect(util.PortIsInUse(port)).To(Equal(true))
		})
	})

	Context("when the port is not in use", func() {
		It("should return false", func() {
			Expect(util.PortIsInUse(port)).To(Equal(false))
		})
	})
})
