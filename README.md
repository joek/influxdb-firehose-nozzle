# Sumary
The Influxdb-firehose-nozzle is a CF component forwarding metrics from Logregator Firehose to Influxdb.

## Configure CloudFoundry UAA for Firehose Nozzle

The influxdb firehose nozzle requires a UAA user who is authorized to access the loggregator firehose. You can add a user by editing your CloudFoundry manifest to include the details about this user under the properties.uaa.clients section. For example to add a user `influxdb-firehose-nozzle`:

```
properties:
  uaa:
    clients:
      influxdb-firehose-nozzle:
        access-token-validity: 1209600
        authorized-grant-types: authorization_code,client_credentials,refresh_token
        override: true
        secret: <password>
        scope: openid,oauth.approvals,doppler.firehose
        authorities: oauth.login,doppler.firehose
```


## Running

The influxdb nozzle uses a configuration file to obtain the firehose URL, influxdb API key and other configuration parameters. The firehose and the influxdb servers both require authentication.

You can start the firehose nozzle by executing:
```
go run main.go -config config/firehose-nozzle-config.json"
```

## Batching

The configuration file specifies the interval at which the nozzle will flush metrics to influxdb. By default this is set to 15 seconds.

## `slowConsumerAlert`
For the most part, the influxdb-firehose-nozzle forwards metrics from the loggregator firehose to influxdb without too much processing. A notable exception is the `slowConsumerAlert` metric. The metric is a binary value (0 or 1) indicating whether or not the nozzle is forwarding metrics to influxdb at the same rate that it is receiving them from the firehose: `0` means the the nozzle is keeping up with the firehose, and `1` means that the nozzle is falling behind.

The nozzle determines the value of `slowConsumerAlert` with the following rules:

1. **When the nozzle receives a `TruncatingBuffer.DroppedMessages` metric, it publishes the value `1`.** The metric indicates that Doppler determined that the client (in this case, the nozzle) could not consume messages as quickly as the firehose was sending them, so it dropped messages from its queue of messages to send.

2. **When the nozzle receives a websocket Close frame with status `1008`, it publishes the value `1`.** Traffic Controller pings clients to determine if the connections are still alive. If it does not receive a Pong response before the KeepAlive deadline, it decides that the connection is too slow (or even dead) and sends the Close frame.

3. **Otherwise, the nozzle publishes `0`.**

## Tests

You need [ginkgo](http://onsi.github.io/ginkgo/) to run the tests. The tests can be executed by:
```
ginkgo -r

```
