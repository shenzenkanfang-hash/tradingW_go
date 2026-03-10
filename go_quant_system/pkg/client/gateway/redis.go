package gateway

import (
	"context"
	"encoding/json"
	"time"

	"go_quant_system/pkg/logger"

	"github.com/redis/go-redis/v9"
)

// WeightLimit Redis权重监控结构体（替代字符串解析）
type WeightLimit struct {
	Minute    int     `json:"minute"`     // 当前分钟
	LimitNum  float64 `json:"limit_num"`  // 已使用权重
	IsBlocked bool    `json:"is_blocked"` // 是否熔断
}

// RedisClient Redis客户端封装（权重监控专用）
type RedisClient struct {
	cli    *redis.Client
	cfg    RedisConfig // 仅Redis相关配置
	logger logger.Logger
}

// RedisConfig Redis子配置（简化传参）
type RedisConfig struct {
	Addr                 string
	Password             string
	DBName               int
	ApiLimitKey          string
	CacheWindow          time.Duration
	WeightBlockThreshold float64
	WeightWarnThreshold  float64
	Timeout              time.Duration
}

// NewRedisClient 创建Redis客户端
func NewRedisClient(cfg RedisConfig, logger logger.Logger) *RedisClient {
	return &RedisClient{
		cli: redis.NewClient(&redis.Options{
			Addr:     cfg.Addr,
			Password: cfg.Password,
			DB:       cfg.DBName,
		}),
		cfg:    cfg,
		logger: logger,
	}
}

// GetWeightLimit 获取权重限制信息
func (r *RedisClient) GetWeightLimit(ctx context.Context) (*WeightLimit, error) {
	// 加超时，避免阻塞
	ctx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()

	val, err := r.cli.Get(ctx, r.cfg.ApiLimitKey).Result()
	if err != nil {
		if err == redis.Nil {
			// 无数据返回默认值
			return &WeightLimit{Minute: -1, LimitNum: 0, IsBlocked: false}, nil
		}
		r.logger.Error("redis get weight limit failed", logger.Err(err))
		return nil, err
	}

	var wl WeightLimit
	if err := json.Unmarshal([]byte(val), &wl); err != nil {
		r.logger.Error("redis unmarshal weight limit failed",
			logger.Err(err), logger.String("data", val))
		// 解析失败，删除脏数据
		_ = r.cli.Del(ctx, r.cfg.ApiLimitKey).Err()
		return &WeightLimit{Minute: -1, LimitNum: 0, IsBlocked: false}, nil
	}

	return &wl, nil
}

// SetWeightLimit 设置权重限制信息
func (r *RedisClient) SetWeightLimit(ctx context.Context, wl *WeightLimit) error {
	ctx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()

	data, err := json.Marshal(wl)
	if err != nil {
		r.logger.Error("redis marshal weight limit failed", logger.Err(err))
		return err
	}

	// 设置1分钟过期（自动清理）
	err = r.cli.Set(ctx, r.cfg.ApiLimitKey, string(data), 60*time.Second).Err()
	if err != nil {
		r.logger.Error("redis set weight limit failed", logger.Err(err))
	}
	return err
}

// Close 关闭Redis连接（资源释放）
func (r *RedisClient) Close() error {
	return r.cli.Close()
}
