#!/bin/bash

# Test cases
# Should be called from within a pod
# e.g. kubectl run -it --tty cmd --image=fedora /bin/sh
curl -k https://k8s-stackdriver-adapter:443/apis/custom-metrics.metrics.k8s.io/v1alpha1/namespaces/default/pods/*/kubernetes.io/memory/usage
curl -k https://k8s-stackdriver-adapter:443/apis/custom-metrics.metrics.k8s.io/v1alpha1/namespaces/default/pods/*/kubernetes.io/memory/usage?labelSelector=run%3Dk8s-stackdriver-adapter
curl -k https://k8s-stackdriver-adapter:443/apis/custom-metrics.metrics.k8s.io/v1alpha1/namespaces/default/metrics/kubernetes.io/cpu/request
curl -k https://k8s-stackdriver-adapter:443/apis/custom-metrics.metrics.k8s.io/v1alpha1/namespaces/*/kubernetes.io/cpu/request
# TODO
# Nodes are currently named as 'instances'
# curl -k https://k8s-stackdriver-adapter:443/apis/custom-metrics.metrics.k8s.io/v1alpha1/nodes/*/kubernetes.io/network/tx_rate
# curl -k https://k8s-stackdriver-adapter:443/apis/custom-metrics.metrics.k8s.io/v1alpha1/nodes/*/kubernetes.io/network/tx_rate?labelSelector=todo_label%3Dtodo-value
