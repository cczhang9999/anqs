package db

import (
	"context"
	"fmt"

	"xops-aegis-guard/config"

	redislib "github.com/redis/go-redis/v9"
)

const (
	RedisClusterMode   = "cluster"
	RedisSingletonMode = "singleton"
)

var RedisClient *redislib.Client
var RedisClusterClient *redislib.ClusterClient

func InitRedis(conf config.Redis) error {
	if len(conf.Addrs) == 0 {
		return nil
	}
	if conf.Pool == nil {
		conf.Pool = &config.RedisPool{PoolSize: 10}
	}
	if conf.Mode == "" {
		conf.Mode = RedisSingletonMode
	}

	switch conf.Mode {
	case RedisClusterMode:
		RedisClusterClient = redislib.NewClusterClient(&redislib.ClusterOptions{
			Addrs:        conf.Addrs,
			Password:     conf.Password,
			PoolSize:     conf.Pool.PoolSize,
			MinIdleConns: conf.Pool.MinIdleConns,
		})
		if err := RedisClusterClient.Ping(context.Background()).Err(); err != nil {
			return fmt.Errorf("connect redis cluster: %w", err)
		}
	case RedisSingletonMode:
		RedisClient = redislib.NewClient(&redislib.Options{
			Addr:         conf.Addrs[0],
			Password:     conf.Password,
			DB:           conf.DB,
			PoolSize:     conf.Pool.PoolSize,
			MinIdleConns: conf.Pool.MinIdleConns,
		})
		if err := RedisClient.Ping(context.Background()).Err(); err != nil {
			return fmt.Errorf("connect redis: %w", err)
		}
	default:
		return fmt.Errorf("unsupported redis mode %q", conf.Mode)
	}
	return nil
}

func RedisUniversalClient() redislib.UniversalClient {
	if RedisClient != nil {
		return RedisClient
	}
	if RedisClusterClient != nil {
		return RedisClusterClient
	}
	return nil
}

func RedisClose() {
	if RedisClusterClient != nil {
		_ = RedisClusterClient.Close()
		RedisClusterClient = nil
	}
	if RedisClient != nil {
		_ = RedisClient.Close()
		RedisClient = nil
	}
}
