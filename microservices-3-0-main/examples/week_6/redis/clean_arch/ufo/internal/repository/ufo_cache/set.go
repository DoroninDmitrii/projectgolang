package ufo_cache

import (
	"context"
	"time"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/model"
	repoConverter "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/repository/converter"
)

func (r *repository) Set(ctx context.Context, uuid string, sighting model.Sighting, ttl time.Duration) error {
	cacheKey := r.getCacheKey(uuid)
	err := r.client.HSet(ctx, cacheKey, new(repoConverter.SightingToRedisView(sighting))).Err()
	if err != nil {
		return err
	}

	return r.client.Expire(ctx, cacheKey, ttl).Err()
}
