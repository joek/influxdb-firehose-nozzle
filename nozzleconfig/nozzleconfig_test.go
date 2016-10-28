package nozzleconfig_test

import (
	"os"

	"github.com/joek/influxdb-firehose-nozzle/nozzleconfig"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("NozzleConfig", func() {
	BeforeEach(func() {
		os.Clearenv()
	})

	It("successfully parses a valid config", func() {
		conf, err := nozzleconfig.Parse("../config/firehose-nozzle-config.json")
		Expect(err).ToNot(HaveOccurred())
		Expect(conf.UAAURL).To(Equal("https://uaa.walnut.cf-app.com"))
		Expect(conf.Username).To(Equal("user"))
		Expect(conf.Password).To(Equal("user_password"))
		Expect(conf.InfluxDbURL).To(Equal("https://88.198.249.61:8086"))
		Expect(conf.InfluxDbDatabase).To(Equal("cloudfoundry"))
		Expect(conf.InfluxDbUser).To(Equal("cf"))
		Expect(conf.InfluxDbPassword).To(Equal("cf"))
		Expect(conf.InfluxDbAllowSelfSigned).To(Equal(true))
		Expect(conf.FlushDurationSeconds).To(BeEquivalentTo(15))
		Expect(conf.InsecureSSLSkipVerify).To(Equal(true))
		Expect(conf.MetricPrefix).To(Equal("influxclient"))
		Expect(conf.Deployment).To(Equal("deployment-name"))
		Expect(conf.DisableAccessControl).To(Equal(false))
		Expect(conf.IdleTimeoutSeconds).To(BeEquivalentTo(60))
	})

	It("successfully overwrites file config values with environmental variables", func() {
		os.Setenv("NOZZLE_UAAURL", "https://uaa.walnut-env.cf-app.com")
		os.Setenv("NOZZLE_USERNAME", "env-user")
		os.Setenv("NOZZLE_PASSWORD", "env-user-password")
		os.Setenv("NOZZLE_INFLUXDBURL", "https://1.2.3.4:8086")
		os.Setenv("NOZZLE_INFLUXDBDATABASE", "test1")
		os.Setenv("NOZZLE_INFLUXDBUSER", "test1")
		os.Setenv("NOZZLE_INFLUXDBPASSWORD", "test1")
		os.Setenv("NOZZLE_INFLUXDBALLOWSELFSIGNED", "false")
		os.Setenv("NOZZLE_FLUSHDURATIONSECONDS", "25")
		os.Setenv("NOZZLE_INSECURESSLSKIPVERIFY", "false")
		os.Setenv("NOZZLE_METRICPREFIX", "env-influxclient")
		os.Setenv("NOZZLE_DEPLOYMENT", "env-deployment-name")
		os.Setenv("NOZZLE_DISABLEACCESSCONTROL", "true")
		os.Setenv("NOZZLE_IDLETIMEOUTSECONDS", "30")

		conf, err := nozzleconfig.Parse("../config/firehose-nozzle-config.json")
		Expect(err).ToNot(HaveOccurred())
		Expect(conf.UAAURL).To(Equal("https://uaa.walnut-env.cf-app.com"))
		Expect(conf.Username).To(Equal("env-user"))
		Expect(conf.Password).To(Equal("env-user-password"))
		Expect(conf.InfluxDbURL).To(Equal("https://1.2.3.4:8086"))
		Expect(conf.InfluxDbDatabase).To(Equal("test1"))
		Expect(conf.InfluxDbUser).To(Equal("test1"))
		Expect(conf.InfluxDbPassword).To(Equal("test1"))
		Expect(conf.InfluxDbAllowSelfSigned).To(Equal(false))
		Expect(conf.FlushDurationSeconds).To(BeEquivalentTo(25))
		Expect(conf.InsecureSSLSkipVerify).To(Equal(false))
		Expect(conf.MetricPrefix).To(Equal("env-influxclient"))
		Expect(conf.Deployment).To(Equal("env-deployment-name"))
		Expect(conf.DisableAccessControl).To(Equal(true))
		Expect(conf.IdleTimeoutSeconds).To(BeEquivalentTo(30))
	})
})
