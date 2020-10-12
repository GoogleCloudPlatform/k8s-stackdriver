module github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter

go 1.13

require (
	cloud.google.com/go v0.58.0
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/google/go-cmp v0.5.0 // indirect
	github.com/googleapis/gnostic v0.4.0 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/prometheus/client_golang v1.7.0
	github.com/stretchr/testify v1.5.1 // indirect
	go.opencensus.io v0.22.4 // indirect
	golang.org/x/crypto v0.0.0-20200604202706-70a84ac30bf9 // indirect
	golang.org/x/net v0.0.0-20200602114024-627f9648deb9
	golang.org/x/sys v0.0.0-20200620081246-981b61492c35 // indirect
	golang.org/x/text v0.3.3 // indirect
	golang.org/x/time v0.0.0-20200416051211-89c76fbcd5d1 // indirect
	google.golang.org/api v0.28.0
	google.golang.org/genproto v0.0.0-20200622133129-d0ee0c36e670 // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	gopkg.in/yaml.v2 v2.3.0 // indirect
	k8s.io/api v0.17.3
	k8s.io/apimachinery v0.17.3
	k8s.io/client-go v0.17.3
)

replace (
	cloud.google.com/go => cloud.google.com/go v0.58.0
	github.com/golang/glog => github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/google/go-cmp => github.com/google/go-cmp v0.5.0
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.4.0
	github.com/hashicorp/golang-lru => github.com/hashicorp/golang-lru v0.5.4
	github.com/niemeyer/pretty => github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v1.7.0
	github.com/stretchr/testify => github.com/stretchr/testify v1.5.1
	go.opencensus.io => go.opencensus.io v0.22.4
	golang.org/x/crypto => golang.org/x/crypto v0.0.0-20200604202706-70a84ac30bf9
	golang.org/x/net => golang.org/x/net v0.0.0-20200602114024-627f9648deb9
	golang.org/x/sys => golang.org/x/sys v0.0.0-20200620081246-981b61492c35
	golang.org/x/text => golang.org/x/text v0.3.3
	golang.org/x/time => golang.org/x/time v0.0.0-20200416051211-89c76fbcd5d1
	google.golang.org/api => google.golang.org/api v0.28.0
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20200622133129-d0ee0c36e670
	gopkg.in/check.v1 => gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f
	gopkg.in/yaml.v2 => gopkg.in/yaml.v2 v2.3.0
	k8s.io/api => k8s.io/api v0.17.3
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.3
	k8s.io/client-go => k8s.io/client-go v0.17.3
)
