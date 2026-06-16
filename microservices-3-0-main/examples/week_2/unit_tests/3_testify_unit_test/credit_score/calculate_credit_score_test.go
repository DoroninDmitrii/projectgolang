package credit_score

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateCreditScore(t *testing.T) {
	t.Run("корректный мужчина инженер", func(t *testing.T) {
		client := Client{"male", 30, "engineer", 7, 60000, 0}
		expected := 750

		result, err := CalculateCreditScore(client)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("корректная женщина учитель", func(t *testing.T) {
		client := Client{"female", 22, "teacher", 3, 40000, 1}
		expected := 460

		result, err := CalculateCreditScore(client)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("корректный мужчина доктор", func(t *testing.T) {
		client := Client{"male", 45, "doctor", 20, 120000, 0}
		expected := 850

		result, err := CalculateCreditScore(client)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("корректная женщина пенсионер", func(t *testing.T) {
		client := Client{"female", 55, "retired", 30, 20000, 2}
		expected := 410

		result, err := CalculateCreditScore(client)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("некорректный возраст", func(t *testing.T) {
		client := Client{"male", -1, "engineer", 7, 60000, 0}

		_, err := CalculateCreditScore(client)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrInvalidAge)
	})

	t.Run("некорректная зарплата", func(t *testing.T) {
		client := Client{"male", 30, "engineer", 7, -5000, 0}

		_, err := CalculateCreditScore(client)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrInvalidSalary)
	})
}
