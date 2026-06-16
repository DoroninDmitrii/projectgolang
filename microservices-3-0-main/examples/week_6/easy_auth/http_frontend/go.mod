module github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/http_frontend

go 1.26.0

require (
	github.com/go-chi/chi/v5 v5.2.5
	github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/grpc_backend v0.0.0
	github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/shared v0.0.0
	github.com/stretchr/testify v1.11.1
	google.golang.org/grpc v1.81.0
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260504160031-60b97b32f348 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/shared => ../shared

replace github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/grpc_backend => ../grpc_backend
