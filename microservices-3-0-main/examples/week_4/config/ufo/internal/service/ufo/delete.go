package ufo

import (
	"context"
)

func (s *service) Delete(ctx context.Context, uuid string) error {
	return s.ufoRepository.Delete(ctx, uuid)
}
