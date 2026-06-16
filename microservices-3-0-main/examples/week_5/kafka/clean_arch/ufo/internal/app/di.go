package app

import (
	"context"
	"log/slog"
	"os"

	"github.com/IBM/sarama"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/platform/pkg/closer"
	wrappedKafkaConsumer "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/platform/pkg/kafka/consumer"
	wrappedKafkaProducer "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/platform/pkg/kafka/producer"
	kafkaMiddleware "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/platform/pkg/middleware/kafka"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/shared/pkg/proto/ufo/v1"
	ufov1API "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/api/ufo/v1"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/config"
	ufoConsumer "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/consumer/ufo"
	ufoProducer "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/producer/ufo"
	ufoRepository "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/repository/ufo"
	ufoService "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/service/ufo"
)

// ConsumerService определяет контракт для запуска Kafka-потребителей
type ConsumerService interface {
	RunConsumer(ctx context.Context) error
}

// diContainer — контейнер зависимостей (Composition Root) приложения
//
// Зачем это нужно:
// В простых приложениях зависимости создаются прямо в main.go: pool := pgxpool.New(...),
// repo := NewRepo(pool), svc := NewService(repo) и т.д. Это работает, пока зависимостей мало
// Когда сервис обрастает десятками компонентов, main.go превращается в «простыню» инициализации,
// а порядок создания начинает зависеть от неочевидных связей
//
// DI-контейнер решает эту проблему: каждый компонент «знает», от чего зависит, и создаёт
// свои зависимости по цепочке автоматически при первом обращении
//
// Как это работает:
// Каждый геттер следует паттерну «ленивая инициализация» (lazy initialization):
//  1. Проверяет, создан ли уже объект (nil-check)
//  2. Если нет — создаёт, запоминает в поле и возвращает
//  3. Если да — сразу возвращает ранее созданный экземпляр
//
// Это гарантирует, что каждый компонент создаётся ровно один раз, независимо от того,
// сколько раз к нему обращаются, и в правильном порядке
//
// Как добавить новую зависимость:
//  1. Добавьте поле с типом интерфейса в структуру
//  2. Напишите геттер с nil-check, который вызывает геттеры зависимостей
//  3. Используйте геттер там, где нужен компонент
//
// Почему интерфейсы (а не конкретные типы):
// Структуры слоёв (repository, service, proto) — unexported, чтобы их нельзя было создать
// в обход конструктора New(). Контейнер хранит интерфейсы, которые определены в потребителях
// (deps.go). Это также позволяет легко подменять реализации при необходимости
//
// Почему геттеры не возвращают ошибки:
// Если не удалось подключиться к базе или Kafka — приложение не может работать. Вместо того,
// чтобы протаскивать ошибку через 5 уровней вызовов, мы логируем и завершаем процесс
// сразу в месте проблемы. Это упрощает API контейнера и код всех вызывающих
type diContainer struct {
	// Инфраструктура
	pgPool        *pgxpool.Pool
	syncProducer  sarama.SyncProducer
	consumerGroup sarama.ConsumerGroup

	// Kafka-обёртки
	ufoRecordedProducer *wrappedKafkaProducer.Producer
	ufoRecordedConsumer *wrappedKafkaConsumer.Consumer

	// Репозитории
	ufoRepo ufoService.UFORepository

	// Сервисы
	ufoSvc         ufov1API.UFOService
	ufoProducerSvc ufoService.UFOProducerService
	ufoConsumerSvc ConsumerService

	// API-обработчики
	ufov1Handler ufov1.UFOServiceServer
}

// PGPool возвращает пул подключений к PostgreSQL
// При первом вызове создаёт пул, проверяет соединение и регистрирует closer
func (d *diContainer) PGPool(ctx context.Context) *pgxpool.Pool {
	if d.pgPool == nil {
		pool, err := pgxpool.New(ctx, config.AppConfig().PG.DSN())
		if err != nil {
			slog.Error("не удалось подключиться к PostgreSQL", "error", err)
			os.Exit(1)
		}

		if err = pool.Ping(ctx); err != nil {
			slog.Error("не удалось выполнить ping PostgreSQL", "error", err)
			os.Exit(1)
		}

		closer.Add("PostgreSQL pool", func(_ context.Context) error {
			pool.Close()
			return nil
		})

		d.pgPool = pool
	}

	return d.pgPool
}

// SyncProducer возвращает синхронный Kafka-продюсер
// При первом вызове создаёт продюсер и регистрирует closer
func (d *diContainer) SyncProducer() sarama.SyncProducer {
	if d.syncProducer == nil {
		p, err := sarama.NewSyncProducer(
			config.AppConfig().Kafka.Brokers,
			config.AppConfig().UfoRecordedProducer.SaramaConfig(),
		)
		if err != nil {
			slog.Error("не удалось создать sync producer", "error", err)
			os.Exit(1)
		}

		closer.Add("Kafka sync producer", func(_ context.Context) error {
			return p.Close()
		})

		d.syncProducer = p
	}

	return d.syncProducer
}

// ConsumerGroup возвращает Kafka consumer group
// При первом вызове создаёт группу и регистрирует closer
func (d *diContainer) ConsumerGroup() sarama.ConsumerGroup {
	if d.consumerGroup == nil {
		consumerGroup, err := sarama.NewConsumerGroup(
			config.AppConfig().Kafka.Brokers,
			config.AppConfig().UfoRecordedConsumer.GroupID(),
			config.AppConfig().UfoRecordedConsumer.SaramaConfig(),
		)
		if err != nil {
			slog.Error("не удалось создать consumer group", "error", err)
			os.Exit(1)
		}

		closer.Add("Kafka consumer group", func(_ context.Context) error {
			return consumerGroup.Close()
		})

		d.consumerGroup = consumerGroup
	}

	return d.consumerGroup
}

// UFORecordedProducer возвращает обёртку Kafka-продюсера для событий UFORecorded
func (d *diContainer) UFORecordedProducer() *wrappedKafkaProducer.Producer {
	if d.ufoRecordedProducer == nil {
		d.ufoRecordedProducer = wrappedKafkaProducer.NewProducer(
			d.SyncProducer(),
			config.AppConfig().UfoRecordedProducer.Topic(),
		)
	}

	return d.ufoRecordedProducer
}

// UFORecordedConsumer возвращает обёртку Kafka-потребителя для событий UFORecorded
func (d *diContainer) UFORecordedConsumer() *wrappedKafkaConsumer.Consumer {
	if d.ufoRecordedConsumer == nil {
		d.ufoRecordedConsumer = wrappedKafkaConsumer.NewConsumer(
			d.ConsumerGroup(),
			[]string{
				config.AppConfig().UfoRecordedConsumer.Topic(),
			},
			wrappedKafkaConsumer.WithMiddlewares(
				kafkaMiddleware.ConsumerLogging(),
			),
		)
	}

	return d.ufoRecordedConsumer
}

// UFORepository возвращает репозиторий наблюдений НЛО
func (d *diContainer) UFORepository(ctx context.Context) ufoService.UFORepository {
	if d.ufoRepo == nil {
		d.ufoRepo = ufoRepository.NewRepository(d.PGPool(ctx))
	}

	return d.ufoRepo
}

// UFOProducerService возвращает сервис отправки событий в Kafka
func (d *diContainer) UFOProducerService() ufoService.UFOProducerService {
	if d.ufoProducerSvc == nil {
		d.ufoProducerSvc = ufoProducer.NewService(d.UFORecordedProducer())
	}

	return d.ufoProducerSvc
}

// UFOConsumerService возвращает сервис потребления событий из Kafka
func (d *diContainer) UFOConsumerService() ConsumerService {
	if d.ufoConsumerSvc == nil {
		d.ufoConsumerSvc = ufoConsumer.NewService(d.UFORecordedConsumer())
	}

	return d.ufoConsumerSvc
}

// UFOService возвращает сервис бизнес-логики наблюдений НЛО
func (d *diContainer) UFOService(ctx context.Context) ufov1API.UFOService {
	if d.ufoSvc == nil {
		d.ufoSvc = ufoService.NewService(d.UFORepository(ctx), d.UFOProducerService())
	}

	return d.ufoSvc
}

// UfoV1API возвращает gRPC-обработчик сервиса наблюдений НЛО
func (d *diContainer) UfoV1API(ctx context.Context) ufov1.UFOServiceServer {
	if d.ufov1Handler == nil {
		d.ufov1Handler = ufov1API.NewAPI(d.UFOService(ctx))
	}

	return d.ufov1Handler
}
