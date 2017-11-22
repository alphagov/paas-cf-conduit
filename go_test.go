package main_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"os"
	"os/exec"
	"strings"
)

var _ = Describe("Go", func() {
	Context("os/exec", func() {
		Context("with a duplicated environment variable", func() {
			It("should use the last value", func() {
				proc := exec.Command("printenv", "VAR")
				proc.Env = append(os.Environ(),
					"VAR=superseded",
					"VAR=utilised",
				)
				outputBytes, err := proc.Output()
				Expect(err).ToNot(HaveOccurred())

				output := strings.TrimSpace(string(outputBytes))
				Expect(output).To(Equal("utilised"), "In Go <1.9, os/exec mishandled duplicate environment variables. This mishandling appears to be taking place. Please use Go >=1.9.")
			})
		})
	})
})
