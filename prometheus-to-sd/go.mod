module github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd

go 1.13

require (
	cloud.google.com/go v0.1.1-0.20160929204940-debdc0ca0113
	github.com/beorn7/perks v0.0.0-20160804104726-4c0e84591b9a // indirect
	github.com/davecgh/go-spew v1.1.0 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/gogo/protobuf v1.1.1 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v0.0.0-20160926185624-87c000235d3d // indirect
	github.com/google/btree v0.0.0-20180124185431-e89373fe6b4a // indirect
	github.com/google/gofuzz v0.0.0-20170612174753-24818f796faf // indirect
	github.com/googleapis/gnostic v0.2.0 // indirect
	github.com/gregjones/httpcache v0.0.0-20180305231024-9cad4c3443a7 // indirect
	github.com/json-iterator/go v0.0.0-20180701071628-ab8a2e0c74be // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v0.0.0-20180701023420-4b7aa43c6742 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v0.8.1-0.20170724081313-94ff84a9a6eb
	github.com/prometheus/client_model v0.0.0-20150212101744-fa8ad6fec335
	github.com/prometheus/common v0.0.0-20160928123818-e35a2e33a50a
	github.com/prometheus/procfs v0.0.0-20170703101242-e645f4e5aaa8 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/testify v1.2.2
	golang.org/x/crypto v0.0.0-20180723164146-c126467f60eb // indirect
	golang.org/x/net v0.0.0-20160929200032-8058fc7b18f8 // indirect
	golang.org/x/oauth2 v0.0.0-20160902055913-3c3a985cb79f
	golang.org/x/sys v0.0.0-20180724212812-e072cadbbdc8 // indirect
	golang.org/x/time v0.0.0-20180412165947-fbb02b2291d2 // indirect
	google.golang.org/api v0.0.0-20160927045140-adba394bac58
	google.golang.org/appengine v1.0.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.2.1 // indirect
	k8s.io/api v0.0.0-20180711052118-183f3326a935
	k8s.io/apimachinery v0.0.0-20180724074904-cbafd24d5796
	k8s.io/client-go v0.0.0-20180724102132-3db81bdd1286
)

replace (
	cloud.google.com/go => cloud.google.com/go v0.1.1-0.20160929204940-debdc0ca0113
	github.com/beorn7/perks => github.com/beorn7/perks v0.0.0-20160804104726-4c0e84591b9a
	github.com/davecgh/go-spew => github.com/davecgh/go-spew v1.1.0
	github.com/ghodss/yaml => github.com/ghodss/yaml v1.0.0
	github.com/gogo/protobuf => github.com/gogo/protobuf v1.1.1
	github.com/golang/glog => github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf => github.com/golang/protobuf v0.0.0-20160926185624-87c000235d3d
	github.com/google/btree => github.com/google/btree v0.0.0-20180124185431-e89373fe6b4a
	github.com/google/gofuzz => github.com/google/gofuzz v0.0.0-20170612174753-24818f796faf
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.2.0
	github.com/gregjones/httpcache => github.com/gregjones/httpcache v0.0.0-20180305231024-9cad4c3443a7
	github.com/json-iterator/go => github.com/json-iterator/go v0.0.0-20180701071628-ab8a2e0c74be
	github.com/matttproud/golang_protobuf_extensions => github.com/matttproud/golang_protobuf_extensions v1.0.1
	github.com/modern-go/concurrent => github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd
	github.com/modern-go/reflect2 => github.com/modern-go/reflect2 v0.0.0-20180701023420-4b7aa43c6742
	github.com/petar/GoLLRB => github.com/petar/GoLLRB v0.0.0-20130427215148-53be0d36a84c
	github.com/peterbourgon/diskv => github.com/peterbourgon/diskv v2.0.1+incompatible
	github.com/pmezard/go-difflib => github.com/pmezard/go-difflib v1.0.0
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.8.1-0.20170724081313-94ff84a9a6eb
	github.com/prometheus/client_model => github.com/prometheus/client_model v0.0.0-20150212101744-fa8ad6fec335
	github.com/prometheus/common => github.com/prometheus/common v0.0.0-20160928123818-e35a2e33a50a
	github.com/prometheus/procfs => github.com/prometheus/procfs v0.0.0-20170703101242-e645f4e5aaa8
	github.com/stretchr/testify => github.com/stretchr/testify v1.2.2
	golang.org/x/crypto => golang.org/x/crypto v0.0.0-20180723164146-c126467f60eb
	golang.org/x/net => golang.org/x/net v0.0.0-20160929200032-8058fc7b18f8
	golang.org/x/oauth2 => golang.org/x/oauth2 v0.0.0-20160902055913-3c3a985cb79f
	golang.org/x/sys => golang.org/x/sys v0.0.0-20180724212812-e072cadbbdc8
	golang.org/x/time => golang.org/x/time v0.0.0-20180412165947-fbb02b2291d2
	google.golang.org/api => google.golang.org/api v0.0.0-20160927045140-adba394bac58
	google.golang.org/appengine => google.golang.org/appengine v1.0.0
	gopkg.in/inf.v0 => gopkg.in/inf.v0 v0.9.1
	gopkg.in/yaml.v2 => gopkg.in/yaml.v2 v2.2.1
	k8s.io/api => k8s.io/api v0.0.0-20180711052118-183f3326a935
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20180724074904-cbafd24d5796
	k8s.io/client-go => k8s.io/client-go v0.0.0-20180724102132-3db81bdd1286
)
