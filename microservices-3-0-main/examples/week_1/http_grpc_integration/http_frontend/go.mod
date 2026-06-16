module github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_grpc_integration/http_frontend

go 1.26.0

require (
	github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_grpc_integration/grpc_backend v0.0.0
	github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_grpc_integration/shared v0.0.0
	github.com/stretchr/testify v1.11.1
	google.golang.org/grpc v1.81.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260504160031-60b97b32f348 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_grpc_integration/shared => ../shared

replace github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_grpc_integration/grpc_backend => ../grpc_backend
