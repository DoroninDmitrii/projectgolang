package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	json "github.com/goccy/go-json"
)

// UUID комплектующих из seed-данных (migrations/00002_seed_components.sql)
const (
	motherboardZ790 = "cccccccc-0000-0000-0000-000000000001" // ASUS ROG Strix Z790 (LGA1700, DDR5)
	motherboardB650 = "cccccccc-0000-0000-0000-000000000002" // MSI MAG B650 (AM5, DDR5)

	cpuIntelI7   = "cccccccc-0000-0000-0000-000000000011" // Intel Core i7-13700K (LGA1700, TDP 125W)
	cpuAMDRyzen9 = "cccccccc-0000-0000-0000-000000000012" // AMD Ryzen 9 7950X (AM5, TDP 170W)

	ramDDR5 = "cccccccc-0000-0000-0000-000000000021" // Kingston Fury Beast DDR5
	ramDDR4 = "cccccccc-0000-0000-0000-000000000022" // Corsair Vengeance DDR4

	gpuRTX4070 = "cccccccc-0000-0000-0000-000000000031" // NVIDIA RTX 4070 (TDP 200W)
	gpuRTX4090 = "cccccccc-0000-0000-0000-000000000032" // NVIDIA RTX 4090 (TDP 450W)
)

var baseURL = "http://localhost:8080"

func main() {
	if u := os.Getenv("BASE_URL"); u != "" {
		baseURL = u
	}

	printHeader("PC Builder DDD Demo")

	// 1. Совместимая Intel-сборка
	printStep(1, "Совместимая Intel-сборка", "Z790 (LGA1700) + i7-13700K (LGA1700) + DDR5 + RTX 4070")
	buildUUID := createBuild(motherboardZ790, cpuIntelI7, ramDDR5, gpuRTX4070)

	// 2. Совместимая AMD-сборка
	printStep(2, "Совместимая AMD-сборка", "B650 (AM5) + Ryzen 9 (AM5) + DDR5 + RTX 4070")
	createBuild(motherboardB650, cpuAMDRyzen9, ramDDR5, gpuRTX4070)

	// 3. Несовместимый сокет
	printStep(3, "Несовместимый сокет", "B650 (AM5) + i7-13700K (LGA1700)")
	createBuild(motherboardB650, cpuIntelI7, ramDDR5, gpuRTX4070)

	// 4. Несовместимая RAM
	printStep(4, "Несовместимая RAM", "Z790 (DDR5) + Corsair DDR4")
	createBuild(motherboardZ790, cpuIntelI7, ramDDR4, gpuRTX4070)

	// 5. Несовместимый TDP
	printStep(5, "Несовместимый TDP", "i7-13700K (125W) + RTX 4090 (450W) -> 450 > 125*2")
	createBuild(motherboardZ790, cpuIntelI7, ramDDR5, gpuRTX4090)

	// 6. Без материнской платы
	printStep(6, "Без материнской платы", "CPU + RAM + GPU без материнки")
	createBuild(cpuIntelI7, ramDDR5, gpuRTX4070)

	// 7. Отмена сборки
	if buildUUID != "" {
		printStep(7, "Отмена сборки", "отменяем Intel-сборку "+buildUUID)
		cancelBuild(buildUUID)

		// 8. Повторная отмена
		printStep(8, "Повторная отмена", "та же сборка -> уже отменена")
		cancelBuild(buildUUID)
	}

	printFooter()
}

// createBuild отправляет POST /api/v1/builds и возвращает build_uuid при успехе
func createBuild(componentUUIDs ...string) string {
	body, err := json.Marshal(map[string]any{
		"component_uuids": componentUUIDs,
	})
	if err != nil {
		out("  [ERR]  " + err.Error() + "\n\n")
		return ""
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/api/v1/builds", bytes.NewReader(body))
	if err != nil {
		out("  [ERR]  " + err.Error() + "\n\n")
		return ""
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req) //nolint:forbidigo // CLI-демо: не продакшен-код
	if err != nil {
		out("  [ERR]  " + err.Error() + "\n\n")
		return ""
	}
	defer resp.Body.Close()

	raw := printResponse(resp.StatusCode, resp.Body)

	if resp.StatusCode == http.StatusCreated {
		var r struct {
			BuildUUID string `json:"build_uuid"`
		}

		if unmarshalErr := json.Unmarshal(raw, &r); unmarshalErr != nil {
			return ""
		}

		return r.BuildUUID
	}

	return ""
}

// cancelBuild отправляет POST /api/v1/builds/{uuid}/cancel
func cancelBuild(buildUUID string) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/api/v1/builds/"+buildUUID+"/cancel", nil)
	if err != nil {
		out("  [ERR]  " + err.Error() + "\n\n")
		return
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req) //nolint:forbidigo // CLI-демо: не продакшен-код
	if err != nil {
		out("  [ERR]  " + err.Error() + "\n\n")
		return
	}
	defer resp.Body.Close()

	printResponse(resp.StatusCode, resp.Body)
}

// Форматированный вывод

func out(s string) {
	fmt.Fprint(os.Stdout, s)
}

func outf(format string, args ...any) {
	fmt.Fprintf(os.Stdout, format, args...)
}

func printHeader(title string) {
	out("\n")
	out("================================================================\n")
	outf("  %s\n", title)
	out("================================================================\n")
	out("\n")
}

func printStep(n int, title, description string) {
	out("----------------------------------------------------------------\n")
	outf("  [%d] %s\n", n, title)
	outf("      %s\n", description)
	out("----------------------------------------------------------------\n")
}

func printResponse(statusCode int, body io.Reader) []byte {
	icon := "OK"
	if statusCode >= 400 {
		icon = "FAIL"
	}

	raw, err := io.ReadAll(body)
	if err != nil {
		outf("  [%s] HTTP %d (failed to read body: %v)\n", icon, statusCode, err)
		return nil
	}

	var pretty bytes.Buffer
	if indentErr := json.Indent(&pretty, raw, "      ", "  "); indentErr != nil {
		pretty.Write(raw)
	}

	outf("  [%s] HTTP %d\n", icon, statusCode)
	outf("      %s\n", pretty.String())

	return raw
}

func printFooter() {
	out("================================================================\n")
	out("  Demo completed\n")
	out("================================================================\n")
	out("\n")
}
