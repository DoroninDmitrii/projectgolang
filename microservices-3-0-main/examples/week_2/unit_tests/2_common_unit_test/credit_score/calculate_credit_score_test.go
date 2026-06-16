package credit_score

import "testing"

func TestCalculateCreditScore(t *testing.T) {
	tests := []struct {
		name     string
		client   Client
		expected int
	}{
		{
			name:     "мужчина инженер 30 лет",
			client:   Client{"male", 30, "engineer", 7, 60000, 0},
			expected: 750,
		},
		{
			name:     "женщина учитель 22 года с ребёнком",
			client:   Client{"female", 22, "teacher", 3, 40000, 1},
			expected: 460,
		},
		{
			name:     "мужчина доктор 45 лет высокий доход",
			client:   Client{"male", 45, "doctor", 20, 120000, 0},
			expected: 850,
		},
		{
			name:     "женщина пенсионер 55 лет двое детей",
			client:   Client{"female", 55, "retired", 30, 20000, 2},
			expected: 410,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := CalculateCreditScore(tc.client)
			if result != tc.expected {
				t.Errorf("CalculateCreditScore(%#v) = %d; want %d", tc.client, result, tc.expected)
			}
		})
	}
}
