module open-match.dev/open-match/tutorials/matchmaker101/evaluator

go 1.23.0

toolchain go1.23.9

require (
	github.com/sirupsen/logrus v1.9.3
	google.golang.org/grpc v1.70.0
	open-match.dev/open-match v0.0.0-dev
)

require (
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.26.3 // indirect
	golang.org/x/net v0.35.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.22.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250303144028-a0af3efb3deb // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250303144028-a0af3efb3deb // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)

replace open-match.dev/open-match v0.0.0-dev => ../../../../
