package influxdbfirehosenozzle_test

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	. "github.com/cloudfoundry-incubator/datadog-firehose-nozzle/testhelpers"
	"github.com/cloudfoundry-incubator/datadog-firehose-nozzle/uaatokenfetcher"
	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"
	"github.com/gorilla/websocket"
	. "github.com/joek/influxdb-firehose-nozzle/influxdbfirehosenozzle"
	. "github.com/joek/influxdb-firehose-nozzle/influxhelpers"
	"github.com/joek/influxdb-firehose-nozzle/nozzleconfig"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Influxdbfirehosenozzle", func() {

	var (
		fakeUAA      *FakeUAA
		fakeFirehose *FakeFirehose
		config       *nozzleconfig.NozzleConfig
		tokenFetcher *uaatokenfetcher.UAATokenFetcher
		nozzle       *InfluxdbFirehoseNozzle
		log          *gosteno.Logger
		logContent   *bytes.Buffer
		fakeBuffer   *FakeBufferSink
		fakeInfluxDB *FakeInfluxDB
	)

	It("Should return error if type is unknown.", func() {
		envelope := events.Envelope{
			Origin:    proto.String("doppler"),
			Timestamp: proto.Int64(1000000000),
			EventType: events.Envelope_LogMessage.Enum(),
			LogMessage: &events.LogMessage{
				Message:     []byte("FOO"),
				MessageType: events.LogMessage_OUT.Enum(),
			},
			Deployment: proto.String("deployment-name"),
			Job:        proto.String("doppler"),
		}

		_, err := GetName(&envelope)
		_, err = GetValue(&envelope)
		Expect(err).ToNot(BeNil())
	})

	Describe("Integration test", func() {

		BeforeEach(func() {
			fakeUAA = NewFakeUAA("bearer", "123456789")
			fakeToken := fakeUAA.AuthToken()
			fakeFirehose = NewFakeFirehose(fakeToken)
			fakeInfluxDB = NewFakeInfluxDB()
			fakeUAA.Start()
			fakeFirehose.Start()
			fakeInfluxDB.Start()

			config = &nozzleconfig.NozzleConfig{
				UAAURL:               fakeUAA.URL(),
				FlushDurationSeconds: 10,
				InfluxDbURL:          fakeInfluxDB.URL(),
				TrafficControllerURL: strings.Replace(fakeFirehose.URL(), "http:", "ws:", 1),
				DisableAccessControl: false,
				MetricPrefix:         "datadog.nozzle.",
			}
			content := make([]byte, 1024)
			logContent = bytes.NewBuffer(content)
			fakeBuffer = NewFakeBufferSink(logContent)
			c := &gosteno.Config{
				Sinks: []gosteno.Sink{
					fakeBuffer,
				},
			}
			gosteno.Init(c)
			log = gosteno.NewLogger("test")
			tokenFetcher = uaatokenfetcher.New(fakeUAA.URL(), "un", "pwd", true, log)
			nozzle = NewInfluxDBFirehoseNozzle(config, tokenFetcher, log)

		})

		AfterEach(func() {
			fakeUAA.Close()
			fakeFirehose.Close()
			fakeInfluxDB.Close()
		})

		It("gets a valid authentication token", func() {
			go nozzle.Start()
			Eventually(fakeFirehose.Requested).Should(BeTrue())
			Consistently(fakeFirehose.LastAuthorization).Should(Equal("bearer 123456789"))
		})

		It("Is not creating influx client with false values", func() {
			config.InfluxDbURL = "FOO"
			nozzle = NewInfluxDBFirehoseNozzle(config, tokenFetcher, log)
			Expect(nozzle.Start()).To(HaveOccurred())
		})

		It("receives data from the firehose", func(done Done) {
			defer close(done)

			for i := 0; i < 10; i++ {
				envelope := events.Envelope{
					Origin:    proto.String("origin"),
					Timestamp: proto.Int64(1000000000),
					EventType: events.Envelope_ValueMetric.Enum(),
					ValueMetric: &events.ValueMetric{
						Name:  proto.String(fmt.Sprintf("metricName-%d", i)),
						Value: proto.Float64(float64(i)),
						Unit:  proto.String("gauge"),
					},
					Deployment: proto.String("deployment-name"),
					Job:        proto.String("doppler"),
				}
				fakeFirehose.AddEvent(envelope)
			}

			go nozzle.Start()

			var contents []byte
			Eventually(fakeInfluxDB.ReceivedContents).Should(Receive(&contents))

			Expect(fakeBuffer.GetContent()).ToNot(ContainSubstring("Error while reading from the firehose"))
			// +3 internal metrics that show totalMessagesReceived, totalMetricSent, and slowConsumerAlert
			Expect(string(contents)).Should(Equal(
				`origin.metricName-0,deployment=deployment-name,job=doppler value=0 1000000000
origin.metricName-1,deployment=deployment-name,job=doppler value=1 1000000000
origin.metricName-2,deployment=deployment-name,job=doppler value=2 1000000000
origin.metricName-3,deployment=deployment-name,job=doppler value=3 1000000000
origin.metricName-4,deployment=deployment-name,job=doppler value=4 1000000000
origin.metricName-5,deployment=deployment-name,job=doppler value=5 1000000000
origin.metricName-6,deployment=deployment-name,job=doppler value=6 1000000000
origin.metricName-7,deployment=deployment-name,job=doppler value=7 1000000000
origin.metricName-8,deployment=deployment-name,job=doppler value=8 1000000000
origin.metricName-9,deployment=deployment-name,job=doppler value=9 1000000000
`))
		}, 2)

		It("InfluxDB is down", func(done Done) {
			defer close(done)

			for i := 0; i < 10; i++ {
				envelope := events.Envelope{
					Origin:    proto.String("origin"),
					Timestamp: proto.Int64(1000000000),
					EventType: events.Envelope_ValueMetric.Enum(),
					ValueMetric: &events.ValueMetric{
						Name:  proto.String(fmt.Sprintf("metricName-%d", i)),
						Value: proto.Float64(float64(i)),
						Unit:  proto.String("gauge"),
					},
					Deployment: proto.String("deployment-name"),
					Job:        proto.String("doppler"),
				}
				fakeFirehose.AddEvent(envelope)
			}

			fakeInfluxDB.Close()
			Expect(nozzle.Start()).To(HaveOccurred())
		}, 2)

		It("Catch slow consumer alerts", func(done Done) {
			defer close(done)

			envelope := events.Envelope{
				Origin:    proto.String("doppler"),
				Timestamp: proto.Int64(1000000000),
				EventType: events.Envelope_CounterEvent.Enum(),
				CounterEvent: &events.CounterEvent{
					Name:  proto.String("TruncatingBuffer.DroppedMessages"),
					Delta: proto.Uint64(1),
					Total: proto.Uint64(10),
				},
				Deployment: proto.String("deployment-name"),
				Job:        proto.String("doppler"),
			}
			fakeFirehose.AddEvent(envelope)

			go nozzle.Start()
			var contents []byte
			Eventually(fakeInfluxDB.ReceivedContents).Should(Receive(&contents))

			matched, _ := regexp.MatchString("slowConsumerAlert value=1 .*\ndoppler.TruncatingBuffer.DroppedMessages,deployment=deployment-name,job=doppler value=10 1000000000", string(contents))
			Expect(matched).Should(BeTrue())
		}, 2)

		It("Ignore none numeric events", func(done Done) {
			defer close(done)
			envelope := events.Envelope{
				Origin:    proto.String("doppler"),
				Timestamp: proto.Int64(1000000000),
				EventType: events.Envelope_LogMessage.Enum(),
				LogMessage: &events.LogMessage{
					Message:     []byte("FOO"),
					MessageType: events.LogMessage_OUT.Enum(),
				},
				Deployment: proto.String("deployment-name"),
				Job:        proto.String("doppler"),
			}
			fakeFirehose.AddEvent(envelope)

			go nozzle.Start()
			var contents []byte
			Eventually(fakeInfluxDB.ReceivedContents).Should(Receive(&contents))

			Expect(string(contents)).Should(Equal(``))
		}, 2)

		It("Add tags", func(done Done) {
			defer close(done)

			for i := 0; i < 3; i++ {
				envelope := events.Envelope{
					Origin:    proto.String("origin"),
					Timestamp: proto.Int64(1000000000),
					EventType: events.Envelope_ValueMetric.Enum(),
					ValueMetric: &events.ValueMetric{
						Name:  proto.String(fmt.Sprintf("metricName-%d", i)),
						Value: proto.Float64(float64(i)),
						Unit:  proto.String("gauge"),
					},
					Deployment: proto.String("deployment-name"),
					Job:        proto.String("doppler"),
					Tags: map[string]string{
						fmt.Sprintf("tag-%d", i): "tagsvalue",
						fmt.Sprintf("tag-%d", i): "tagsvalue",
					},
				}
				fakeFirehose.AddEvent(envelope)
			}

			go nozzle.Start()

			var contents []byte
			Eventually(fakeInfluxDB.ReceivedContents).Should(Receive(&contents))

			Expect(fakeBuffer.GetContent()).ToNot(ContainSubstring("Error while reading from the firehose"))
			// +3 internal metrics that show totalMessagesReceived, totalMetricSent, and slowConsumerAlert
			Expect(string(contents)).Should(Equal(
				`origin.metricName-0,deployment=deployment-name,job=doppler,tag-0=tagsvalue value=0 1000000000
origin.metricName-1,deployment=deployment-name,job=doppler,tag-1=tagsvalue value=1 1000000000
origin.metricName-2,deployment=deployment-name,job=doppler,tag-2=tagsvalue value=2 1000000000
`))

		}, 2)
		It("Handle ClosePolicyViolation", func(done Done) {
			defer close(done)

			for i := 0; i < 10; i++ {
				envelope := events.Envelope{
					Origin:    proto.String("origin"),
					Timestamp: proto.Int64(1000000000),
					EventType: events.Envelope_ValueMetric.Enum(),
					ValueMetric: &events.ValueMetric{
						Name:  proto.String(fmt.Sprintf("metricName-%d", i)),
						Value: proto.Float64(float64(i)),
						Unit:  proto.String("gauge"),
					},
					Deployment: proto.String("deployment-name"),
					Job:        proto.String("doppler"),
				}
				fakeFirehose.AddEvent(envelope)
			}

			fakeFirehose.SetCloseMessage(websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "Client did not respond to ping before keep-alive timeout expired."))

			go nozzle.Start()

			var contents []byte
			Eventually(fakeInfluxDB.ReceivedContents).Should(Receive(&contents))

			matched, _ := regexp.MatchString(".*slowConsumerAlert value=1 .*", string(contents))
			Expect(matched).Should(BeTrue())

			logOutput := fakeBuffer.GetContent()
			Expect(logOutput).To(ContainSubstring("Error while reading from the firehose"))
			Expect(logOutput).To(ContainSubstring("Client did not respond to ping before keep-alive timeout expired."))
			Expect(logOutput).To(ContainSubstring("Disconnected because nozzle couldn't keep up."))
		})
		It("Handle ClosePolicyViolation", func(done Done) {
			defer close(done)

			fakeFirehose.SetCloseMessage(websocket.FormatCloseMessage(websocket.CloseInvalidFramePayloadData, "Weird things happened."))

			go nozzle.Start()

			var contents []byte
			Eventually(fakeInfluxDB.ReceivedContents).Should(Receive(&contents))

			matched, _ := regexp.MatchString(".*slowConsumerAlert value=1 .*", string(contents))
			Expect(matched).Should(BeFalse())

			logOutput := fakeBuffer.GetContent()
			Expect(logOutput).To(ContainSubstring("Error while reading from the firehose"))
			Expect(logOutput).NotTo(ContainSubstring("Client did not respond to ping before keep-alive timeout expired."))
			Expect(logOutput).NotTo(ContainSubstring("Disconnected because nozzle couldn't keep up."))
		})
	})
})
