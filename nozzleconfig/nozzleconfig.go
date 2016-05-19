package nozzleconfig

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
)

// NozzleConfig stores configuration of the influx firehose nozzle
type NozzleConfig struct {
	UAAURL                  string
	Username                string
	Password                string
	TrafficControllerURL    string
	FirehoseSubscriptionID  string
	InfluxDbURL             string
	InfluxDbDatabase        string
	InfluxDbUser            string
	InfluxDbPassword        string
	InfluxDbAllowSelfSigned bool
	FlushDurationSeconds    uint32
	InsecureSSLSkipVerify   bool
	MetricPrefix            string
	Deployment              string
	DisableAccessControl    bool
	IdleTimeoutSeconds      uint32
}

// Parse nozzle config file and overwrite values if env variables are set.
func Parse(configPath string) (*NozzleConfig, error) {
	configBytes, err := ioutil.ReadFile(configPath)
	var config NozzleConfig
	if err != nil {
		return nil, fmt.Errorf("Can not read config file [%s]: %s", configPath, err)
	}

	err = json.Unmarshal(configBytes, &config)
	if err != nil {
		return nil, fmt.Errorf("Can not parse config file %s: %s", configPath, err)
	}

	overrideWithEnvVar("NOZZLE_UAAURL", &config.UAAURL)
	overrideWithEnvVar("NOZZLE_USERNAME", &config.Username)
	overrideWithEnvVar("NOZZLE_PASSWORD", &config.Password)
	overrideWithEnvVar("NOZZLE_TRAFFICCONTROLLERURL", &config.TrafficControllerURL)
	overrideWithEnvVar("NOZZLE_FIREHOSESUBSCRIPTIONID", &config.FirehoseSubscriptionID)

	overrideWithEnvVar("NOZZLE_INFLUXDBURL", &config.InfluxDbURL)
	overrideWithEnvVar("NOZZLE_INFLUXDBDATABASE", &config.InfluxDbDatabase)
	overrideWithEnvVar("NOZZLE_INFLUXDBUSER", &config.InfluxDbUser)
	overrideWithEnvVar("NOZZLE_INFLUXDBPASSWORD", &config.InfluxDbPassword)
	overrideWithEnvBool("NOZZLE_INFLUXDBALLOWSELFSIGNED", &config.InfluxDbAllowSelfSigned)

	overrideWithEnvVar("NOZZLE_METRICPREFIX", &config.MetricPrefix)
	overrideWithEnvVar("NOZZLE_DEPLOYMENT", &config.Deployment)

	overrideWithEnvUint32("NOZZLE_FLUSHDURATIONSECONDS", &config.FlushDurationSeconds)

	overrideWithEnvBool("NOZZLE_INSECURESSLSKIPVERIFY", &config.InsecureSSLSkipVerify)
	overrideWithEnvBool("NOZZLE_DISABLEACCESSCONTROL", &config.DisableAccessControl)
	overrideWithEnvUint32("NOZZLE_IDLETIMEOUTSECONDS", &config.IdleTimeoutSeconds)
	return &config, nil
}

func overrideWithEnvVar(name string, value *string) {
	envValue := os.Getenv(name)
	if envValue != "" {
		*value = envValue
	}
}

func overrideWithEnvUint32(name string, value *uint32) {
	envValue := os.Getenv(name)
	if envValue != "" {
		tmpValue, err := strconv.Atoi(envValue)
		if err != nil {
			panic(err)
		}
		*value = uint32(tmpValue)
	}
}

func overrideWithEnvBool(name string, value *bool) {
	envValue := os.Getenv(name)
	if envValue != "" {
		var err error
		*value, err = strconv.ParseBool(envValue)
		if err != nil {
			panic(err)
		}
	}
}
