//go:build tools

package tools

import (
	_ "github.com/onsi/ginkgo/v2/ginkgo"
	_ "github.com/vburenin/ifacemaker"
	_ "github.com/maxbrunsfeld/counterfeiter/v6"
)

// This file imports packages that are used when running go generate, or used
// during the development process but not otherwise depended on by built code.
