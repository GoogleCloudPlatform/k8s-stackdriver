#!/bin/sh

SLEEP_SECONDS=${SLEEP_SECONDS:-60}
NAMESPACE=${NAMESPACE:-kube-system}
DEFAULT_CPU_REQUEST=${CPU_REQUEST}
DEFAULT_MEMORY_REQUEST=${MEMORY_REQUEST}
DEFAULT_CPU_LIMIT=${CPU_LIMIT}
DEFAULT_MEMORY_LIMIT=${MEMORY_LIMIT}

reset_to_defaults() {
  REQUESTS=
  LIMITS=
  CPU_REQUEST=${DEFAULT_CPU_REQUEST}
  MEMORY_REQUEST=${DEFAULT_MEMORY_REQUEST}
  CPU_LIMIT=${DEFAULT_CPU_LIMIT}
  MEMORY_LIMIT=${DEFAULT_MEMORY_LIMIT}
}

apply_scaling() {
  if ! kubectl get scalingpolicies -n ${NAMESPACE} ${SCALING_POLICY}
  then
    echo "${SCALING_POLICY} not found in namespace ${NAMESPACE}, using defaults."
    exit 0
  fi
  for resource_class in request limit
  do
    for resource_type in cpu memory
    do
      TMP=$(kubectl get scalingpolicies -n ${NAMESPACE} ${SCALING_POLICY} \
        -o=jsonpath="{.spec.containers[?(@.name=='fluentd-gcp')].resources.${resource_class}s[?(@.resource=='${resource_type}')].base}")
      if [ ${TMP} ]; then
        # Build the right variable name from resource_type and resource_class
        # and assign it.
        export $(echo ${resource_type}_${resource_class} | awk '{print toupper($0)}')=${TMP}
      fi
    done
  done
}

# $1 is the resource to update (cpu/memory).
# $2 is the value to add, if any.
add_request() {
  RESOURCE=$1
  VALUE=$2
  if [ ${VALUE} ]; then
    if [ -z ${REQUESTS} ]; then
      REQUESTS='--requests='
    else
      REQUESTS=${REQUESTS},
    fi
    REQUESTS=${REQUESTS}${RESOURCE}=${VALUE}
  fi
}

# $1 is the resource to update (cpu/memory).
# $2 is the value to add, if any.
add_limit() {
  RESOURCE=$1
  VALUE=$2
  if [ ${VALUE} ]; then
    if [ -z ${LIMITS} ]; then
      LIMITS='--limits='
    else
      LIMITS=${LIMITS},
    fi
    LIMITS=${LIMITS}${RESOURCE}=${VALUE}
  fi
}

build_flags() {
  add_request cpu ${CPU_REQUEST}
  add_request memory ${MEMORY_REQUEST}
  add_limit cpu ${CPU_LIMIT}
  add_limit memory ${MEMORY_LIMIT}
}

if [ -z ${DS_NAME} ]; then
  echo "DaemonSet name has to be set via DS_NAME variable." >&2
  exit 1
fi

if [ -z ${SCALING_POLICY} ]; then
  echo "Scaling policy name was not specified." >&2
  exit 1
fi

while true
do
  reset_to_defaults
  apply_scaling
  build_flags
  if [ ${REQUESTS} ] || [ ${LIMITS} ]
  then
    echo "Running: kubectl set resources -n ${NAMESPACE} ds ${DS_NAME} ${REQUESTS} ${LIMITS}"
    kubectl set resources -n ${NAMESPACE} ds ${DS_NAME} ${REQUESTS} ${LIMITS}
  else
    echo "No requests nor limits provided, doing nothing and going back to sleep."
  fi
  sleep ${SLEEP_SECONDS}
done

