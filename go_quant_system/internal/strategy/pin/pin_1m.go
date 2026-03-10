package pin

import "go_quant_system/pkg/model"

// Pin1mStrategy 1分钟插针策略（1min 3% & 1h价格真空）
type Pin1mStrategy struct {
    Threshold float64 // 0.03
}

func (s *Pin1mStrategy) CheckSignal(klines []model.KLine) bool {
    // 逻辑实现...
    return false
}
