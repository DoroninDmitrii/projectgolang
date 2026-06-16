// Package static содержит встроенные статические файлы для HTTP сервера
package static

import "embed"

// FS содержит встроенные файлы:
// - swagger-ui.html - интерфейс Swagger UI
// - generated/ufo.swagger.json - OpenAPI спецификация (генерируется buf)
//
//go:embed swagger-ui.html generated/ufo.swagger.json
var FS embed.FS
