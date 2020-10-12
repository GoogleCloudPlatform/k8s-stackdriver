module github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter

go 1.13

require (
	cloud.google.com/go v0.40.0
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/beorn7/perks v0.0.0-00010101000000-000000000000 // indirect
	github.com/coreos/etcd v3.3.13+incompatible // indirect
	github.com/coreos/go-systemd v0.0.0-20190620071333-e64a0ec8b42a // indirect
	github.com/emicklei/go-restful v2.9.6+incompatible // indirect
	github.com/emicklei/go-restful-swagger12 v0.0.0-20170208215640-dcef7f557305 // indirect
	github.com/go-openapi/jsonreference v0.19.2 // indirect
	github.com/gogo/protobuf v1.2.1 // indirect
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/google/gofuzz v1.0.0 // indirect
	github.com/google/uuid v1.1.1 // indirect
	github.com/googleapis/gnostic v0.3.0 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/json-iterator/go v0.0.0-00010101000000-000000000000 // indirect
	github.com/kubernetes-incubator/custom-metrics-apiserver v0.0.0-20190703094830-abe433176c52
	github.com/mailru/easyjson v0.0.0-20190620125010-da37f6c1e481 // indirect
	github.com/modern-go/concurrent v0.0.0-00010101000000-000000000000 // indirect
	github.com/modern-go/reflect2 v0.0.0-00010101000000-000000000000 // indirect
	github.com/munnerz/goautoneg v0.0.0-20190414153302-2ae31c8b6b30 // indirect
	github.com/pborman/uuid v0.0.0-20180906182336-adf5a7427709 // indirect
	github.com/prometheus/client_golang v1.0.0
	github.com/prometheus/common v0.6.0 // indirect
	github.com/prometheus/procfs v0.0.0-00010101000000-000000000000 // indirect
	go.opencensus.io v0.22.0 // indirect
	golang.org/x/crypto v0.0.0-20190621222207-cc06ce4a13d4 // indirect
	golang.org/x/net v0.0.0-20190620200207-3b0461eec859
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4 // indirect
	google.golang.org/api v0.6.0
	google.golang.org/appengine v1.6.1 // indirect
	google.golang.org/genproto v0.0.0-20190620144150-6af8c5fc6601 // indirect
	google.golang.org/grpc v1.21.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.1.0 // indirect
	k8s.io/api v0.17.3
	k8s.io/apimachinery v0.17.3
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/component-base v0.17.3
	k8s.io/klog v1.0.0
	k8s.io/metrics v0.17.3
	k8s.io/utils v0.0.0-20190607212802-c55fbcfc754a // indirect
	sigs.k8s.io/metrics-server v0.3.7
	sigs.k8s.io/structured-merge-diff v0.0.0-20190302045857-e85c7b244fd2 // indirect
)

replace (
	cloud.google.com/go => cloud.google.com/go v0.40.0
	github.com/NYTimes/gziphandler => github.com/NYTimes/gziphandler v1.1.1
	github.com/beorn7/perks => github.com/beorn7/perks v1.0.0
	github.com/coreos/etcd => github.com/coreos/etcd v3.3.13+incompatible
	github.com/coreos/go-systemd => github.com/coreos/go-systemd v0.0.0-20190620071333-e64a0ec8b42a
	github.com/emicklei/go-restful => github.com/emicklei/go-restful v2.9.6+incompatible
	github.com/emicklei/go-restful-swagger12 => github.com/emicklei/go-restful-swagger12 v0.0.0-20170208215640-dcef7f557305
	github.com/evanphx/json-patch => github.com/evanphx/json-patch v4.5.0+incompatible
	github.com/go-openapi/jsonreference => github.com/go-openapi/jsonreference v0.19.2
	github.com/go-openapi/spec => github.com/go-openapi/spec v0.0.0-20180710175419-bce47c9386f9
	github.com/gogo/protobuf => github.com/gogo/protobuf v1.2.1
	github.com/golang/groupcache => github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef
	github.com/google/gofuzz => github.com/google/gofuzz v1.0.0
	github.com/google/uuid => github.com/google/uuid v1.1.1
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.3.0
	github.com/grpc-ecosystem/go-grpc-prometheus => github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.7
	github.com/json-iterator/go => github.com/json-iterator/go v1.1.6
	github.com/kubernetes-incubator/custom-metrics-apiserver => github.com/kubernetes-incubator/custom-metrics-apiserver v0.0.0-20190703094830-abe433176c52
	github.com/mailru/easyjson => github.com/mailru/easyjson v0.0.0-20190620125010-da37f6c1e481
	github.com/modern-go/concurrent => github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd
	github.com/modern-go/reflect2 => github.com/modern-go/reflect2 v0.0.0-20180701023420-4b7aa43c6742
	github.com/munnerz/goautoneg => github.com/munnerz/goautoneg v0.0.0-20190414153302-2ae31c8b6b30
	github.com/pborman/uuid => github.com/pborman/uuid v0.0.0-20180906182336-adf5a7427709
	github.com/pkg/errors => github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.8.0
	github.com/prometheus/common => github.com/prometheus/common v0.6.0
	github.com/prometheus/procfs => github.com/prometheus/procfs v0.0.2
	github.com/spf13/pflag => github.com/spf13/pflag v1.0.3
	go.opencensus.io => go.opencensus.io v0.22.0
	golang.org/x/crypto => golang.org/x/crypto v0.0.0-20190621222207-cc06ce4a13d4
	golang.org/x/net => golang.org/x/net v0.0.0-20190620200207-3b0461eec859
	golang.org/x/oauth2 => golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sys => golang.org/x/sys v0.0.0-20190624142023-c5567b49c5d0
	golang.org/x/time => golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	google.golang.org/api => google.golang.org/api v0.6.0
	google.golang.org/appengine => google.golang.org/appengine v1.6.1
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190620144150-6af8c5fc6601
	google.golang.org/grpc => google.golang.org/grpc v1.21.1
	gopkg.in/inf.v0 => gopkg.in/inf.v0 v0.9.1
	gopkg.in/natefinch/lumberjack.v2 => gopkg.in/natefinch/lumberjack.v2 v2.0.0-20170531160350-a96e63847dc3
	k8s.io/api => k8s.io/api v0.0.0-20190313235455-40a48860b5ab
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/apiserver => k8s.io/apiserver v0.0.0-20190313205120-8b27c41bdbb1
	k8s.io/client-go => k8s.io/client-go v11.0.0+incompatible
	k8s.io/component-base => k8s.io/component-base v0.0.0-20190314000054-4a91899592f4
	k8s.io/klog => k8s.io/klog v0.3.3
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20190228160746-b3a7cee44a30
	k8s.io/kubernetes/pkg/kubelet/apis/stats/v1alpha1 => ./localvendor/k8s.io/kubernetes/pkg/kubelet/apis/stats/v1alpha1
	k8s.io/metrics => k8s.io/metrics v0.0.0-20190314001731-1bd6a4002213
	k8s.io/utils => k8s.io/utils v0.0.0-20190607212802-c55fbcfc754a
	sigs.k8s.io/metrics-server v0.3.7 => sigs.k8s.io/metrics-server v0.0.0-20200406215547-5fcf6956a533
	sigs.k8s.io/structured-merge-diff => sigs.k8s.io/structured-merge-diff v0.0.0-20190302045857-e85c7b244fd2
	sigs.k8s.io/yaml => sigs.k8s.io/yaml v1.1.0
)
