package ready

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestReady(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ready Suite")
}
