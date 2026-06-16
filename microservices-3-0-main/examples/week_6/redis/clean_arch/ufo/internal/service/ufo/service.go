package ufo

import "time"

// service предоставляет бизнес-логику для работы с наблюдениями НЛО
type service struct {
	ufoRepo   UFORepository
	cacheRepo UFOCacheRepository
	cacheTTL  time.Duration
}

// New создаёт сервис наблюдений НЛО
func New(ufoRepo UFORepository, cacheRepo UFOCacheRepository, cacheTTL time.Duration) *service {
	return &service{
		ufoRepo:   ufoRepo,
		cacheRepo: cacheRepo,
		cacheTTL:  cacheTTL,
	}
}
