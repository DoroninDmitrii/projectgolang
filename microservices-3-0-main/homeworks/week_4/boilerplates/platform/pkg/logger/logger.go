// Package logger конфигурирует стандартный slog для Cloud Native приложений
//
// ЗАЧЕМ НУЖЕН ЭТОТ ПАКЕТ:
//
// Стандартный slog из коробки пишет текстом в stderr. Для Cloud Native приложений под
// Kubernetes этого недостаточно: нужен JSON в stdout, чтобы Filebeat/Fluentd могли собирать
// и парсить логи контейнеров для отправки в ELK (Elasticsearch + Kibana)
//
// Этот пакет решает две задачи:
//  1. При загрузке пакета — устанавливает безопасный дефолт (JSON, stdout, INFO)
//     Это значит, что slog.Info() работает корректно с первой строчки кода, даже до парсинга конфига
//  2. Init() — применяет настройки из конфига (уровень логирования, формат вывода)
//     Вызывается после парсинга конфига. Все последующие вызовы slog используют новые настройки
//
// ПОРЯДОК ИНИЦИАЛИЗАЦИИ:
//
//  1. Go загружает пакет logger → slog.SetDefault() ставит JSON handler в stdout (level=INFO)
//  2. Go загружает пакет closer → closer использует slog.Info() → всё работает
//  3. Приложение стартует, парсит конфиг → вызывает logger.Init("debug")
//  4. slog.SetDefault() подменяет handler → все вызовы slog.Info() теперь используют новый уровень
//
// Благодаря этому нет проблемы "курица и яйцо": closer и другие пакеты спокойно
// используют slog до вызова Init() — логи просто пойдут с дефолтным уровнем INFO
//
// ИСПОЛЬЗОВАНИЕ В КОДЕ:
//
// Везде используется стандартный slog напрямую — никаких обёрток:
//
//	slog.Info("сервер запущен", "port", 50051)
//	slog.Error("ошибка подключения", "error", err)
//
// РАСШИРЕНИЕ (OTLP/ELK):
//
// В week_7 (observability) сюда добавляется teeHandler, который дублирует логи
// в OpenTelemetry Collector для отправки в ELK-стек. При этом весь код приложения
// продолжает использовать стандартный slog.Info() — меняется только handler в Init()
package logger

import (
	"log/slog"
	"os"
)

// При загрузке пакета устанавливаем безопасный дефолт: JSON в stdout с уровнем INFO
// Это гарантирует, что slog корректно работает до вызова Init(), и логи сразу идут
// в формате, пригодном для Kubernetes (JSON в stdout)
func init() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))
}

// Init применяет настройки логгера из конфига приложения
// Вызывается один раз после парсинга конфига. Подменяет глобальный slog handler,
// после чего все вызовы slog.Info(), slog.Error() и т.д. используют новые настройки
func Init(level string) {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(level),
	})))
}

// parseLevel преобразует строковое значение уровня логирования в slog.Level
func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
