module github.com/olezhek28-courses/microservices-course-examples/examples/week_1/workspace/grpc

go 1.26.0

require (
	github.com/google/uuid v1.6.0
	github.com/olezhek28-courses/microservices-course-examples/examples/week_1/workspace/shared v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.11.1
	google.golang.org/grpc v1.81.0
	google.golang.org/protobuf v1.36.11
)

// Локальная зависимость на shared модуль.
// Необходима для сборки, пока репозиторий не опубликован на GitHub.
// go.work дополняет эту директиву, но не заменяет её полностью.
replace github.com/olezhek28-courses/microservices-course-examples/examples/week_1/workspace/shared => ../shared

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260504160031-60b97b32f348 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
