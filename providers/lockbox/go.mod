module github.com/altessa-s/go-kryptos/providers/lockbox

go 1.27.0

toolchain go1.27.3

require (
	github.com/altessa-s/go-atlas-core v0.3.3
	github.com/altessa-s/go-atlas-net v0.2.3
	github.com/altessa-s/go-kryptos v0.1.3
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/stretchr/testify v1.12.1
	github.com/yandex-cloud/go-genproto v0.134.0
	google.golang.org/grpc v1.85.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260817212433-ac3dfec99bb1 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260817212433-ac3dfec99bb1 // indirect
)

replace github.com/altessa-s/go-kryptos => ../..
