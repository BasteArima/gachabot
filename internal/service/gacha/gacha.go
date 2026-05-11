package gacha

import (
	"gachabot/internal/repository"
	"time"

	"github.com/redis/go-redis/v9"
)

type GachaService struct {
	repo          *repository.PostgresRepo
	rdb           *redis.Client
	loc           *time.Location
	adminID       int64
	cooldownHours time.Duration
}

func NewGachaService(repo *repository.PostgresRepo, rdb *redis.Client, adminID int64, cooldown time.Duration) *GachaService {
	loc := time.FixedZone("MSK", 3*60*60)
	return &GachaService{
		repo:          repo,
		rdb:           rdb,
		loc:           loc,
		adminID:       adminID,
		cooldownHours: cooldown,
	}
}
