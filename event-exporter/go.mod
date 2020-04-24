module github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter

go 1.13

require (
	cloud.google.com/go v0.38.0
	github.com/PuerkitoBio/purell v1.0.0 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20160726150825-5bd2802263f2 // indirect
	github.com/beorn7/perks v0.0.0-20160804104726-4c0e84591b9a // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/emicklei/go-restful v1.1.4-0.20170410110728-ff4f55a20633 // indirect
	github.com/emicklei/go-restful-swagger12 v0.0.0-20170208215640-dcef7f557305 // indirect
	github.com/ghodss/yaml v0.0.0-20150909031657-73d445a93680 // indirect
	github.com/go-openapi/analysis v0.0.0-20160815203709-b44dc874b601 // indirect
	github.com/go-openapi/jsonpointer v0.0.0-20160704185906-46af16f9f7b1 // indirect
	github.com/go-openapi/jsonreference v0.0.0-20160704190145-13c6e3589ad9 // indirect
	github.com/go-openapi/loads v0.0.0-20160704185440-18441dfa706d // indirect
	github.com/go-openapi/spec v0.0.0-20160808142527-6aced65f8501 // indirect
	github.com/go-openapi/swag v0.0.0-20160704191624-1d0bd113de87 // indirect
	github.com/gogo/protobuf v0.0.0-20170330071051-c0656edd0d9e // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/google/gofuzz v0.0.0-20161122191042-44d81051d367 // indirect
	github.com/juju/ratelimit v0.0.0-20151125201925-77ed1c8a0121 // indirect
	github.com/mailru/easyjson v0.0.0-20160728113105-d5b7844b561a // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/prometheus/client_golang v0.8.1-0.20170511141251-42552c195dd3
	github.com/prometheus/client_model v0.0.0-20170216185247-6f3806018612 // indirect
	github.com/prometheus/common v0.0.0-20170427095455-13ba4ddd0caa // indirect
	github.com/prometheus/procfs v0.0.0-20170519190837-65c1f6f8f0fc // indirect
	github.com/spf13/pflag v0.0.0-20170130214245-9ff6c6923cff // indirect
	github.com/stretchr/testify v1.5.1 // indirect
	github.com/ugorji/go v0.0.0-20170107133203-ded73eae5db7 // indirect
	go.opencensus.io v0.22.0 // indirect
	golang.org/x/net v0.0.0-20190724013045-ca1201d0de80
	golang.org/x/sys v0.0.0-20190730183949-1393eb018365 // indirect
	google.golang.org/api v0.7.0
	google.golang.org/genproto v0.0.0-20190716160619-c506a9f90610 // indirect
	google.golang.org/grpc v1.22.1 // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	gopkg.in/inf.v0 v0.9.0 // indirect
	k8s.io/apimachinery v0.0.0-20170521172002-2de00c78cb6d
	k8s.io/client-go v0.0.0-20170521172049-450baa5d60f8
)

replace (
	cloud.google.com/go => cloud.google.com/go v0.1.1-0.20160913182117-3b1ae45394a2
	github.com/PuerkitoBio/purell => github.com/PuerkitoBio/purell v1.0.0
	github.com/PuerkitoBio/urlesc => github.com/PuerkitoBio/urlesc v0.0.0-20160726150825-5bd2802263f2
	github.com/beorn7/perks => github.com/beorn7/perks v0.0.0-20160804104726-4c0e84591b9a
	github.com/davecgh/go-spew => github.com/davecgh/go-spew v0.0.0-20151105211317-5215b55f46b2
	github.com/emicklei/go-restful => github.com/emicklei/go-restful v1.1.4-0.20170410110728-ff4f55a20633
	github.com/emicklei/go-restful-swagger12 => github.com/emicklei/go-restful-swagger12 v0.0.0-20170208215640-dcef7f557305
	github.com/ghodss/yaml => github.com/ghodss/yaml v0.0.0-20150909031657-73d445a93680
	github.com/go-openapi/analysis => github.com/go-openapi/analysis v0.0.0-20160815203709-b44dc874b601
	github.com/go-openapi/jsonpointer => github.com/go-openapi/jsonpointer v0.0.0-20160704185906-46af16f9f7b1
	github.com/go-openapi/jsonreference => github.com/go-openapi/jsonreference v0.0.0-20160704190145-13c6e3589ad9
	github.com/go-openapi/loads => github.com/go-openapi/loads v0.0.0-20160704185440-18441dfa706d
	github.com/go-openapi/spec => github.com/go-openapi/spec v0.0.0-20160808142527-6aced65f8501
	github.com/go-openapi/swag => github.com/go-openapi/swag v0.0.0-20160704191624-1d0bd113de87
	github.com/gogo/protobuf => github.com/gogo/protobuf v0.0.0-20170330071051-c0656edd0d9e
	github.com/golang/glog => github.com/golang/glog v0.0.0-20141105023935-44145f04b68c
	github.com/golang/protobuf => github.com/golang/protobuf v1.3.2
	github.com/google/gofuzz => github.com/google/gofuzz v0.0.0-20161122191042-44d81051d367
	github.com/hashicorp/golang-lru => github.com/hashicorp/golang-lru v0.0.0-20160207214719-a0d98a5f2880
	github.com/juju/ratelimit => github.com/juju/ratelimit v0.0.0-20151125201925-77ed1c8a0121
	github.com/mailru/easyjson => github.com/mailru/easyjson v0.0.0-20160728113105-d5b7844b561a
	github.com/matttproud/golang_protobuf_extensions => github.com/matttproud/golang_protobuf_extensions v1.0.1
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.8.1-0.20170511141251-42552c195dd3
	github.com/prometheus/client_model => github.com/prometheus/client_model v0.0.0-20170216185247-6f3806018612
	github.com/prometheus/common => github.com/prometheus/common v0.0.0-20170427095455-13ba4ddd0caa
	github.com/prometheus/procfs => github.com/prometheus/procfs v0.0.0-20170519190837-65c1f6f8f0fc
	github.com/spf13/pflag => github.com/spf13/pflag v0.0.0-20170130214245-9ff6c6923cff
	github.com/ugorji/go => github.com/ugorji/go v0.0.0-20170107133203-ded73eae5db7
	go.opencensus.io => go.opencensus.io v0.22.0
	golang.org/x/net => golang.org/x/net v0.0.0-20190724013045-ca1201d0de80
	golang.org/x/oauth2 => golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sys => golang.org/x/sys v0.0.0-20190730183949-1393eb018365
	golang.org/x/text => golang.org/x/text v0.3.2
	google.golang.org/api => google.golang.org/api v0.7.0
	google.golang.org/appengine => google.golang.org/appengine v1.2.0
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190716160619-c506a9f90610
	google.golang.org/grpc => google.golang.org/grpc v1.22.1
	gopkg.in/inf.v0 => gopkg.in/inf.v0 v0.9.0
	gopkg.in/yaml.v2 => gopkg.in/yaml.v2 v2.0.0-20150924142314-53feefa2559f
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20170521172002-2de00c78cb6d
	k8s.io/client-go => k8s.io/client-go v0.0.0-20170521172049-450baa5d60f8
)
