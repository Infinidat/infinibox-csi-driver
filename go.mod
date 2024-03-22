module infinibox-csi-driver

go 1.21

require (
	github.com/container-storage-interface/spec v1.8.0
	github.com/containerd/containerd v1.7.11
	github.com/go-resty/resty/v2 v2.7.0
	github.com/golang/protobuf v1.5.4
	github.com/kubernetes-csi/csi-lib-utils v0.9.0
	github.com/kubernetes-csi/external-snapshotter/client/v6 v6.2.0
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.63.0
	github.com/prometheus-operator/prometheus-operator/pkg/client v0.63.0
	github.com/prometheus/client_golang v1.16.0
	github.com/rs/zerolog v1.29.1
	github.com/stretchr/testify v1.8.4
	google.golang.org/grpc v1.59.0
	google.golang.org/protobuf v1.33.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.28.4
	k8s.io/apimachinery v0.28.4
	k8s.io/client-go v0.28.4
	k8s.io/klog/v2 v2.100.1
	k8s.io/kubernetes v1.28.4
	k8s.io/mount-utils v0.28.4
	k8s.io/utils v0.0.0-20231121161247-cf03d44ff3cf
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/emicklei/go-restful/v3 v3.10.1 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.3.1 // indirect
	github.com/imdario/mergo v0.3.13 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/sys/mountinfo v0.6.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/opencontainers/selinux v1.11.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.10.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/oauth2 v0.11.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/term v0.13.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230822172742-b8732ec3820d // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiextensions-apiserver v0.26.1 // indirect
	k8s.io/apiserver v0.28.4 // indirect
	k8s.io/cloud-provider v0.0.0 // indirect
	k8s.io/component-base v0.28.4 // indirect
	k8s.io/component-helpers v0.28.4 // indirect
	k8s.io/kube-openapi v0.0.0-20230717233707-2695361300d9 // indirect
	sigs.k8s.io/controller-runtime v0.14.4 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace k8s.io/cloud-provider => k8s.io/cloud-provider v0.28.4

replace k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.26.8

replace k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.28.4
