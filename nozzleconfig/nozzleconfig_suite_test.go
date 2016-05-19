package nozzleconfig_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestNozzleconfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Nozzleconfig Suite")
}
