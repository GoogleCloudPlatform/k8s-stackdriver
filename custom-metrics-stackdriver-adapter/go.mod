module github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter

go 1.16

require (
	cloud.google.com/go v0.84.0
	github.com/prometheus/client_golang v1.11.0
	golang.org/x/net v0.0.0-20210520170846-37e1c6afe023
	golang.org/x/oauth2 v0.0.0-20210628180205-a41e5a781914
	google.golang.org/api v0.50.0
	k8s.io/api v0.22.0
	k8s.io/apimachinery v0.22.0
	k8s.io/client-go v0.22.0
	k8s.io/component-base v0.22.0
	k8s.io/klog v1.0.0
	k8s.io/metrics v0.22.0
	sigs.k8s.io/custom-metrics-apiserver v1.22.0
	sigs.k8s.io/metrics-server v0.5.0
)
