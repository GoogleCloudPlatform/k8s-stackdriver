module github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter

go 1.13

require (
	cloud.google.com/go v0.56.0
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf // indirect
	github.com/coreos/pkg v0.0.0-20180928190104-399ea9e2e55f // indirect
	github.com/emicklei/go-restful v2.12.0+incompatible // indirect
	github.com/emicklei/go-restful-swagger12 v0.0.0-20170926063155-7524189396c6 // indirect
	github.com/evanphx/json-patch v4.5.0+incompatible // indirect
	github.com/go-openapi/spec v0.19.7 // indirect
	github.com/go-openapi/swag v0.19.9 // indirect
	github.com/golang/protobuf v1.4.0 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/imdario/mergo v0.3.9 // indirect
	github.com/kubernetes-incubator/custom-metrics-apiserver v0.0.0-20200323093244-5046ce1afe6b
	github.com/mailru/easyjson v0.7.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.5.1
	github.com/prometheus/procfs v0.0.11 // indirect
	go.uber.org/zap v1.14.1 // indirect
	golang.org/x/crypto v0.0.0-20200422194213-44a606286825 // indirect
	golang.org/x/net v0.0.0-20200421231249-e086a090c8fd
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	golang.org/x/sys v0.0.0-20200420163511-1957bb5e6d1f // indirect
	golang.org/x/time v0.0.0-20200416051211-89c76fbcd5d1 // indirect
	google.golang.org/api v0.20.0
	google.golang.org/appengine v1.6.6 // indirect
	google.golang.org/genproto v0.0.0-20200420144010-e5e8543f8aeb // indirect
	k8s.io/api v0.17.5
	k8s.io/apimachinery v0.17.5
	k8s.io/apiserver v0.17.5 // indirect
	k8s.io/client-go v0.17.5
	k8s.io/component-base v0.17.5
	k8s.io/klog v1.0.0
	k8s.io/metrics v0.17.5
	k8s.io/utils v0.0.0-20200414100711-2df71ebbae66 // indirect
)

replace (
	cloud.google.com/go => cloud.google.com/go v0.56.0
	github.com/NYTimes/gziphandler => github.com/NYTimes/gziphandler v1.1.1
	github.com/blang/semver => github.com/blang/semver v3.5.1+incompatible
	github.com/coreos/go-systemd => github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf
	github.com/coreos/pkg => github.com/coreos/pkg v0.0.0-20180928190104-399ea9e2e55f
	github.com/emicklei/go-restful => github.com/emicklei/go-restful v2.12.0+incompatible
	github.com/emicklei/go-restful-swagger12 => github.com/emicklei/go-restful-swagger12 v0.0.0-20170926063155-7524189396c6
	github.com/evanphx/json-patch => github.com/evanphx/json-patch v4.5.0+incompatible
	github.com/go-openapi/spec => github.com/go-openapi/spec v0.19.7
	github.com/go-openapi/swag => github.com/go-openapi/swag v0.19.9
	github.com/golang/protobuf => github.com/golang/protobuf v1.4.0
	github.com/hashicorp/golang-lru => github.com/hashicorp/golang-lru v0.5.4
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.9
	github.com/kubernetes-incubator/custom-metrics-apiserver => github.com/kubernetes-incubator/custom-metrics-apiserver v0.0.0-20200323093244-5046ce1afe6b
	github.com/mailru/easyjson => github.com/mailru/easyjson v0.7.1
	github.com/pkg/errors => github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v1.5.1
	github.com/prometheus/procfs => github.com/prometheus/procfs v0.0.11
	go.uber.org/zap => go.uber.org/zap v1.14.1
	golang.org/x/crypto => golang.org/x/crypto v0.0.0-20200422194213-44a606286825
	golang.org/x/net => golang.org/x/net v0.0.0-20200421231249-e086a090c8fd
	golang.org/x/oauth2 => golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	golang.org/x/sys => golang.org/x/sys v0.0.0-20200420163511-1957bb5e6d1f
	golang.org/x/time => golang.org/x/time v0.0.0-20200416051211-89c76fbcd5d1
	google.golang.org/api => google.golang.org/api v0.16.0
	google.golang.org/appengine => google.golang.org/appengine v1.6.6
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20200420144010-e5e8543f8aeb
	google.golang.org/grpc => google.golang.org/grpc v1.21.1
	k8s.io/api => k8s.io/api v0.17.5
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.5
	k8s.io/apiserver => k8s.io/apiserver v0.17.5
	k8s.io/client-go => k8s.io/client-go v0.17.5
	k8s.io/component-base => k8s.io/component-base v0.17.5
	k8s.io/klog => k8s.io/klog v1.0.0
	k8s.io/metrics => k8s.io/metrics v0.17.5
	k8s.io/utils => k8s.io/utils v0.0.0-20200414100711-2df71ebbae66
)
