package model

type KLine struct {
    Symbol    string  `json:"symbol"`
    Cycle     string  `json:"cycle"` // "1m", "1h", "1d"
    Open      float64 `json:"open"`
    High      float64 `json:"high"`
    Low       float64 `json:"low"`
    Close     float64 `json:"close"`
    Volume    float64 `json:"volume"`
    Timestamp int64   `json:"timestamp"`
}
