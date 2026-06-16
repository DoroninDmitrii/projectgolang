package model

// ClassificationResult — результат классификации наблюдения
type ClassificationResult struct {
	ObjectType  string
	Confidence  float32
	Explanation string
}
