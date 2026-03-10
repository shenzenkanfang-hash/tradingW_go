import os

def create_project():
    project_name = "go_quant_system"
    
    # 1. 定义需要创建的所有目录（按周期拆分）
    directories = [
        # cmd 入口
        "cmd/data_collector",
        "cmd/indicator_calculator",
        "cmd/risk_monitor",
        "cmd/strategy_executor",
        
        # pkg 基础库
        "pkg/client/binance/rest",
        "pkg/client/binance/ws",
        "pkg/client/redis",
        "pkg/config",
        "pkg/logger",
        "pkg/model",
        "pkg/utils",
        
        # internal 核心业务（按周期深层拆分）
        "internal/datasource/kline_1m",
        "internal/datasource/kline_1h",
        "internal/datasource/kline_1d",
        
        "internal/dataprocess/indicator_1m",
        "internal/dataprocess/indicator_1h",
        "internal/dataprocess/indicator_1d",
        
        "internal/risk",
        
        "internal/strategy/pin",
        "internal/strategy/trend",
        
        # 配置文件与日志
        "configs",
        "scripts",
        "test/unit/indicator_1m",
        "test/unit/indicator_1h",
        "test/integration",
        
        "logs/collector",
        "logs/indicator",
        "logs/strategy",
    ]

    # 2. 定义核心文件的基础代码
    files = {
        # 数据模型：支持 Cycle 字段
        "pkg/model/kline.go": """package model

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
""",
        # 核心策略接口骨架
        "internal/strategy/pin/pin_1m.go": """package pin

import "go_quant_system/pkg/model"

// Pin1mStrategy 1分钟插针策略（1min 3% & 1h价格真空）
type Pin1mStrategy struct {
    Threshold float64 // 0.03
}

func (s *Pin1mStrategy) CheckSignal(klines []model.KLine) bool {
    // 逻辑实现...
    return false
}
""",
        # 配置文件示例
        "configs/strategy_1m.yaml": """strategy:
  name: "pin_1m"
  threshold: 0.03
  leverage: 1
  scaling_steps: [1, 1, 1, 1, 1]
""",
        # Makefile：支持多目标构建
        "Makefile": """.PHONY: build test clean

build:
	go build -o bin/collector ./cmd/data_collector
	go build -o bin/calculator ./cmd/indicator_calculator
	go build -o bin/executor ./cmd/strategy_executor

test:
	go test ./internal/...

clean:
	rm -rf bin/*
""",
        "go.mod": f"module {project_name}\n\ngo 1.21",
        "scripts/setup.sh": "#!/bin/bash\necho \"Project initialization complete\"",
    }

    print(f"📁 正在初始化 Go 量化交易项目: {project_name}")

    # 创建文件夹
    for folder in directories:
        path = os.path.join(project_name, folder)
        os.makedirs(path, exist_ok=True)
        # 保证 Git 能识别空文件夹
        with open(os.path.join(path, ".gitkeep"), "w") as f:
            pass

    # 创建文件并写入初始内容
    for file_path, content in files.items():
        full_path = os.path.join(project_name, file_path)
        # 确保父目录存在（防止 files 里的键没在 directories 里定义）
        os.makedirs(os.path.dirname(full_path), exist_ok=True)
        with open(full_path, "w", encoding="utf-8") as f:
            f.write(content)

    print(f"✅ 成功生成目录结构及核心文件！")
    print(f"🚀 建议执行: cd {project_name} && go mod tidy")

if __name__ == "__main__":
    create_project()