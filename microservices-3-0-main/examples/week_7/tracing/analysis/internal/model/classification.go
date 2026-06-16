package model

// ClassificationResult — результат классификации объекта
type ClassificationResult struct {
	ObjectType     string
	Confidence     float32
	Explanation    string
	AnalysisResult string
}
