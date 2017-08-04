#!/bin/bash
kubectl create -f adapter.yaml
kubectl proxy & 
sleep 10
curl localhost:8001
curl localhost:8001//apis/v1events/v1alpha1/namespaces/default/events/blah
kubectl delete -f adapter.yaml
kill %1
