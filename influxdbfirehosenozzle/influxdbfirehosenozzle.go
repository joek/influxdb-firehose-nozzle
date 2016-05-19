package influxdbfirehosenozzle

import (
	"crypto/tls"
	"fmt"
	"log"
	"time"

	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/noaa/consumer"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gorilla/websocket"
	influxdbclient "github.com/influxdata/influxdb/client/v2"
	"github.com/joek/influxdb-firehose-nozzle/nozzleconfig"
)

// InfluxdbFirehoseNozzle type
type InfluxdbFirehoseNozzle struct {
	config                *nozzleconfig.NozzleConfig
	errs                  <-chan error
	messages              <-chan *events.Envelope
	authTokenFetcher      AuthTokenFetcher
	consumer              *consumer.Consumer
	client                influxdbclient.Client
	log                   *gosteno.Logger
	batchPoints           influxdbclient.BatchPoints
	totalMessagesReceived uint64
}

// AuthTokenFetcher interface
type AuthTokenFetcher interface {
	FetchAuthToken() string
}

// NewInfluxDBFirehoseNozzle creates new InfluxDBFirehoseNozzle
func NewInfluxDBFirehoseNozzle(config *nozzleconfig.NozzleConfig, tokenFetcher AuthTokenFetcher, log *gosteno.Logger) *InfluxdbFirehoseNozzle {
	i := &InfluxdbFirehoseNozzle{
		config:           config,
		authTokenFetcher: tokenFetcher,
		log:              log,
	}
	i.newBatchPoints()
	return i
}

func (i *InfluxdbFirehoseNozzle) createClient() {

	c, err := influxdbclient.NewHTTPClient(influxdbclient.HTTPConfig{
		Addr:               i.config.InfluxDbURL,
		Username:           i.config.InfluxDbUser,
		Password:           i.config.InfluxDbPassword,
		UserAgent:          i.config.FirehoseSubscriptionID,
		InsecureSkipVerify: !i.config.InfluxDbAllowSelfSigned,
	})
	if err != nil {
		fmt.Println("Error creating InfluxDB Client: ", err.Error())
	}
	i.client = c
}

func (i *InfluxdbFirehoseNozzle) Start() error {
	var authToken string

	if !i.config.DisableAccessControl {
		authToken = i.authTokenFetcher.FetchAuthToken()
	}

	i.log.Info("Starting Influxdb Firehose Nozzle...")
	i.createClient()
	i.consumeFirehose(authToken)
	err := i.postToInfluxDB()
	i.log.Info("Influxdb Firehose Nozzle shutting down...")
	return err
}

func (i *InfluxdbFirehoseNozzle) consumeFirehose(authToken string) {
	i.consumer = consumer.New(
		i.config.TrafficControllerURL,
		&tls.Config{InsecureSkipVerify: i.config.InsecureSSLSkipVerify},
		nil)
	i.consumer.SetIdleTimeout(time.Duration(i.config.IdleTimeoutSeconds) * time.Second)
	i.messages, i.errs = i.consumer.Firehose(i.config.FirehoseSubscriptionID, authToken)
}

func (i *InfluxdbFirehoseNozzle) postToInfluxDB() error {
	ticker := time.NewTicker(time.Duration(i.config.FlushDurationSeconds) * time.Second)
	for {
		select {
		case <-ticker.C:
			i.postMetrics()
		case envelope := <-i.messages:
			i.handleMessage(envelope)
			i.addMetric(envelope)
		case err := <-i.errs:
			i.handleError(err)
			return err
		}
	}
}

func (i *InfluxdbFirehoseNozzle) postMetrics() {
	err := i.client.Write(i.batchPoints)
	if err != nil {
		i.log.Fatalf("FATAL ERROR: %s\n\n", err)
	}
	i.newBatchPoints()
}

func (i *InfluxdbFirehoseNozzle) newBatchPoints() {
	bp, _ := influxdbclient.NewBatchPoints(influxdbclient.BatchPointsConfig{
		Database: i.config.InfluxDbDatabase,
	})
	i.batchPoints = bp
}

func (i *InfluxdbFirehoseNozzle) handleMessage(envelope *events.Envelope) {
	if envelope.GetEventType() == events.Envelope_CounterEvent && envelope.CounterEvent.GetName() == "TruncatingBuffer.DroppedMessages" && envelope.GetOrigin() == "doppler" {
		i.log.Infof("We've intercepted an upstream message which indicates that the nozzle or the TrafficController is not keeping up. Please try scaling up the nozzle.")
		i.alertSlowConsumerError()
	}
}

func (i *InfluxdbFirehoseNozzle) addMetric(envelope *events.Envelope) {
	i.totalMessagesReceived++
	if envelope.GetEventType() != events.Envelope_ValueMetric && envelope.GetEventType() != events.Envelope_CounterEvent {
		return
	}
	tags := map[string]string{
		"deployment": envelope.GetDeployment(),
		"job":        envelope.GetJob(),
		"index":      envelope.GetIndex(),
		"ip":         envelope.GetIp(),
	}

	fields := map[string]interface{}{
		"value": float64(getValue(envelope)),
	}

	t := time.Unix(0, envelope.GetTimestamp())
	pt, err := influxdbclient.NewPoint(getName(envelope), tags, fields, t)
	if err != nil {
		log.Println(err)
	} else {
		i.batchPoints.AddPoint(pt)
	}
}

func getName(envelope *events.Envelope) string {
	switch envelope.GetEventType() {
	case events.Envelope_ValueMetric:
		return envelope.GetOrigin() + "." + envelope.GetValueMetric().GetName()
	case events.Envelope_CounterEvent:
		return envelope.GetOrigin() + "." + envelope.GetCounterEvent().GetName()
	default:
		panic("Unknown event type")
	}
}

func getValue(envelope *events.Envelope) float64 {
	switch envelope.GetEventType() {
	case events.Envelope_ValueMetric:
		return envelope.GetValueMetric().GetValue()
	case events.Envelope_CounterEvent:
		return float64(envelope.GetCounterEvent().GetTotal())
	default:
		panic("Unknown event type")
	}
}

func (i *InfluxdbFirehoseNozzle) alertSlowConsumerError() {
	i.addInternalMetric("slowConsumerAlert", uint64(1))
}

func (i *InfluxdbFirehoseNozzle) addInternalMetric(name string, value uint64) {
	tags := map[string]string{
		"deployment": i.config.Deployment,
	}

	fields := map[string]interface{}{
		"value": float64(value),
	}

	t := time.Now()
	pt, _ := influxdbclient.NewPoint(name, tags, fields, t)
	i.batchPoints.AddPoint(pt)
}

func (i *InfluxdbFirehoseNozzle) handleError(err error) {
	switch closeErr := err.(type) {
	case *websocket.CloseError:
		switch closeErr.Code {
		case websocket.CloseNormalClosure:
		// no op
		case websocket.ClosePolicyViolation:
			i.log.Errorf("Error while reading from the firehose: %v", err)
			i.log.Errorf("Disconnected because nozzle couldn't keep up. Please try scaling up the nozzle.")
			i.alertSlowConsumerError()
		default:
			i.log.Errorf("Error while reading from the firehose: %v", err)
		}
	default:
		i.log.Errorf("Error while reading from the firehose: %v", err)

	}

	i.log.Infof("Closing connection with traffic controller due to %v", err)
	i.consumer.Close()
	i.postMetrics()
}
