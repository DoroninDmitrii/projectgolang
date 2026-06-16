package credit_score

// Client структура для хранения данных клиента
type Client struct {
	Gender        string
	Age           int
	Profession    string
	Experience    int
	AverageSalary float64
	ChildCount    int
}

// CalculateCreditScore функция для расчета кредитного рейтинга
//
//nolint:cyclop // учебный пример с намеренно сложной логикой для демонстрации тестирования
func CalculateCreditScore(client Client) int {
	score := 0

	// Пример логики расчета кредитного рейтинга
	switch client.Gender {
	case "male":
		score += 50
	case "female":
		score += 60
	}

	switch {
	case client.Age >= 18 && client.Age <= 25:
		score += 100
	case client.Age > 25 && client.Age <= 35:
		score += 150
	case client.Age > 35 && client.Age <= 50:
		score += 200
	case client.Age > 50:
		score += 100
	}

	switch client.Profession {
	case "engineer":
		score += 200
	case "teacher":
		score += 150
	default:
		score += 100
	}

	switch {
	case client.Experience >= 1 && client.Experience <= 5:
		score += 100
	case client.Experience > 5 && client.Experience <= 10:
		score += 150
	case client.Experience > 10:
		score += 200
	}

	switch {
	case client.AverageSalary >= 30000 && client.AverageSalary <= 50000:
		score += 100
	case client.AverageSalary > 50000 && client.AverageSalary <= 100000:
		score += 200
	case client.AverageSalary > 100000:
		score += 300
	}

	if client.ChildCount > 0 {
		score -= 50
	}

	// Ограничение рейтинга от 0 до 1000.
	return max(0, min(1000, score))
}
