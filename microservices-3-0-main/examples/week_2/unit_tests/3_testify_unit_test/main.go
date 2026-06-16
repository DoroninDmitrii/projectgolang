package main

import (
	"log/slog"
	"os"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/3_testify_unit_test/credit_score"
)

func main() {
	client := credit_score.Client{
		Gender:        "male",
		Age:           30,
		Profession:    "engineer",
		Experience:    7,
		AverageSalary: 60000,
	}

	creditScore, err := credit_score.CalculateCreditScore(client)
	if err != nil {
		slog.Error("ошибка расчёта кредитного рейтинга", "error", err)
		os.Exit(1)
	}

	slog.Info("кредитный рейтинг рассчитан", "score", creditScore)
}
