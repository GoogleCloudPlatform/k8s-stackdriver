# Event Adapter
This tool allows users to read Kubernetes events from Stackdriver. This is  brief guide that explains how to use it.

# 1. Create a cluster ensuring that Stackdriver Logging API is enabled (both reading and writing)

Download Kubernetes and create a cluster with Stackdriver Logging enabled:
```sh
mkdir -p $GOPATH/src/k8s.io
cd $GOPATH/src/k8s.io
git clone git@github.com:kubernetes/kubernetes.git
cd kubernetes
git checkout 9aef242a4c1e423270488206670d3d1f23eaab52
sed -i 's/logging-write/https:\/\/www.googleapis.com\/auth\/logging.admin/g' cluster/gce/config-common.sh
make quick-release
./cluster/kube-up.sh
```

# 2. Install the API

Clone the project "https://github.com/GoogleCloudPlatform/k8s-stackdriver" into your GOPATH.
```sh
mkdir -p $GOPATH/src/github.com/GoogleCloudPlatform
cd $GOPATH/src/github.com/GoogleCloudPlatform
git clone git@github.com:GoogleCloudPlatform/k8s-stackdriver.git
cd k8s-stackdriver
git checkout 692c11f5395b3b1d321d8384cde56a4112b5696d
cd k8s-stackdriver/event-adapter
```
Now, you need to set it to work with your project. You should define a variable that identify your project GCP_PROJECT=myProject.
```sh
sed -i 's/google-containers/'"$GCP_PROJECT"'/g’adapter.yaml
make push GCP_PROJECT
kubectl create -f adapter.yaml --validate=false
```
The adapter.yaml file specifies the source of Event Adapter Docker image, and Makefile describes the destination where the image you build is going to be pushed.
Every time you change one of these values, you should change the other one accordingly.
Calling "make push", it will compile the code and push the image.
Then call "kubectl create -f adapter.yaml --validate=false", it will require a bit (~1 minute) to install everything.
You can check everything is installed:
```sh
kubectl api-versions | grep v1events/v1alpha1
```
If you need to re-install it, then you will have to delete it using "kubectl delete -f adapter.yaml".
```sh
kubectl delete -f adapter.yaml
make push GCP_PROJECT
kubectl create -f adapter.yaml --validate=false
```

# 3. Create a proxy
Create a proxy so you can conveniently query API Server from your workstation. You can specify the port on which it will serve:
```sh
KUBE_PORT=8001
kubectl proxy --port=${KUBE_PORT}
```

# 4. Use the API
The API enables 3 differents URLs in the API group /apis/v1events/v1alpha1. If you have the proxy on, you can see them with:
```sh
curl http://localhost:${KUBE_PORT}/apis/v1events/v1alpha1
```
Now you can see:
* namespaces/{namespace}/events/{eventName}
    * GET : it will allow you to retrieve from Stackdriver a single event giving the name {eventName} and namespace {namespace} in which it occurs.
 * namespaces/{namespace}/events
    * GET : it will allow you to retrieve from Stackdriver a list of events with the given namespace {namespace} in which them occurs.
    * POST : it is not implemented yet, it should allow you to store in Stackdriver an event in the given namespace {namespace}.
 * events
    * GET : it will retrieve from Stackdriver a list of events.

For example, calling:
 ```sh
curl -X GET http://localhost:${KUBE_PORT}/apis/v1events/v1alpha1/namespaces/default/events
 ```
you will get something like this:
```
{
  "kind": "EventList",
  "apiVersion": "v1events/v1alpha1",
  "metadata": {
	"selfLink": "/apis/v1events/v1alpha1/namespaces/default/events"
  },
  "items": [
	{
	  "kind": "Event",
	  "apiVersion": "v1",
	  "metadata": {
		"name": "event-adapter-3051674978-3mt16.14e3920fe82c7b4c",
		"namespace": "default",
		"selfLink": "/apis/v1events/v1alpha1/namespaces/default/events",
		"uid": "ea55efed-9799-11e7-be52-42010a800002",
		"resourceVersion": "22248",
		"creationTimestamp": "2017-09-12T09:08:17Z"
	  },
	  "involvedObject": {
		"kind": "Pod",
		"namespace": "default",
		"name": "event-adapter-3051674978-3mt16",
		"uid": "e7756e8d-9799-11e7-be52-42010a800002",
		"apiVersion": "v1",
		"resourceVersion": "2290232",
		"fieldPath": "spec.containers{pod-event-adapter}"
	  },
	  "reason": "Pulled",
	  "message": "Successfully pulled image \"gcr.io/erocchi-gke-dev-1attempt/event-adapter:1.0\"",
	  "source": {
		"component": "kubelet",
		"host": "kubernetes-minion-group-lbfg"
	  },
	  "firstTimestamp": "2017-09-12T09:08:17Z",
	  "lastTimestamp": "2017-09-12T09:08:17Z",
	  "count": 1,
	  "type": "Normal"
	},

	...
	]
}
```
Other examples may be:
```sh
curl -X GET http://localhost:${KUBE_PORT}/apis/v1events/v1alpha1/events
```
You can also retrieve a single event, you should set EVENT_NAME variables equal to the name of the event. Then call:
```sh
curl -X GET http://localhost:${KUBE_PORT}/apis/v1events/v1alpha1/namespaces/default/events/$EVENT_NAME
```
Then, there is the one that is not supported yet.
```sh
curl -X POST http://localhost:${KUBE_PORT}/apis/v1events/v1alpha1/namespaces/default/events
```
There are flags you can use if you want to limit the events you are going to retrieve.
* ***max-retrieved-events=N***
If you want to retrieve at most N events. The default of 100 events will be used.
* ***retrieve-events-since-millis=T***
It will retrieve only events that were logged after timestamp T (number of milliseconds after epoch -- 1 January 1970). The default of last 1h will be used.
If T < 0 the default option will be used as well.

In order to change these flags, modify the adapter.yaml file.
You should change the deployment declaration, for example:
<pre>
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: event-adapter
  namespace: default
  labels:
run: event-adapter
	k8s-app: event-adapter
spec:
  replicas: 1
  selector:
	matchLabels:
	  run: event-adapter
	  k8s-app: event-adapter
  template:
	metadata:
	  labels:
		run: event-adapter8001
		k8s-app: event-adapter
		kubernetes.io/cluster-service: "true"
	spec:
	  containers:
		- image: gcr.io/google-containers/event-adapter:1.0
		  imagePullPolicy: Always
		  name: pod-event-adapter
		  command:
		    - /adapter
		    <b>- --max-retrieved-events=123
		    - --retrieve-events-since-millis=0 </b>
		  resources:
 		      cpu: 250m
		      memory: 200Mi
		    requests:
		      cpu: 250m
		      memory: 200Mi
</pre>
This will allow you to retrieve at most 123 events that happens after epoch (T = 0 milliseconds).

