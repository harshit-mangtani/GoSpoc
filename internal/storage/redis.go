package storage

import (
	"context"
    "github.com/redis/go-redis/v9"
	"github.com/harshit-mangtani/GoSpoc/internal/config"
)

func NewRedis(ctx context.Context, cfg config.Config) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
        Addr:     cfg.RedisAddr,
        Password: cfg.RedisPassword, // no password set
        DB:       cfg.RedisDB,  // use default DB
    })
	defer rdb.Close()
	err:= rdb.Set(ctx,"redis","running",0).Err()
	if err!=nil{
		return nil, err
	}
	return rdb,nil
}
