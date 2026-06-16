package tests

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
)

var brokerAddr string

// TestMain поднимает Kafka через testcontainers и запускает тесты
func TestMain(m *testing.M) {
	_ = godotenv.Load("../.env")

	ctx := context.Background()

	kafkaContainer, err := kafka.Run(
		ctx, os.Getenv("KAFKA_IMAGE"),
		kafka.WithClusterID("test-cluster"),
	)
	if err != nil {
		slog.Error("не удалось запустить Kafka контейнер", "error", err)
		os.Exit(1)
	}

	brokers, err := kafkaContainer.Brokers(ctx)
	if err != nil {
		slog.Error("не удалось получить адрес брокера", "error", err)
		os.Exit(1)
	}

	brokerAddr = brokers[0]

	code := m.Run()

	if termErr := kafkaContainer.Terminate(ctx); termErr != nil {
		slog.Error("не удалось остановить контейнер", "error", termErr)
	}

	os.Exit(code)
}

// runProgram запускает Go программу из каталога проекта и возвращает вывод
// Переопределяем только KAFKA_BROKER_ADDR, потому что testcontainers назначает случайный порт
// Остальные настройки (topic и т.д.) программы берут из своих дефолтов
func runProgram(t *testing.T, program string) string {
	t.Helper()

	cmd := exec.Command("go", "run", fmt.Sprintf("./cmd/%s/main.go", program))
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "KAFKA_BROKER_ADDR="+brokerAddr)

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "ошибка запуска %s:\n%s", program, string(output))

	return string(output)
}

func TestProducer_SendsMessages(t *testing.T) {
	output := runProgram(t, "producer")

	assert.Contains(t, output, "все сообщения отправлены")
}

func TestAsyncProducer_SendsMessages(t *testing.T) {
	output := runProgram(t, "async_producer")

	assert.Contains(t, output, "все подтверждения получены")
}

func TestProducerConsumer_EndToEnd(t *testing.T) {
	// Запускаем consumer в фоне
	consumerCmd := exec.Command("go", "run", "./cmd/consumer/main.go")
	consumerCmd.Dir = ".."
	consumerCmd.Env = append(os.Environ(), "KAFKA_BROKER_ADDR="+brokerAddr)
	stdout, err := consumerCmd.StdoutPipe()
	require.NoError(t, err)
	consumerCmd.Stderr = consumerCmd.Stdout

	require.NoError(t, consumerCmd.Start())
	defer func() {
		_ = consumerCmd.Process.Kill()
		_ = consumerCmd.Wait()
	}()

	// Ждём, пока consumer будет готов
	scanner := bufio.NewScanner(stdout)
	ready := make(chan struct{})
	lines := make(chan string, 100)

	go func() {
		readySignaled := false
		for scanner.Scan() {
			line := scanner.Text()
			if !readySignaled && strings.Contains(line, "✅ потребитель запущен и работает") {
				close(ready)
				readySignaled = true
			}
			lines <- line
		}
	}()

	select {
	case <-ready:
	case <-time.After(60 * time.Second):
		t.Fatal("consumer не стал готов за 60 секунд")
	}

	// Отправляем сообщения
	producerCmd := exec.Command("go", "run", "./cmd/producer/main.go")
	producerCmd.Dir = ".."
	producerCmd.Env = append(os.Environ(), "KAFKA_BROKER_ADDR="+brokerAddr)

	producerOutput, err := producerCmd.CombinedOutput()
	require.NoError(t, err, "producer завершился с ошибкой: %s", string(producerOutput))
	assert.Contains(t, string(producerOutput), "все сообщения отправлены")

	// Ждём, пока consumer получит сообщение
	timeout := time.After(30 * time.Second)
	for {
		select {
		case line := <-lines:
			if strings.Contains(line, "получено сообщение") {
				return // Успех
			}
		case <-timeout:
			t.Fatal("consumer не получил сообщение за 30 секунд")
		}
	}
}
