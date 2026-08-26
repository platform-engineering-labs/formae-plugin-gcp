module github.com/platform-engineering-labs/formae-plugin-gcp

go 1.26.4

require (
	cloud.google.com/go/auth v0.23.2
	cloud.google.com/go/bigquery v1.81.0
	github.com/google/uuid v1.6.0
	github.com/googleapis/gax-go/v2 v2.23.0
	github.com/platform-engineering-labs/formae/pkg/model v0.1.26
	// The OidcAware interface this plugin implements landed after
	// pkg/plugin/v0.4.1. Re-pin to the next official tag when one is cut, as
	// the aws plugin does from the same commit.
	github.com/platform-engineering-labs/formae/pkg/plugin v0.4.2-0.20260821224650-dc5149d5a102
	github.com/platform-engineering-labs/formae/pkg/plugin-conformance-tests v0.2.6
	// Provides oidcx/gcp, the token exchange. Re-pin when oox cuts a tag.
	github.com/platform-engineering-labs/oox v0.1.1-0.20260825170105-3bd97cb18d15
	github.com/stretchr/testify v1.11.1
	golang.org/x/oauth2 v0.36.0
	google.golang.org/api v0.293.0
	google.golang.org/grpc v1.83.1
)

// gcpname is its own dependency-free module: the canonical parser shared with
// the provisioner, the credential broker and the CLI. Re-pin when oox tags.
require github.com/platform-engineering-labs/oox/gcpname v0.0.0-20260825170105-3bd97cb18d15

require (
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	cloud.google.com/go/iam v1.11.0 // indirect
	ergo.services/actor/statemachine v0.0.0-20251202053101-c0aa08b403e5 // indirect
	ergo.services/ergo v1.999.320 // indirect
	github.com/apache/arrow/go/v15 v15.0.2 // indirect
	github.com/apple/pkl-go v0.13.2 // indirect
	github.com/asdine/storm v2.1.2+incompatible // indirect
	github.com/aws/aws-sdk-go-v2 v1.43.5 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.7 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.32.35 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.19.34 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.35 // indirect
	github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager v0.1.10 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.36 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.35 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.20 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3 v1.97.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.5.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.33.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.38.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.45.4 // indirect
	github.com/aws/smithy-go v1.27.7 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/ebitengine/purego v0.9.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/gofrs/flock v0.13.0 // indirect
	github.com/google/flatbuffers v23.5.26+incompatible // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.20 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.7 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/klauspost/cpuid/v2 v2.2.5 // indirect
	github.com/lufia/plan9stats v0.0.0-20251013123823-9fd1530e3ec3 // indirect
	github.com/lunixbochs/struc v0.0.0-20241101090106-8d528fa2c543 // indirect
	github.com/mashiike/s3-setlock v0.2.0 // indirect
	github.com/masterminds/semver v1.5.0 // indirect
	github.com/miekg/dns v1.1.72 // indirect
	github.com/naegelejd/go-acl v0.0.0-20260323030528-42e4d61407df // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/platform-engineering-labs/formae/pkg/api/model v0.1.1 // indirect
	github.com/platform-engineering-labs/formae/pkg/credential v0.0.0-20260821213704-ba68bacf6dd6 // indirect
	github.com/platform-engineering-labs/orbital v0.1.36 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/segmentio/ksuid v1.0.4 // indirect
	github.com/shirou/gopsutil/v4 v4.26.1 // indirect
	github.com/theory/jsonpath v0.10.2 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	github.com/zeebo/xxh3 v1.0.2 // indirect
	github.com/zps-io/sat v0.0.0-20190412034122-acaa8fa26246 // indirect
	go.etcd.io/bbolt v1.4.3 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.67.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/host v0.65.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.67.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/runtime v0.65.0 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.39.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.39.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/sdk v1.44.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	go.opentelemetry.io/proto/otlp v1.9.0 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56 // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/telemetry v0.0.0-20260625142307-59b4966ccb57 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.47.0 // indirect
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
	google.golang.org/genproto v0.0.0-20260319201613-d00831a3d3e7 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260630182238-925bb5da69e7 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260807164820-c8921c73eeea // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	resty.dev/v3 v3.0.0-beta.6.0.20260127085140-f531c9de7027 // indirect
)

replace ergo.services/ergo => github.com/JeroenSoeters/ergo v1.999.320-pel.1

replace ergo.services/actor/statemachine => github.com/JeroenSoeters/actor/statemachine v0.0.0-20260414171442-1ef660e4a3bc
