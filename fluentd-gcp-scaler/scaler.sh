#!/bin/sh

SLEEP_SECONDS=${SLEEP_SECONDS:-60}
NAMESPACE=${NAMESPACE:-kube-system}
DEFAULT_CPU_REQUEST=${CPU_REQUEST:-}
DEFAULT_MEMORY_REQUEST=${MEMORY_REQUEST:-}
DEFAULT_CPU_LIMIT=${CPU_LIMIT:-}
DEFAULT_MEMORY_LIMIT=${MEMORY_LIMIT:-}

reset_to_defaults() {
  REQUESTS_FLAG=
  LIMITS_FLAG=
  CPU_REQUEST=${DEFAULT_CPU_REQUEST}
  MEMORY_REQUEST=${DEFAULT_MEMORY_REQUEST}
  CPU_LIMIT=${DEFAULT_CPU_LIMIT}
  MEMORY_LIMIT=${DEFAULT_MEMORY_LIMIT}
}

log() {
  echo $(date -Ins) $*
}

apply_scaling() {
  # This is assuming there is a ScalingPolicy installed in the cluster.
  # See https://github.com/justinsb/scaler for more details.
  if ! kubectl get scalingpolicies -n ${NAMESPACE} ${SCALING_POLICY} 2> /dev/null
  then
    return
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
    if [ -z ${REQUESTS_FLAG} ]; then
      REQUESTS_FLAG='--requests='
    else
      REQUESTS_FLAG=${REQUESTS_FLAG},
    fi
    REQUESTS_FLAG=${REQUESTS_FLAG}${RESOURCE}=${VALUE}
  fi
}

# $1 is the resource to update (cpu/memory).
# $2 is the value to add, if any.
add_limit() {
  RESOURCE=$1
  VALUE=$2
  if [ ${VALUE} ]; then
    if [ -z ${LIMITS_FLAG} ]; then
      LIMITS_FLAG='--limits='
    else
      LIMITS_FLAG=${LIMITS_FLAG},
    fi
    LIMITS_FLAG=${LIMITS_FLAG}${RESOURCE}=${VALUE}
  fi
}

build_flags() {
  add_request cpu ${CPU_REQUEST}
  add_request memory ${MEMORY_REQUEST}
  add_limit cpu ${CPU_LIMIT}
  add_limit memory ${MEMORY_LIMIT}
}

# $1 is {limits,requests}.{cpu,memory}
# $2 is the desired value
needs_update() {
  CURRENT=$(kubectl get ds -n kube-system ${DS_NAME} -o \
    jsonpath="{.spec.template.spec.containers[?(@.name=='fluentd-gcp')].resources.$1}")
  DESIRED=$2
  if [ "${CURRENT}" != "${DESIRED}" ]
  then
    log "$1 needs updating. Is: '${CURRENT}', want: '${DESIRED}'."
    return 0
  fi
  return 1
}

update_if_needed() {
  NEED_UPDATE=false
  if needs_update requests.cpu ${CPU_REQUEST}; then NEED_UPDATE=true; fi
  if needs_update requests.memory ${MEMORY_REQUEST}; then NEED_UPDATE=true; fi
  if needs_update limits.cpu ${CPU_LIMIT}; then NEED_UPDATE=true; fi
  if needs_update limits.memory ${MEMORY_LIMIT}; then NEED_UPDATE=true; fi
  if ! ${NEED_UPDATE}
  then
    return
  fi
  if [ ${REQUESTS_FLAG} ] || [ ${LIMITS_FLAG} ]
  then
    KUBECTL_CMD="kubectl set resources -n ${NAMESPACE} ds ${DS_NAME} -c fluentd-gcp ${REQUESTS_FLAG} ${LIMITS_FLAG}"
    log "Running: ${KUBECTL_CMD}"
    ${KUBECTL_CMD}
  fi
}

while test $# -gt 0; do
  case "$1" in
    --ds-name=*)
      export DS_NAME=$(echo $1 | sed -e 's/^[^=]*=//g')
      if [ -z ${DS_NAME} ]; then
        log "Missing DaemonSet name in --ds-name flag." >&2
        exit 1
      fi
      shift
      ;;
    --scaling-policy=*)
      export SCALING_POLICY=$(echo $1 | sed -e 's/^[^=]*=//g')
      if [ -z ${SCALING_POLICY} ]; then
        log "Missing ScalingPolicy name in --scaling-policy flag." >&2
        exit 1
      fi
      shift
      ;;
    *)
      log "Unrecognized argument $1." >&2
      exit 1
      ;;
  esac
done

if [ -z ${DS_NAME} ]; then
  log "DaemonSet name has to be set via --ds-name flag." >&2
  exit 1
fi

if [ -z ${SCALING_POLICY} ]; then
  log "ScalingPolicy name has to be set via --scaling-policy flag." >&2
  exit 1
fi

while true
do
  reset_to_defaults
  apply_scaling
  build_flags
  update_if_needed
  sleep ${SLEEP_SECONDS}
done

