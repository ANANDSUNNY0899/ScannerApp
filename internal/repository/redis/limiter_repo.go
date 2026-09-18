package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type LimiterRepository interface {
	GetOCRUsage(ctx context.Context, userID uuid.UUID) (int64, error)
	IncrementOCRUsage(ctx context.Context, userID uuid.UUID) (int64, error)
	GetRemainingOCR(ctx context.Context, userID uuid.UUID, maxLimit int64) (int64, error)
}

type limiterRepo struct {
	client *RedisClient
}

func NewLimiterRepository(client *RedisClient) LimiterRepository {
	return &limiterRepo{client: client}
}

func (r *limiterRepo) ocrKey(userID uuid.UUID) string {
	return fmt.Sprintf("ocr_usage:%s", userID.String())
}

func (r *limiterRepo) GetOCRUsage(ctx context.Context, userID uuid.UUID) (int64, error) {
	key := r.ocrKey(userID)
	val, err := r.client.Get(ctx, key).Int64()
	if err != nil {
		if err.Error() == "redis: nil" {
			return 0, nil
		}
		return 0, err
	}
	return val, nil
}

func (r *limiterRepo) IncrementOCRUsage(ctx context.Context, userID uuid.UUID) (int64, error) {
	key := r.ocrKey(userID)
	count, err := r.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	// If it's the first usage, set 30-day or perpetual expiration
	if count == 1 {
		_ = r.client.Expire(ctx, key, 30*24*time.Hour).Err()
	}

	return count, nil
}

func (r *limiterRepo) GetRemainingOCR(ctx context.Context, userID uuid.UUID, maxLimit int64) (int64, error) {
	usage, err := r.GetOCRUsage(ctx, userID)
	if err != nil {
		return 0, err
	}
	remaining := maxLimit - usage
	if remaining < 0 {
		remaining = 0
	}
	return remaining, nil
}
