package influxdbfirehosenozzle

import (
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/noaa/consumer"
	noaaerrors "github.com/cloudfoundry/noaa/errors"
	"github.com/cloudfoundry/sonde-go/events"
	influxdbclient "github.com/influxdata/influxdb/client/v2"
	"github.com/joek/influxdb-firehose-nozzle/nozzleconfig"
)

// InfluxdbFirehoseNozzle type
type InfluxdbFirehoseNozzle struct {
	config                *nozzleconfig.NozzleConfig
	Errs                  <-chan error
	Messages              <-chan *events.Envelope
	authTokenFetcher      AuthTokenFetcher
	Consumer              *consumer.Consumer
	Client                influxdbclient.Client
	Log                   *gosteno.Logger
	batchPoints           influxdbclient.BatchPoints
	totalMessagesReceived uint64
}

// AuthTokenFetcher interface
type AuthTokenFetcher interface {
	FetchAuthToken() string
}

// NewInfluxDBFirehoseNozzle creates new InfluxDBFirehoseNozzle
func NewInfluxDBFirehoseNozzle(config *nozzleconfig.NozzleConfig, tokenFetcher AuthTokenFetcher, Log *gosteno.Logger) *InfluxdbFirehoseNozzle {
	i := &InfluxdbFirehoseNozzle{
		config:           config,
		authTokenFetcher: tokenFetcher,
		Log:              Log,
	}

	i.Consumer = consumer.New(
		i.config.TrafficControllerURL,
		&tls.Config{InsecureSkipVerify: i.config.InsecureSSLSkipVerify},
		nil)

	i.newBatchPoints()
	return i
}

func (i *InfluxdbFirehoseNozzle) createClient() error {

	c, err := influxdbclient.NewHTTPClient(influxdbclient.HTTPConfig{
		Addr:               i.config.InfluxDbURL,
		Username:           i.config.InfluxDbUser,
		Password:           i.config.InfluxDbPassword,
		UserAgent:          i.config.FirehoseSubscriptionID,
		InsecureSkipVerify: !i.config.InfluxDbAllowSelfSigned,
	})
	if err != nil {
		fmt.Println("Error creating InfluxDB Client: ", err.Error())
		return err
	}
	i.Client = c
	return nil
}

// Start is openning the connection to the firehose and forwarding messages to influxDB.
func (i *InfluxdbFirehoseNozzle) Start() error {
	var authToken string

	if !i.config.DisableAccessControl {
		authToken = i.authTokenFetcher.FetchAuthToken()
	}

	i.Log.Info("Starting Influxdb Firehose Nozzle...")
	err := i.createClient()
	if err != nil {
		return err
	}
	i.consumeFirehose(authToken)
	err = i.postToInfluxDB()
	i.Log.Info("Influxdb Firehose Nozzle shutting down...")
	return err
}

func (i *InfluxdbFirehoseNozzle) consumeFirehose(authToken string) {
	i.Consumer.SetIdleTimeout(time.Duration(i.config.IdleTimeoutSeconds) * time.Second)
	i.Messages, i.Errs = i.Consumer.Firehose(i.config.FirehoseSubscriptionID, authToken)
}

func (i *InfluxdbFirehoseNozzle) postToInfluxDB() (err error) {
	ticker := time.NewTicker(time.Duration(i.config.FlushDurationSeconds) * time.Second)
	for {
		select {
		case <-ticker.C:
			// TODO: Refactor, post metrics should run in it's own process
			err = i.postMetrics()
			if err != nil {
				return err
			}
		case envelope := <-i.Messages:
			i.handleMessage(envelope)
			i.AddMetric(envelope)
			if err != nil {
				return err
			}
		case err := <-i.Errs:
			i.handleError(err)
			return err
		}
	}
}

func (i *InfluxdbFirehoseNozzle) postMetrics() (err error) {
	err = i.Client.Write(i.batchPoints)
	if err != nil {
		i.Log.Errorf("FATAL ERROR: %s\n\n", err)
		return
	}
	i.newBatchPoints()
	return
}

func (i *InfluxdbFirehoseNozzle) newBatchPoints() {
	bp, _ := influxdbclient.NewBatchPoints(influxdbclient.BatchPointsConfig{
		Database: i.config.InfluxDbDatabase,
	})
	i.batchPoints = bp
}

func (i *InfluxdbFirehoseNozzle) handleMessage(envelope *events.Envelope) {
	if envelope.GetEventType() == events.Envelope_CounterEvent && envelope.CounterEvent.GetName() == "TruncatingBuffer.DroppedMessages" && envelope.GetOrigin() == "doppler" {
		i.Log.Infof("We've intercepted an upstream message which indicates that the nozzle or the TrafficController is not keeping up. Please try scaling up the nozzle.")
		i.alertSlowConsumerError()
	}
}

// AddMetric is parsing envelop events and adding numeric metrics to the influx batch cache
func (i *InfluxdbFirehoseNozzle) AddMetric(envelope *events.Envelope) error {
	i.totalMessagesReceived++
	if envelope.GetEventType() == events.Envelope_ValueMetric || envelope.GetEventType() == events.Envelope_CounterEvent {

		tags := map[string]string{
			"deployment": envelope.GetDeployment(),
			"job":        envelope.GetJob(),
			"index":      envelope.GetIndex(),
			"ip":         envelope.GetIp(),
		}

		for k, v := range envelope.GetTags() {
			tags[k] = v
		}

		v, err := GetValue(envelope)
		fields := map[string]interface{}{
			"value": v,
		}

		t := time.Unix(0, envelope.GetTimestamp())
		n, err := GetName(envelope)
		pt, err := influxdbclient.NewPoint(n, tags, fields, t)
		if err != nil {
			return errors.New("Failed to add Point")
		}
		i.batchPoints.AddPoint(pt)
	}
	return nil
}

// GetName generates event name string
func GetName(envelope *events.Envelope) (string, error) {
	switch envelope.GetEventType() {
	case events.Envelope_ValueMetric:
		return envelope.GetOrigin() + "." + envelope.GetValueMetric().GetName(), nil
	case events.Envelope_CounterEvent:
		return envelope.GetOrigin() + "." + envelope.GetCounterEvent().GetName(), nil
	default:
		return "", errors.New("Unknown event type")
	}
}

// GetValue extracts the numeric value from different event types.
func GetValue(envelope *events.Envelope) (float64, error) {
	switch envelope.GetEventType() {
	case events.Envelope_ValueMetric:
		return envelope.GetValueMetric().GetValue(), nil
	case events.Envelope_CounterEvent:
		return float64(envelope.GetCounterEvent().GetTotal()), nil
	default:
		return 0, errors.New("Unknown event type")
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
	switch err.(type) {
	case noaaerrors.RetryError:
		i.Log.Errorf("Reconnecting: %v", err)
	default:
		i.Log.Errorf("Error while reading from the firehose: %v", err)

	}

	i.Log.Infof("Closing connection with traffic controller due to %v", err)
	i.Consumer.Close()
	i.postMetrics()
}
