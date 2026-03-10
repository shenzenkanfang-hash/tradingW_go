package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"go_quant_system/pkg/logger"
)

// -------------------------- 错误类型 --------------------------
var (
	ErrRateLimitRisk    = errors.New("权重使用率超过阈值，请求被软拦截")
	ErrNetwork          = errors.New("网络请求失败（可重试）")
	ErrBusiness         = errors.New("业务逻辑错误（不可重试）")
	ErrRateLimit        = errors.New("限流错误（需等待/切换IP）")
	ErrSignatureExpired = errors.New("签名时间戳过期")
	ErrRedisLimit       = errors.New("Redis权重监控拦截")
)

// -------------------------- 产品线枚举 --------------------------
type ProductType string

const (
	ProductSpot     ProductType = "spot"     // 现货
	ProductFutures  ProductType = "futures"  // U本位期货
	ProductDelivery ProductType = "delivery" // 交割合约
	ProductTestnet  ProductType = "testnet"  // 期货测试网
)

// -------------------------- API响应结构 --------------------------
type APIResponse struct {
	RawBody    []byte      // 原始响应体
	StatusCode int         // HTTP状态码
	Headers    http.Header // 响应头
	UsedWeight float64     // 已使用权重
	RequestID  string      // 请求ID
}

type APIError struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// -------------------------- 核心接口抽象 --------------------------
// ExchangeGateway 交易所网关接口（便于扩展其他交易所）
type ExchangeGateway interface {
	Request(ctx context.Context, method, endpoint string, params map[string]string, signed bool) (*APIResponse, error)
	CalibrateTime(ctx context.Context) error
	Close() error
	SetAuth(apiKey, apiSecret string)
	SetWeightLimitRatio(ratio float64)
}

// -------------------------- 核心客户端 --------------------------
type Client struct {
	// 基础配置
	productType ProductType
	baseURL     string
	apiKey      string
	apiSecret   string
	cfg         GatewayConfig // 完整配置

	// 运行时状态
	httpClient  *http.Client
	timeOffset  int64        // 时间偏移（毫秒）
	lastWeight  float64      // 上一次权重
	redisClient *RedisClient // Redis客户端
	logger      logger.Logger

	// 并发安全
	mu             sync.RWMutex
	hmacPool       sync.Pool
	cacheMu        sync.Mutex
	lastRedisCheck int64 // 上次Redis检查时间（毫秒）
	cachedBlock    bool  // 缓存的受限标记
}

// NewClient 创建币安网关客户端
func NewClient(productType ProductType, cfg GatewayConfig, logger logger.Logger) ExchangeGateway {
	// 兜底默认配置
	if cfg.HTTP.MaxIdleConns == 0 {
		defaultCfg := DefaultConfig()
		cfg.HTTP = defaultCfg.HTTP
		cfg.Redis = defaultCfg.Redis
		cfg.Weight = defaultCfg.Weight
		cfg.Time = defaultCfg.Time
	}

	// 构建HTTP客户端（连接池）
	transport := &http.Transport{
		MaxIdleConns:        cfg.HTTP.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.HTTP.MaxIdleConnsPerHost,
		IdleConnTimeout:     cfg.HTTP.IdleConnTimeout,
		TLSHandshakeTimeout: cfg.HTTP.TLSHandshakeTimeout,
		DisableCompression:  false,
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   cfg.HTTP.RequestTimeout,
	}

	// 创建Redis客户端（可选，nil则禁用Redis权重监控）
	var redisCli *RedisClient
	if cfg.Redis.Addr != "" {
		redisCfg := RedisConfig{
			Addr:                 cfg.Redis.Addr,
			Password:             cfg.Redis.Password,
			DBName:               cfg.Redis.DBName,
			ApiLimitKey:          cfg.Redis.ApiLimitKey,
			CacheWindow:          cfg.Redis.CacheWindow,
			WeightBlockThreshold: cfg.Redis.WeightBlockThreshold,
			WeightWarnThreshold:  cfg.Redis.WeightWarnThreshold,
			Timeout:              cfg.Redis.Timeout,
		}
		redisCli = NewRedisClient(redisCfg, logger)
	}

	// 初始化客户端
	c := &Client{
		productType: productType,
		baseURL:     getBaseURL(productType),
		cfg:         cfg,
		httpClient:  httpClient,
		logger:      logger,
		redisClient: redisCli,
		hmacPool: sync.Pool{
			New: func() interface{} {
				return hmac.New(sha256.New, []byte(""))
			},
		},
	}

	// 自动校准时间
	if err := c.CalibrateTime(context.Background()); err != nil {
		logger.Warn("calibrate time failed on init", logger.Err(err))
	}

	return c
}

// SetAuth 设置API密钥
func (c *Client) SetAuth(apiKey, apiSecret string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.apiKey = apiKey
	c.apiSecret = apiSecret

	// 重置HMAC池
	c.hmacPool = sync.Pool{
		New: func() interface{} {
			return hmac.New(sha256.New, []byte(apiSecret))
		},
	}
}

// SetWeightLimitRatio 设置权重限流比例
func (c *Client) SetWeightLimitRatio(ratio float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ratio > 0 && ratio <= 1 {
		c.cfg.Weight.LimitRatio = ratio
	}
}

// CalibrateTime 校准本地与服务器时间
func (c *Client) CalibrateTime(ctx context.Context) error {
	resp, err := c.Request(ctx, "GET", "/fapi/v1/time", nil, false)
	if err != nil {
		return fmt.Errorf("calibrate time failed: %w", err)
	}

	var timeResp struct {
		ServerTime int64 `json:"serverTime"`
	}
	if err := json.Unmarshal(resp.RawBody, &timeResp); err != nil {
		return fmt.Errorf("unmarshal server time failed: %w", err)
	}

	localTime := time.Now().UnixMilli()
	c.mu.Lock()
	c.timeOffset = timeResp.ServerTime - localTime
	c.mu.Unlock()

	c.logger.Info("time calibrated", logger.Int64("offset_ms", c.timeOffset))
	return nil
}

// Request 核心请求方法
func (c *Client) Request(ctx context.Context, method, endpoint string, params map[string]string, signed bool) (*APIResponse, error) {
	// 1. Redis权重监控拦截
	if c.redisClient != nil {
		if err := c.checkRedisWeightLimit(ctx); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrRedisLimit, err)
		}
	}

	// 2. 本地权重软拦截
	if err := c.checkLocalWeightLimit(); err != nil {
		return nil, err
	}

	// 3. 构建请求参数
	query := url.Values{}
	if params != nil {
		for k, v := range params {
			query.Set(k, v)
		}
	}

	// 4. 签名处理
	if signed {
		if c.apiKey == "" || c.apiSecret == "" {
			return nil, fmt.Errorf("%w: missing api key/secret", ErrBusiness)
		}

		c.mu.RLock()
		timestamp := time.Now().UnixMilli() + c.timeOffset
		recvWindow := c.cfg.Time.RecvWindow
		c.mu.RUnlock()

		query.Set("timestamp", strconv.FormatInt(timestamp, 10))
		query.Set("recvWindow", strconv.FormatInt(recvWindow, 10))

		signature, err := c.generateSignature(query.Encode())
		if err != nil {
			return nil, fmt.Errorf("generate signature failed: %w", err)
		}
		query.Set("signature", signature)
	}

	// 5. 构建URL
	var sb strings.Builder
	sb.WriteString(c.baseURL)
	sb.WriteString(endpoint)
	if len(query) > 0 {
		sb.WriteString("?")
		sb.WriteString(query.Encode())
	}
	fullURL := sb.String()

	// 6. 创建HTTP请求
	req, err := http.NewRequest(method, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: create request failed: %v", ErrNetwork, err)
	}
	req = req.WithContext(ctx)

	// 7. 设置请求头
	req.Header.Set("User-Agent", "BinanceProductionClient/2.0")
	if c.apiKey != "" {
		req.Header.Set("X-MBX-APIKEY", c.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")

	// 8. 发起请求（带重试）
	resp, err := c.doRequestWithRetry(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 9. 读取响应体
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read response body failed: %v", ErrNetwork, err)
	}

	// 10. 解析权重
	usedWeight := c.parseUsedWeight(resp.Header)
	c.mu.Lock()
	c.lastWeight = usedWeight
	c.mu.Unlock()

	// 11. 权重回填到Redis
	if c.redisClient != nil {
		if err := c.updateRedisWeightLimit(ctx, usedWeight); err != nil {
			c.logger.Warn("update redis weight limit failed", logger.Err(err))
		}
	}

	// 12. 封装响应
	apiResp := &APIResponse{
		RawBody:    rawBody,
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		UsedWeight: usedWeight,
		RequestID:  resp.Header.Get("X-MBX-REQUEST-ID"),
	}

	// 13. 错误处理
	if err := c.handleResponseError(apiResp); err != nil {
		return apiResp, err
	}

	return apiResp, nil
}

// checkRedisWeightLimit 检查Redis权重限制（带本地缓存）
func (c *Client) checkRedisWeightLimit(ctx context.Context) error {
	// 先查本地缓存
	c.cacheMu.Lock()
	now := time.Now().UnixMilli()
	cacheWindowMs := int64(c.cfg.Redis.CacheWindow / time.Millisecond)
	if now-c.lastRedisCheck < cacheWindowMs {
		if c.cachedBlock {
			c.cacheMu.Unlock()
			return errors.New("local cache marked as blocked")
		}
		c.cacheMu.Unlock()
		return nil
	}
	c.cacheMu.Unlock()

	// 缓存过期，查Redis
	wl, err := c.redisClient.GetWeightLimit(ctx)
	if err != nil {
		// Redis失败不拦截，仅记录
		c.logger.Warn("get redis weight limit failed", logger.Err(err))
		c.cacheMu.Lock()
		c.lastRedisCheck = now
		c.cachedBlock = false
		c.cacheMu.Unlock()
		return nil
	}

	// 跨分钟自动解锁
	currentMinute := time.Now().Minute()
	if wl.Minute != currentMinute {
		wl.Minute = currentMinute
		wl.LimitNum = 0
		wl.IsBlocked = false
		// 同步更新Redis
		_ = c.redisClient.SetWeightLimit(ctx, wl)
	}

	// 判断是否拦截
	blocked := wl.IsBlocked || wl.LimitNum >= c.cfg.Redis.WeightBlockThreshold
	c.cacheMu.Lock()
	c.lastRedisCheck = now
	c.cachedBlock = blocked
	c.cacheMu.Unlock()

	if blocked {
		return fmt.Errorf("weight %.2f >= block threshold %.2f or blocked",
			wl.LimitNum, c.cfg.Redis.WeightBlockThreshold)
	}

	return nil
}

// updateRedisWeightLimit 更新Redis权重
func (c *Client) updateRedisWeightLimit(ctx context.Context, usedWeight float64) error {
	currentMinute := time.Now().Minute()
	isBlocked := usedWeight >= c.cfg.Redis.WeightWarnThreshold

	wl := &WeightLimit{
		Minute:    currentMinute,
		LimitNum:  usedWeight,
		IsBlocked: isBlocked,
	}

	if err := c.redisClient.SetWeightLimit(ctx, wl); err != nil {
		return err
	}

	// 熔断日志
	if isBlocked {
		c.logger.Warn("weight limit triggered",
			logger.Float64("used_weight", usedWeight),
			logger.Float64("warn_threshold", c.cfg.Redis.WeightWarnThreshold))
	}

	return nil
}

// checkLocalWeightLimit 本地权重拦截
func (c *Client) checkLocalWeightLimit() error {
	c.mu.RLock()
	lastWeight := c.lastWeight
	limitRatio := c.cfg.Weight.LimitRatio
	maxWeight := c.cfg.Weight.MaxWeight
	c.mu.RUnlock()

	threshold := maxWeight * limitRatio
	if lastWeight >= threshold {
		return fmt.Errorf("%w: current weight %.2f >= threshold %.2f",
			ErrRateLimitRisk, lastWeight, threshold)
	}

	return nil
}

// generateSignature 生成签名
func (c *Client) generateSignature(data string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	h := c.hmacPool.Get().(hash.Hash)
	defer c.hmacPool.Put(h)

	h.Reset()
	if _, err := h.Write([]byte(data)); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// doRequestWithRetry 带重试的请求
func (c *Client) doRequestWithRetry(req *http.Request) (*http.Response, error) {
	maxRetries := 3
	backoff := 200 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		resp, err := c.httpClient.Do(req)
		if err == nil {
			return resp, nil
		}

		// 判断是否可重试
		if !isRetryableError(err) {
			return nil, fmt.Errorf("%w: non-retryable error: %v", ErrBusiness, err)
		}

		// 最后一次重试失败
		if i == maxRetries-1 {
			return nil, fmt.Errorf("%w: retry %d times failed: %v", ErrNetwork, maxRetries, err)
		}

		// 指数退避
		time.Sleep(backoff * (1 << i))
		c.logger.Debug("retrying request", logger.Int("retry_times", i+1), logger.Err(err))
	}

	return nil, fmt.Errorf("%w: retry exhausted", ErrNetwork)
}

// isRetryableError 判断是否是可重试错误
func isRetryableError(err error) bool {
	var netErr *url.Error
	if errors.As(err, &netErr) {
		return true
	}
	if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "connection") {
		return true
	}
	return false
}

// parseUsedWeight 解析已使用权重
func (c *Client) parseUsedWeight(header http.Header) float64 {
	weightKeys := []string{
		"x-mbx-used-weight-1m",
		"X-MBX-USED-WEIGHT-1M",
	}

	for _, key := range weightKeys {
		if val := header.Get(key); val != "" {
			weight, err := strconv.ParseFloat(val, 64)
			if err == nil {
				return weight
			}
		}
	}
	return 0
}

// handleResponseError 处理响应错误
func (c *Client) handleResponseError(resp *APIResponse) error {
	switch resp.StatusCode {
	case http.StatusOK:
		var apiErr APIError
		if err := json.Unmarshal(resp.RawBody, &apiErr); err == nil && apiErr.Code != 0 {
			switch apiErr.Code {
			case -1121, -2010:
				return fmt.Errorf("%w: code=%d, msg=%s", ErrBusiness, apiErr.Code, apiErr.Msg)
			case -1008:
				return fmt.Errorf("%w: code=%d, msg=%s", ErrRateLimit, apiErr.Code, apiErr.Msg)
			case -1021:
				_ = c.CalibrateTime(context.Background())
				return fmt.Errorf("%w: code=%d, msg=%s", ErrSignatureExpired, apiErr.Code, apiErr.Msg)
			default:
				return fmt.Errorf("%w: code=%d, msg=%s", ErrBusiness, apiErr.Code, apiErr.Msg)
			}
		}
		return nil

	case 429:
		return fmt.Errorf("%w: HTTP 429, used weight %.2f", ErrRateLimit, resp.UsedWeight)
	case 418:
		return fmt.Errorf("%w: HTTP 418, IP blocked", ErrRateLimit)
	case 403:
		return fmt.Errorf("%w: HTTP 403, WAF restricted", ErrBusiness)
	case 503:
		var errMsg map[string]string
		_ = json.Unmarshal(resp.RawBody, &errMsg)
		msg := errMsg["msg"]
		switch {
		case strings.Contains(msg, "Unknown error"):
			return fmt.Errorf("%w: HTTP 503, unknown status", ErrNetwork)
		case strings.Contains(msg, "Service Unavailable"):
			return fmt.Errorf("%w: HTTP 503, service unavailable", ErrNetwork)
		case strings.Contains(msg, "Request throttled"):
			return fmt.Errorf("%w: HTTP 503, system overload", ErrRateLimit)
		}
	default:
		return fmt.Errorf("unknown error: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	return nil
}

// Close 关闭客户端（释放资源）
func (c *Client) Close() error {
	var errs []error

	// 关闭Redis客户端
	if c.redisClient != nil {
		if err := c.redisClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close redis client failed: %w", err))
		}
	}

	// 关闭HTTP连接池
	c.httpClient.CloseIdleConnections()

	if len(errs) > 0 {
		return fmt.Errorf("close client failed: %v", errs)
	}
	return nil
}

// ParseError 解析API错误
func (r *APIResponse) ParseError() (*APIError, error) {
	var apiErr APIError
	if err := json.Unmarshal(r.RawBody, &apiErr); err != nil {
		return nil, fmt.Errorf("unmarshal error response failed: %w", err)
	}
	return &apiErr, nil
}

// getBaseURL 获取产品线BaseURL
func getBaseURL(productType ProductType) string {
	switch productType {
	case ProductSpot:
		return "https://api.binance.com"
	case ProductFutures:
		return "https://fapi.binance.com"
	case ProductDelivery:
		return "https://dapi.binance.com"
	case ProductTestnet:
		return "https://testnet.binancefuture.com"
	default:
		return "https://fapi.binance.com"
	}
}
