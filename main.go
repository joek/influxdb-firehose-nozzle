package main

import (
	"flag"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"github.com/cloudfoundry-incubator/datadog-firehose-nozzle/logger"
	"github.com/cloudfoundry-incubator/datadog-firehose-nozzle/uaatokenfetcher"
	"github.com/joek/influxdb-firehose-nozzle/influxdbfirehosenozzle"
	"github.com/joek/influxdb-firehose-nozzle/nozzleconfig"
)

var (
	logFilePath = flag.String("logFile", "", "The agent log file, defaults to STDOUT")
	logLevel    = flag.Bool("debug", false, "Debug logging")
	configFile  = flag.String("config", "config/firehose-nozzle-config.json", "Location of the nozzle config json file")
)

func main() {
	flag.Parse()

	log := logger.NewLogger(*logLevel, *logFilePath, "influxdb-firehose-nozzle", "")

	config, err := nozzleconfig.Parse(*configFile)
	if err != nil {
		log.Fatalf("Error parsing config: %s", err.Error())
	}

	tokenFetcher := uaatokenfetcher.New(
		config.UAAURL,
		config.Username,
		config.Password,
		config.InsecureSSLSkipVerify,
		log,
	)

	threadDumpChan := registerGoRoutineDumpSignalChannel()
	defer close(threadDumpChan)
	go dumpGoRoutine(threadDumpChan)

	log.Infof("Targeting inluxdb URL: %s \n", config.InfluxDbURL)
	nozzle := influxdbfirehosenozzle.NewInfluxDBFirehoseNozzle(config, tokenFetcher, log)
	nozzle.Start()

}

func registerGoRoutineDumpSignalChannel() chan os.Signal {
	threadDumpChan := make(chan os.Signal, 1)
	signal.Notify(threadDumpChan, syscall.SIGUSR1)

	return threadDumpChan
}

func dumpGoRoutine(dumpChan chan os.Signal) {
	for range dumpChan {
		goRoutineProfiles := pprof.Lookup("goroutine")
		if goRoutineProfiles != nil {
			goRoutineProfiles.WriteTo(os.Stdout, 2)
		}
	}
}
