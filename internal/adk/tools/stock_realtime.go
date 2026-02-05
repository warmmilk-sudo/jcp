package tools

import (
	"fmt"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// GetStockRealtimeInput 获取股票实时数据输入参数
type GetStockRealtimeInput struct {
	Codes []string `json:"codes" jsonschema:"股票代码列表，如 sh600519, sz000001"`
}

// GetStockRealtimeOutput 获取股票实时数据输出
type GetStockRealtimeOutput struct {
	Data        string `json:"data" jsonschema:"股票实时数据，包含价格、涨跌幅等信息"`
	MarketIndex string `json:"marketIndex" jsonschema:"大盘指数数据，包含上证指数、深证成指、创业板指等"`
}

// createStockRealtimeTool 创建股票实时数据工具
func (r *Registry) createStockRealtimeTool() (tool.Tool, error) {
	handler := func(ctx tool.Context, input GetStockRealtimeInput) (GetStockRealtimeOutput, error) {
		fmt.Printf("[Tool:get_stock_realtime] 调用开始, codes=%v\n", input.Codes)

		if len(input.Codes) == 0 {
			fmt.Println("[Tool:get_stock_realtime] 错误: 未提供股票代码")
			return GetStockRealtimeOutput{Data: "请提供股票代码"}, nil
		}

		stocks, err := r.marketService.GetStockRealTimeData(input.Codes...)
		if err != nil {
			fmt.Printf("[Tool:get_stock_realtime] 错误: %v\n", err)
			return GetStockRealtimeOutput{}, err
		}

		// 格式化股票数据输出
		var result string
		for _, s := range stocks {
			result += fmt.Sprintf("【%s(%s)】价格:%.2f 涨跌:%.2f%% 开盘:%.2f 最高:%.2f 最低:%.2f 成交量:%d\n",
				s.Name, s.Symbol, s.Price, s.ChangePercent, s.Open, s.High, s.Low, s.Volume)
		}

		// 获取大盘指数数据
		var marketIndexResult string
		indices, err := r.marketService.GetMarketIndices()
		if err != nil {
			fmt.Printf("[Tool:get_stock_realtime] 获取大盘指数失败: %v\n", err)
		} else {
			for _, idx := range indices {
				marketIndexResult += fmt.Sprintf("【%s】点位:%.2f 涨跌:%.2f(%.2f%%)\n",
					idx.Name, idx.Price, idx.Change, idx.ChangePercent)
			}
		}

		fmt.Printf("[Tool:get_stock_realtime] 调用完成, 返回%d条股票数据, %d条大盘数据\n", len(stocks), len(indices))
		return GetStockRealtimeOutput{Data: result, MarketIndex: marketIndexResult}, nil
	}

	return functiontool.New(functiontool.Config{
		Name:        "get_stock_realtime",
		Description: "获取股票实时行情数据，包括当前价格、涨跌幅、开盘价、最高价、最低价、成交量等，以及大盘指数数据",
	}, handler)
}
