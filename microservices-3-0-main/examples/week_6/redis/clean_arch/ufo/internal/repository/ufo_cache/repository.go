package ufo_cache

import "fmt"

const (
	cacheKeyPrefix = "ufo:sighting:"
)

type repository struct {
	client redisClient
}

func NewRepository(client redisClient) *repository {
	return &repository{
		client: client,
	}
}

func (r *repository) getCacheKey(uuid string) string {
	return fmt.Sprintf("%s%s", cacheKeyPrefix, uuid)
}
