module github.com/GoogleCloudPlatform/k8s-stackdriver/kubelet-to-gcm

go 1.13

require (
	cloud.google.com/go v0.1.1-0.20160929204940-debdc0ca0113 // indirect
	github.com/beorn7/perks v0.0.0-20180321164747-3a771d992973 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/gogo/protobuf v0.0.0-20160926202419-89f1976ff373 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v0.0.0-20160926185624-87c000235d3d // indirect
	github.com/google/gofuzz v0.0.0-20160201174807-fd52762d25a4
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/prometheus/client_golang v0.9.1
	github.com/prometheus/client_model v0.0.0-20150212101744-fa8ad6fec335 // indirect
	github.com/prometheus/common v0.0.0-20160928123818-e35a2e33a50a
	github.com/prometheus/procfs v0.0.0-20181129180645-aa55a523dc0a // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/ugorji/go v1.1.7 // indirect
	golang.org/x/net v0.0.0-20160929200032-8058fc7b18f8
	golang.org/x/oauth2 v0.0.0-20160902055913-3c3a985cb79f
	google.golang.org/api v0.0.0-20160927045140-adba394bac58
	google.golang.org/appengine v1.0.0 // indirect
	gopkg.in/yaml.v2 v2.2.8 // indirect
	k8s.io/kubernetes v1.5.0-alpha.0.0.20160909200950-24c8d0655268
)

replace (
	cloud.google.com/go => cloud.google.com/go v0.1.1-0.20160929204940-debdc0ca0113
	github.com/beorn7/perks => github.com/beorn7/perks v0.0.0-20180321164747-3a771d992973
	github.com/gogo/protobuf => github.com/gogo/protobuf v0.0.0-20160926202419-89f1976ff373
	github.com/golang/glog => github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf => github.com/golang/protobuf v0.0.0-20160926185624-87c000235d3d
	github.com/google/gofuzz => github.com/google/gofuzz v0.0.0-20160201174807-fd52762d25a4
	github.com/matttproud/golang_protobuf_extensions => github.com/matttproud/golang_protobuf_extensions v1.0.1
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.9.1
	github.com/prometheus/client_model => github.com/prometheus/client_model v0.0.0-20150212101744-fa8ad6fec335
	github.com/prometheus/common => github.com/prometheus/common v0.0.0-20160928123818-e35a2e33a50a
	github.com/prometheus/procfs => github.com/prometheus/procfs v0.0.0-20181129180645-aa55a523dc0a
	golang.org/x/net => golang.org/x/net v0.0.0-20160929200032-8058fc7b18f8
	golang.org/x/oauth2 => golang.org/x/oauth2 v0.0.0-20160902055913-3c3a985cb79f
	google.golang.org/api => google.golang.org/api v0.0.0-20160927045140-adba394bac58
	google.golang.org/appengine => google.golang.org/appengine v1.0.0
	k8s.io/kubernetes => k8s.io/kubernetes v1.5.0-alpha.0.0.20160909200950-24c8d0655268
)
