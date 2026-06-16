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

	"github.com/IBM/sarama"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
)

var brokerAddr string

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

	if err = createTopic(brokers, "batch-topic", 3); err != nil {
		slog.Error("не удалось создать топик", "error", err)
		os.Exit(1)
	}

	code := m.Run()

	if termErr := kafkaContainer.Terminate(ctx); termErr != nil {
		slog.Error("не удалось остановить контейнер", "error", termErr)
	}

	os.Exit(code)
}

func createTopic(brokers []string, topic string, partitions int32) error {
	config := sarama.NewConfig()
	config.Version = sarama.V3_6_0_0

	admin, err := sarama.NewClusterAdmin(brokers, config)
	if err != nil {
		return fmt.Errorf("не удалось создать admin client: %w", err)
	}
	defer admin.Close()

	err = admin.CreateTopic(topic, &sarama.TopicDetail{
		NumPartitions:     partitions,
		ReplicationFactor: 1,
	}, false)
	if err != nil {
		return fmt.Errorf("не удалось создать топик %s: %w", topic, err)
	}

	return nil
}

func runProgram(t *testing.T, program string, extraEnv ...string) string {
	t.Helper()

	cmd := exec.Command("go", "run", fmt.Sprintf("./cmd/%s/", program))
	cmd.Dir = ".."
	cmd.Env = append(
		os.Environ(),
		append([]string{
			"KAFKA_BROKER_ADDR=" + brokerAddr,
			"MESSAGE_COUNT=20",
		}, extraEnv...)...,
	)

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "ошибка запуска %s:\n%s", program, string(output))

	return string(output)
}

func TestProducer_SendsMessages(t *testing.T) {
	output := runProgram(t, "producer")
	assert.Contains(t, output, "все сообщения отправлены")
}

func TestConsumer_ReceivesMessages(t *testing.T) {
	groupID := fmt.Sprintf("test-one-by-one-%d", time.Now().UnixNano())
	testConsumerReceives(t, "consumer", groupID, "обработано сообщение")
}

func TestBatchConsumer_ReceivesMessages(t *testing.T) {
	groupID := fmt.Sprintf("test-batch-%d", time.Now().UnixNano())
	testConsumerReceives(t, "batch_consumer", groupID, "обработан батч")
}

// testConsumerReceives — общий хелпер для тестов consumer'ов
// Запускает consumer в фоне, отправляет сообщения, проверяет получение
func testConsumerReceives(t *testing.T, program, groupID, expectedLog string) {
	t.Helper()

	consumerCmd := exec.Command("go", "run", fmt.Sprintf("./cmd/%s/", program))
	consumerCmd.Dir = ".."
	consumerCmd.Env = append(
		os.Environ(),
		"KAFKA_BROKER_ADDR="+brokerAddr,
		"KAFKA_GROUP_ID="+groupID,
		"BATCH_SIZE=5",
		"FLUSH_INTERVAL_SEC=1",
	)
	stdout, err := consumerCmd.StdoutPipe()
	require.NoError(t, err)
	consumerCmd.Stderr = consumerCmd.Stdout

	require.NoError(t, consumerCmd.Start())
	defer func() {
		_ = consumerCmd.Process.Kill()
		_ = consumerCmd.Wait()
	}()

	scanner := bufio.NewScanner(stdout)
	ready := make(chan struct{})
	lines := make(chan string, 200)

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
	output := runProgram(t, "producer")
	assert.Contains(t, output, "все сообщения отправлены")

	// Ждём, пока consumer обработает
	timeout := time.After(30 * time.Second)
	for {
		select {
		case line := <-lines:
			if strings.Contains(line, expectedLog) {
				return // Успех
			}
		case <-timeout:
			t.Fatalf("consumer не вывел '%s' за 30 секунд", expectedLog)
		}
	}
}
