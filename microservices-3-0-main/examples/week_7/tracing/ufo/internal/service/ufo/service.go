package ufo

type service struct {
	ufoRepository  UFORepository
	analysisClient AnalysisClient
}

func NewService(ufoRepository UFORepository, analysisClient AnalysisClient) *service {
	return &service{
		ufoRepository:  ufoRepository,
		analysisClient: analysisClient,
	}
}
