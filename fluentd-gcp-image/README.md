# Collecting Docker Log Files with Fluentd and sending to GCP.

This directory contains the source files needed to make a Docker image
that collects Docker container log files using [Fluentd][fluentd]
and sends them to [Stackdriver Logging][stackdriverLogging].

This image is designed to be used as part of the [Kubernetes][kubernetes]
cluster bring up process. The image resides at GCR under the name
[gcr.io/google-containers/fluentd-gcp][image].

# Usage

The image is built with its own set of plugins which you can later use
in the configuration. The set of plugin is enumerated in a Gemfile in the
image's directory.

In order to configure fluentd image, you should mount a directory with `.conf`
files to `/etc/fluent/config.d` or add files to that directory by building
a new image on top. All `.conf` files in the `/etc/fluent/config.d` directory
will be included to the final fluentd configuration. You can find details about
fluentd configuration [in the Fluentd documentation][fluentdDocs].

Command line arguments to the fluentd executable are passed via environment
variable `FLUENTD_ARGS`.

[fluentd]: http://www.fluentd.org
[kubernetes]: https://github.com/kubernetes/kubernetes
[stackdriverLogging]: https://cloud.google.com/logging
[image]: https://gcr.io/google-containers/fluentd-gcp
[fluentdDocs]: http://docs.fluentd.org/articles/config-file
