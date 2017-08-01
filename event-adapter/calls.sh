#!/bin/bash
kubectl create -f adapter.yaml
kubectl proxy & 
sleep 10
curl -k localhost:8001
curl -k localhost:8001//apis/v1events/v1alpha/namespaces/default/events/ere
curl -k localhost:8001
curl -k localhost:8001//apis/v1events/v1alpha/namespaces/default/event/ere
curl -k localhost:8001
curl -k localhost:8001//apis/v1events/v1alpha/namespces/default/events/ere
curl -k localhost:8001
curl -k localhost:8001//namespaces/default/events/ere 
kubectl delete -f adapter.yaml
kill %1
