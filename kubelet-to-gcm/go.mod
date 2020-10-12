module github.com/GoogleCloudPlatform/k8s-stackdriver/kubelet-to-gcm

go 1.13

require (
	cloud.google.com/go v0.58.0 // indirect
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/google/gofuzz v1.1.0
	github.com/prometheus/client_golang v1.7.0
	github.com/prometheus/common v0.10.0
	golang.org/x/net v0.0.0-20200602114024-627f9648deb9
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	google.golang.org/api v0.28.0
	k8s.io/apimachinery v0.17.3
	k8s.io/kubernetes/pkg/kubelet/apis/stats/v1alpha1 v0.0.0
)

replace (
	cloud.google.com/go => cloud.google.com/go v0.58.0
	github.com/gogo/protobuf => github.com/gogo/protobuf v1.3.1
	github.com/golang/glog => github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/google/gofuzz => github.com/google/gofuzz v1.1.0
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v1.7.0
	github.com/prometheus/common => github.com/prometheus/common v0.10.0
	golang.org/x/net => golang.org/x/net v0.0.0-20200602114024-627f9648deb9
	golang.org/x/oauth2 => golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	google.golang.org/api => google.golang.org/api v0.28.0
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.3
	k8s.io/kubernetes/pkg/kubelet/apis/stats/v1alpha1 => ./vendor/k8s.io/kubernetes/pkg/kubelet/apis/stats/v1alpha1
)
