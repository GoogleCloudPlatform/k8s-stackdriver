# Event Exporter

This tool is used to export Kubernetes events. It effectively runs a watch on
the apiserver, detecting as granular as possible all changes to the event
objects. Event exporter exports only to Stackdriver.

## Build

To build the binary, run

```shell
make build
```

To run unit tests, run

```shell
make test
```

To build the container, run

```shell
make container
```

## Run

Event exporter has following options:

```
-prometheus-endpoint string
    Endpoint on which to expose Prometheus http handler (default ":80")
-resync-period duration
    Reflector resync period (default 1m0s)
-sink-opts string
    Parameters for configuring sink
```

Set of flags for configuring sink is the following:

```
Usage of stackdriver:
  -flush-delay duration
      Delay after receiving the first event in batch before sending the request to Stackdriver, if batchdoesn't get sent before (default 5s)
  -max-buffer-size int
      Maximum number of events in the request to Stackdriver (default 100)
  -max-concurrency int
      Maximum number of concurrent requests to Stackdriver (default 10)
  -endpoint string
      Base path for Stackdriver API (default "https://logging.googleapis.com/")
```

## Deploy

Example deployment:

```yaml
apiVersion: apps/v1beta1
kind: Deployment
metadata:
  name: event-exporter-deployment
spec:
  replicas: 1
  template:
    metadata:
      labels:
        app: event-exporter
    spec:
      containers:
      - name: event-exporter
        image: gcr.io/google-containers/event-exporter:v0.1.4
        command:
        - '/event-exporter'
```

## Sharding

On large clusters a single replica may not fit in its memory limit, mostly
because of the cluster-wide pod metadata cache used for owner labels. Event
exporter can be sharded across multiple replicas:

```
-total-shards int
    Total number of event-exporter replicas (shards). Each event is exported
    by exactly one shard, chosen by hashing the involved object's UID. All
    replicas must run with the same value. 1 disables sharding. (default 1)
-shard-id int
    ID of this shard, in [0, total-shards). -1 derives the ID from the
    ordinal suffix of the pod hostname, which works for StatefulSet
    replicas. (default -1)
```

Deploy the shards as a StatefulSet with `replicas` equal to `-total-shards`;
each pod picks up its shard ID from its hostname ordinal. Each replica still
receives the full event and pod watch streams from the apiserver and filters
them locally, so sharding divides memory usage per replica but multiplies
apiserver watch load by the number of shards. When `-enable-pod-owner-label`
is on, the pod label cache is sharded by the same key (events are sharded by
involved object UID, which for pod events is the pod UID), so each replica
only caches the pods whose events it exports.

Note: changing `-total-shards` reassigns shard ownership, so events may be
duplicated or missed while the rollout is in progress.

## Notes
### ClusterRoleBinding
This pod's service account should be authorized to get events, you
might need to set up ClusterRoleBinding in order to make it possible. Complete
example with the service account and the cluster role binding you can find in
the `example` directory.
### "resourceVersion for the provided watch is too old"
On a system with few/no events, you may see "The resourceVersion for the provided
watch is too old" warnings. These can be ignored. This is due to compacted resource
versions being referenced.