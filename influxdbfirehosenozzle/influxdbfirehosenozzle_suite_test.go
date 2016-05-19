package influxdbfirehosenozzle_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestInfluxdbfirehosenozzle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Influxdbfirehosenozzle Suite")
}
