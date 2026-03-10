package gateway

import "time"

// GatewayConfig 币安网关配置（对应configs/binance_gateway.yaml）
type GatewayConfig struct {
	Redis struct {
		Addr                 string        `yaml:"addr"`                   // Redis地址（如127.0.0.1:6379）
		Password             string        `yaml:"password"`               // Redis密码
		DBName               int           `yaml:"db_name"`                // Redis数据库
		ApiLimitKey          string        `yaml:"api_limit_key"`          // 权重监控Key
		CacheWindow          time.Duration `yaml:"cache_window"`           // 本地缓存窗口期
		WeightBlockThreshold float64       `yaml:"weight_block_threshold"` // 拦截阈值
		WeightWarnThreshold  float64       `yaml:"weight_warn_threshold"`  // 熔断阈值
		Timeout              time.Duration `yaml:"timeout"`                // Redis操作超时
	} `yaml:"redis"`
	HTTP struct {
		MaxIdleConns        int           `yaml:"max_idle_conns"`          // HTTP连接池最大空闲连接
		MaxIdleConnsPerHost int           `yaml:"max_idle_conns_per_host"` // 单Host最大空闲连接
		IdleConnTimeout     time.Duration `yaml:"idle_conn_timeout"`       // 空闲连接超时
		TLSHandshakeTimeout time.Duration `yaml:"tls_handshake_timeout"`   // TLS握手超时
		RequestTimeout      time.Duration `yaml:"request_timeout"`         // 请求超时
	} `yaml:"http"`
	Weight struct {
		LimitRatio float64 `yaml:"limit_ratio"` // 本地权重限流比例（如0.9=90%）
		MaxWeight  float64 `yaml:"max_weight"`  // 币安分钟级权重上限（默认1200）
	} `yaml:"weight"`
	Time struct {
		RecvWindow int64 `yaml:"recv_window"` // 签名有效期（毫秒，默认5000）
	} `yaml:"time"`
}

// DefaultConfig 默认配置（兜底）
func DefaultConfig() GatewayConfig {
	var cfg GatewayConfig
	// Redis默认配置
	cfg.Redis.Addr = "127.0.0.1:6379"
	cfg.Redis.Password = ""
	cfg.Redis.DBName = 0
	cfg.Redis.ApiLimitKey = "x-mbx-used-weight-1m_trade"
	cfg.Redis.CacheWindow = 100 * time.Millisecond
	cfg.Redis.WeightBlockThreshold = 5500
	cfg.Redis.WeightWarnThreshold = 5000
	cfg.Redis.Timeout = 100 * time.Millisecond

	// HTTP默认配置（适配1G服务器）
	cfg.HTTP.MaxIdleConns = 100
	cfg.HTTP.MaxIdleConnsPerHost = 20
	cfg.HTTP.IdleConnTimeout = 90 * time.Second
	cfg.HTTP.TLSHandshakeTimeout = 5 * time.Second
	cfg.HTTP.RequestTimeout = 10 * time.Second

	// 权重默认配置
	cfg.Weight.LimitRatio = 0.9
	cfg.Weight.MaxWeight = 1200.0

	// 时间默认配置
	cfg.Time.RecvWindow = 5000

	return cfg
}
