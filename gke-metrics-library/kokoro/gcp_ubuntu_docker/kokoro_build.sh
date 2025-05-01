#!/bin/bash

# Fail on any error.
set -e

# Display commands being run.
# WARNING: please only enable 'set -x' if necessary for debugging, and be very
#  careful if you handle credentials (e.g. from Keystore) with 'set -x':
#  statements like "export VAR=$(cat /tmp/keystore/credentials)" will result in
#  the credentials being printed in build logs.
#  Additionally, recursive invocation with credentials as command-line
#  parameters, will print the full command, with credentials, in the build logs.
set -x

bash "/usr/local/bin/use_go.sh" 1.20
go version

# Code under repo is checked out to ${KOKORO_ARTIFACTS_DIR}/git.
# The final directory name in this path is determined by the scm name specified
# in the job configuration.
# KOKORO_ARTIFACTS_DIR is populated by kokoro using
# http://google3/devtools/kokoro/config/prod/gke-metrics/gke-metrics-library/common.cfg
cd "${KOKORO_ARTIFACTS_DIR}/git/gke-metrics-library"
make build
make test
make coverage
