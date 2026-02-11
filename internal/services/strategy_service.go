package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/adk/model"
	"google.golang.org/genai"

	"github.com/run-bigpig/jcp/internal/logger"
	"github.com/run-bigpig/jcp/internal/models"
)

var strategyLog = logger.New("strategy")

// 内置策略 - 使用默认agent配置作为专家组合
var builtinStrategies = []models.Strategy{
	{
		ID:          "default",
		Name:        "均衡分析",
		Description: "六大专家全面分析",
		Icon:        "⚖️",
		Color:       "#64748B",
		Agents:      getDefaultStrategyAgents(),
		IsBuiltin:   true,
		Source:      "builtin",
	},
}

// getDefaultStrategyAgents 获取默认策略专家配置
func getDefaultStrategyAgents() []models.StrategyAgent {
	return []models.StrategyAgent{
		{
			ID:          "fundamental",
			Name:        "老陈",
			Role:        "基本面研究员",
			Avatar:      "财",
			Color:       "#10B981",
			Instruction: "你是老陈，一位在券商研究所深耕15年的基本面研究员。你说话沉稳务实，喜欢用数据说话。\n\n【分析框架】\n1. 盈利能力：ROE、毛利率、净利率趋势\n2. 成长性：营收/利润增速，行业天花板\n3. 估值水平：PE/PB分位，与同行对比\n4. 财务健康：现金流、负债率、商誉风险\n\n【回复风格】简洁专业，150字以内。先给结论，再用核心数据支撑。",
			Tools:       []string{"get_research_report", "get_report_content", "get_stock_realtime"},
			Enabled:     true,
		},
		{
			ID:          "technical",
			Name:        "K线王",
			Role:        "技术分析师",
			Avatar:      "K",
			Color:       "#3B82F6",
			Instruction: "你是K线王，混迹A股20年的技术派老炮。你相信'价格包含一切信息'。\n\n【分析框架】\n1. 趋势判断：均线系统、趋势线\n2. 形态识别：头肩顶底、双重顶底\n3. 量价关系：放量突破、缩量回调\n4. 技术指标：MACD、KDJ、RSI\n\n【回复风格】直接了当，150字以内。明确给出关键价位和操作建议。",
			Tools:       []string{"get_kline_data", "get_stock_realtime", "get_orderbook"},
			Enabled:     true,
		},
		{
			ID:          "capital",
			Name:        "钱姐",
			Role:        "资金流向分析师",
			Avatar:      "资",
			Color:       "#F59E0B",
			Instruction: "你是钱姐，私募圈出身的资金流向专家。你深谙'跟着主力走'的生存法则。\n\n【分析框架】\n1. 主力动向：大单净流入、主力持仓变化\n2. 北向资金：外资流向、重仓股变化\n3. 筹码分布：集中度、套牢盘、获利盘\n4. 盘口异动：大单托盘、压盘信号\n\n【回复风格】直白实在，150字以内。重点说清资金动向和主力意图。",
			Tools:       []string{"get_orderbook", "get_stock_realtime", "get_kline_data"},
			Enabled:     true,
		},
		{
			ID:          "policy",
			Name:        "政策通",
			Role:        "政策解读专家",
			Avatar:      "政",
			Color:       "#8B5CF6",
			Instruction: "你是政策通，前财经记者出身，现专注政策研究。擅长解读政策背后的投资机会。\n\n【分析框架】\n1. 宏观政策：货币政策、财政政策、产业政策\n2. 行业监管：准入门槛、合规要求、扶持方向\n3. 地方政策：区域规划、地方补贴\n4. 政策周期：出台节奏、执行力度\n\n【回复风格】有理有据，150字以内。点明政策要点和投资含义。",
			Tools:       []string{"get_news", "get_research_report", "get_stock_realtime"},
			Enabled:     true,
		},
		{
			ID:          "risk",
			Name:        "风控李",
			Role:        "风险控制师",
			Avatar:      "险",
			Color:       "#EF4444",
			Instruction: "你是风控李，曾在公募基金做过5年风控。养成了'先想风险再想收益'的习惯。\n\n【分析框架】\n1. 下行风险：最大回撤、支撑位破位风险\n2. 波动风险：振幅、beta值、流动性\n3. 事件风险：财报、解禁、政策不确定性\n4. 仓位建议：根据风险收益比给出建议\n\n【回复风格】冷静客观，150字以内。明确风险点和应对建议。",
			Tools:       []string{"get_kline_data", "get_stock_realtime", "get_research_report", "get_news"},
			Enabled:     true,
		},
		{
			ID:          "hottrend",
			Name:        "舆情师",
			Role:        "全网舆情分析专家",
			Avatar:      "舆",
			Color:       "#F97316",
			Instruction: "你是舆情师，专注全网热点追踪。监控微博、知乎、B站等平台热搜，擅长从社会热点中发现投资机会或风险。\n\n【分析框架】\n1. 热点识别：筛选与市场相关的话题\n2. 关联分析：热点对相关行业/个股的影响\n3. 情绪判断：通过讨论判断市场情绪\n4. 时效评估：热点的持续性和发酵可能\n\n【回复风格】信息量大但有重点，150字以内。先说热点，再分析影响。",
			Tools:       []string{"get_hottrend", "get_news", "get_stock_realtime"},
			Enabled:     true,
		},
	}
}

// StrategyService 策略服务
type StrategyService struct {
	configPath string
	store      models.StrategyStore
	llm        model.LLM
	mu         sync.RWMutex
}

// NewStrategyService 创建策略服务
func NewStrategyService(dataDir string) *StrategyService {
	s := &StrategyService{
		configPath: filepath.Join(dataDir, "strategies.json"),
	}
	s.load()
	return s
}

// load 加载策略配置
func (s *StrategyService) load() {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		strategyLog.Info("策略配置不存在，初始化默认配置")
		s.initDefault()
		return
	}

	if err := json.Unmarshal(data, &s.store); err != nil {
		strategyLog.Error("解析策略配置失败: %v", err)
		s.initDefault()
		return
	}

	// 确保内置策略存在
	s.ensureBuiltinStrategies()
	strategyLog.Info("加载策略配置成功，共 %d 个策略", len(s.store.Strategies))
}

// initDefault 初始化默认配置
func (s *StrategyService) initDefault() {
	s.store = models.StrategyStore{
		ActiveID:   "default",
		Strategies: builtinStrategies,
	}
	s.saveNoLock()
}

// ensureBuiltinStrategies 确保内置策略存在
func (s *StrategyService) ensureBuiltinStrategies() {
	existingIDs := make(map[string]bool)
	for _, st := range s.store.Strategies {
		existingIDs[st.ID] = true
	}

	for _, builtin := range builtinStrategies {
		if !existingIDs[builtin.ID] {
			s.store.Strategies = append(s.store.Strategies, builtin)
		}
	}
}

// save 保存配置（带锁）
func (s *StrategyService) save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveNoLock()
}

// saveNoLock 保存配置（不带锁）
func (s *StrategyService) saveNoLock() error {
	data, err := json.MarshalIndent(s.store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.configPath, data, 0644)
}

// GetAllStrategies 获取所有策略
func (s *StrategyService) GetAllStrategies() []models.Strategy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]models.Strategy, len(s.store.Strategies))
	copy(result, s.store.Strategies)
	return result
}

// GetActiveStrategy 获取当前激活的策略
func (s *StrategyService) GetActiveStrategy() *models.Strategy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, st := range s.store.Strategies {
		if st.ID == s.store.ActiveID {
			return &st
		}
	}
	return nil
}

// GetActiveID 获取当前激活策略ID
func (s *StrategyService) GetActiveID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.store.ActiveID
}

// SetActiveStrategy 设置当前激活策略
func (s *StrategyService) SetActiveStrategy(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 查找策略
	var found bool
	var strategyName string
	for _, st := range s.store.Strategies {
		if st.ID == id {
			found = true
			strategyName = st.Name
			break
		}
	}
	if !found {
		return fmt.Errorf("策略不存在: %s", id)
	}

	// 更新激活ID
	s.store.ActiveID = id
	if err := s.saveNoLock(); err != nil {
		return err
	}

	strategyLog.Info("切换策略: %s (%s)", strategyName, id)
	return nil
}

// AddStrategy 添加新策略
func (s *StrategyService) AddStrategy(strategy models.Strategy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查ID是否重复
	for _, st := range s.store.Strategies {
		if st.ID == strategy.ID {
			return fmt.Errorf("策略ID已存在: %s", strategy.ID)
		}
	}

	// 设置创建时间
	if strategy.CreatedAt == 0 {
		strategy.CreatedAt = time.Now().Unix()
	}

	s.store.Strategies = append(s.store.Strategies, strategy)
	return s.saveNoLock()
}

// UpdateStrategy 更新策略
func (s *StrategyService) UpdateStrategy(strategy models.Strategy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, st := range s.store.Strategies {
		if st.ID == strategy.ID {
			// 内置策略不允许修改核心字段
			if st.IsBuiltin {
				strategy.IsBuiltin = true
				strategy.Source = "builtin"
			}
			s.store.Strategies[i] = strategy
			return s.saveNoLock()
		}
	}
	return fmt.Errorf("策略不存在: %s", strategy.ID)
}

// DeleteStrategy 删除策略
func (s *StrategyService) DeleteStrategy(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, st := range s.store.Strategies {
		if st.ID == id {
			if st.IsBuiltin {
				return fmt.Errorf("内置策略不可删除")
			}
			// 当前激活的策略不允许删除
			if s.store.ActiveID == id {
				return fmt.Errorf("当前激活的策略不可删除，请先切换到其他策略")
			}
			s.store.Strategies = append(s.store.Strategies[:i], s.store.Strategies[i+1:]...)
			return s.saveNoLock()
		}
	}
	return fmt.Errorf("策略不存在: %s", id)
}

// AddAgentToActiveStrategy 向当前激活策略添加专家
func (s *StrategyService) AddAgentToActiveStrategy(agent models.StrategyAgent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, st := range s.store.Strategies {
		if st.ID == s.store.ActiveID {
			// 检查ID是否重复
			for _, a := range st.Agents {
				if a.ID == agent.ID {
					return fmt.Errorf("专家ID已存在: %s", agent.ID)
				}
			}
			s.store.Strategies[i].Agents = append(s.store.Strategies[i].Agents, agent)
			return s.saveNoLock()
		}
	}
	return fmt.Errorf("当前策略不存在")
}

// UpdateAgentInActiveStrategy 更新当前激活策略中的专家
func (s *StrategyService) UpdateAgentInActiveStrategy(agent models.StrategyAgent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, st := range s.store.Strategies {
		if st.ID == s.store.ActiveID {
			for j, a := range st.Agents {
				if a.ID == agent.ID {
					s.store.Strategies[i].Agents[j] = agent
					return s.saveNoLock()
				}
			}
			return fmt.Errorf("专家不存在: %s", agent.ID)
		}
	}
	return fmt.Errorf("当前策略不存在")
}

// DeleteAgentFromActiveStrategy 从当前激活策略删除专家
func (s *StrategyService) DeleteAgentFromActiveStrategy(agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, st := range s.store.Strategies {
		if st.ID == s.store.ActiveID {
			for j, a := range st.Agents {
				if a.ID == agentID {
					s.store.Strategies[i].Agents = append(
						s.store.Strategies[i].Agents[:j],
						s.store.Strategies[i].Agents[j+1:]...,
					)
					return s.saveNoLock()
				}
			}
			return fmt.Errorf("专家不存在: %s", agentID)
		}
	}
	return fmt.Errorf("当前策略不存在")
}

// SetLLM 设置LLM用于AI生成策略
func (s *StrategyService) SetLLM(llm model.LLM) {
	s.llm = llm
}

// GenerateResult AI生成结果
type GenerateResult struct {
	Strategy  models.Strategy `json:"strategy"`
	Reasoning string          `json:"reasoning"`
}

// GenerateInput 策略生成输入
type GenerateInput struct {
	Prompt     string           // 用户描述
	Tools      []ToolInfoForGen // 可用工具列表
	MCPServers []MCPInfoForGen  // MCP服务器列表
}

// ToolInfoForGen 工具信息（用于生成）
type ToolInfoForGen struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// MCPInfoForGen MCP服务器信息（用于生成）
type MCPInfoForGen struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Tools []string `json:"tools"` // 该服务器提供的工具列表
}

// Generate 根据用户描述生成策略
func (s *StrategyService) Generate(ctx context.Context, input GenerateInput) (*GenerateResult, error) {
	if s.llm == nil {
		return nil, fmt.Errorf("LLM未配置")
	}
	strategyLog.Info("开始生成策略, prompt=%s", input.Prompt)

	// 构建AI提示词
	aiPrompt := s.buildGeneratePrompt(input)

	// 调用LLM
	response, err := s.callLLM(ctx, aiPrompt)
	if err != nil {
		return nil, fmt.Errorf("调用LLM失败: %w", err)
	}

	// 解析结果
	result, err := s.parseGenerateResponse(response, input.Prompt)
	if err != nil {
		return nil, fmt.Errorf("解析结果失败: %w", err)
	}

	strategyLog.Info("策略生成完成: %s", result.Strategy.Name)
	return result, nil
}

// buildGeneratePrompt 构建AI提示词
func (s *StrategyService) buildGeneratePrompt(input GenerateInput) string {
	var sb strings.Builder
	sb.WriteString("你是投资策略设计专家。根据用户需求设计投资策略和专属团队成员。\n\n")

	// 核心约束
	sb.WriteString("## 核心约束\n")
	sb.WriteString("1. 每个成员必须是独立个体，专注于特定的分析维度或职能\n")
	sb.WriteString("2. 禁止创建汇总型/裁决型角色（如：总结专家、决策裁判、综合分析师等）\n")
	sb.WriteString("3. 成员可以是各类投资相关角色：分析师、交易员、研究员、风控官、行业专家、散户、游资等\n")

	// 动态生成可用工具列表
	sb.WriteString("## 可用内置工具\n")
	for _, t := range input.Tools {
		fmt.Fprintf(&sb, "- %s: %s\n", t.Name, t.Description)
	}
	sb.WriteString("\n")

	// 动态生成MCP服务器列表
	if len(input.MCPServers) > 0 {
		sb.WriteString("## 可用MCP服务器\n")
		sb.WriteString("当成员需要使用MCP服务器的工具时，在mcpServers字段中填写服务器ID即可。\n")
		sb.WriteString("注意：MCP工具不要写入tools字段，只需在mcpServers中指定服务器ID。\n\n")
		for _, m := range input.MCPServers {
			fmt.Fprintf(&sb, "### %s (ID: %s)\n", m.Name, m.ID)
			if len(m.Tools) > 0 {
				sb.WriteString("提供的工具：\n")
				for _, tool := range m.Tools {
					fmt.Fprintf(&sb, "- %s\n", tool)
				}
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("## 用户需求\n")
	sb.WriteString(input.Prompt)
	sb.WriteString("\n\n## 任务\n")
	sb.WriteString("根据用户需求，设计一个投资策略，包含4-6个团队成员。\n")
	sb.WriteString("每个成员需要有独特的分析视角和专业的系统指令。\n")
	sb.WriteString("重要：必须为每个成员分配合适的工具，确保tools字段包含该成员需要使用的具体工具名称。\n\n")

	sb.WriteString("## 输出格式（纯JSON）\n")
	sb.WriteString("```json\n")
	sb.WriteString(s.getOutputTemplate())
	sb.WriteString("\n```")

	return sb.String()
}

// getOutputTemplate 获取输出模板
func (s *StrategyService) getOutputTemplate() string {
	return `{
  "strategy": {
    "name": "策略名称",
    "description": "一句话描述",
    "icon": "emoji图标",
    "color": "#3B82F6",
    "agents": [
      {
        "id": "agent-1",
        "name": "成员名称",
        "role": "角色定位",
        "avatar": "单字头像",
        "color": "#颜色代码",
        "instruction": "# 角色定位\n你是...\n\n## 核心职责\n- 职责1\n- 职责2\n\n## 分析框架\n### 1. 分析维度一\n- 要点\n\n### 2. 分析维度二\n- 要点\n\n## 工具使用\n- 使用 get-stock-info 获取股票基本信息\n- 使用 get-kline-data 获取K线数据进行技术分析\n\n## 输出要求\n1. 要求一\n2. 要求二",
        "tools": ["get-stock-info", "get-kline-data"],
        "mcpServers": ["MCP服务器ID（可选）"]
      }
    ]
  },
  "reasoning": "设计理由"
}`
}

// callLLM 调用LLM生成内容
func (s *StrategyService) callLLM(ctx context.Context, prompt string) (string, error) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{
				Role:  "user",
				Parts: []*genai.Part{{Text: prompt}},
			},
		},
	}

	var result string
	for resp, err := range s.llm.GenerateContent(ctx, req, false) {
		if err != nil {
			return "", err
		}
		if resp != nil && resp.Content != nil {
			for _, part := range resp.Content.Parts {
				if part.Text != "" {
					result += part.Text
				}
			}
		}
	}
	return result, nil
}

// parseGenerateResponse 解析LLM响应
func (s *StrategyService) parseGenerateResponse(response, userPrompt string) (*GenerateResult, error) {
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("未找到有效JSON")
	}

	var result GenerateResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w", err)
	}

	// 生成策略ID
	strategyID := uuid.New().String()[:8]
	result.Strategy.ID = fmt.Sprintf("ai-%s", strategyID)
	result.Strategy.Source = "ai"
	result.Strategy.SourceMeta = userPrompt
	result.Strategy.CreatedAt = time.Now().Unix()

	// 为每个agent生成唯一ID并设置默认启用
	for i := range result.Strategy.Agents {
		result.Strategy.Agents[i].ID = fmt.Sprintf("ai-%s-%d", strategyID, i+1)
		result.Strategy.Agents[i].Enabled = true
	}

	return &result, nil
}

// extractJSON 从响应中提取JSON
func extractJSON(response string) string {
	// 尝试提取```json...```块
	start := strings.Index(response, "```json")
	if start != -1 {
		start += 7
		end := strings.Index(response[start:], "```")
		if end != -1 {
			return strings.TrimSpace(response[start : start+end])
		}
	}

	// 尝试提取{...}
	start = strings.Index(response, "{")
	if start != -1 {
		end := strings.LastIndex(response, "}")
		if end > start {
			return response[start : end+1]
		}
	}

	return ""
}

// getAgentConfigsFromStrategy 从当前策略获取Agent配置
func (s *StrategyService) getAgentConfigsFromStrategy() []models.AgentConfig {
	strategy := s.GetActiveStrategy()
	if strategy == nil {
		return nil
	}

	agents := make([]models.AgentConfig, len(strategy.Agents))
	for i, sa := range strategy.Agents {
		agents[i] = models.AgentConfig{
			ID:          sa.ID,
			Name:        sa.Name,
			Role:        sa.Role,
			Avatar:      sa.Avatar,
			Color:       sa.Color,
			Instruction: sa.Instruction,
			Tools:       sa.Tools,
			MCPServers:  sa.MCPServers,
			Enabled:     sa.Enabled,
			AIConfigID:  sa.AIConfigID,
		}
	}
	return agents
}

// GetAllAgents 获取所有Agent配置
func (s *StrategyService) GetAllAgents() []models.AgentConfig {
	return s.getAgentConfigsFromStrategy()
}

// GetEnabledAgents 获取已启用的Agent
func (s *StrategyService) GetEnabledAgents() []models.AgentConfig {
	agents := s.getAgentConfigsFromStrategy()
	var result []models.AgentConfig
	for _, agent := range agents {
		if agent.Enabled {
			result = append(result, agent)
		}
	}
	return result
}

// GetAgentByID 根据ID获取Agent
func (s *StrategyService) GetAgentByID(id string) *models.AgentConfig {
	agents := s.getAgentConfigsFromStrategy()
	for i := range agents {
		if agents[i].ID == id {
			return &agents[i]
		}
	}
	return nil
}

// GetAgentsByIDs 根据ID列表获取Agent
func (s *StrategyService) GetAgentsByIDs(ids []string) []models.AgentConfig {
	agents := s.getAgentConfigsFromStrategy()
	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}

	var result []models.AgentConfig
	for _, agent := range agents {
		if idSet[agent.ID] {
			result = append(result, agent)
		}
	}
	return result
}

// EnhancePromptInput 提示词增强输入
type EnhancePromptInput struct {
	OriginalPrompt string `json:"originalPrompt"` // 原始提示词
	AgentRole      string `json:"agentRole"`      // Agent角色
	AgentName      string `json:"agentName"`      // Agent名称
}

// EnhancePromptResult 提示词增强结果
type EnhancePromptResult struct {
	EnhancedPrompt string `json:"enhancedPrompt"` // 增强后的提示词
}

// EnhancePrompt 增强Agent提示词
func (s *StrategyService) EnhancePrompt(ctx context.Context, input EnhancePromptInput) (*EnhancePromptResult, error) {
	if s.llm == nil {
		return nil, fmt.Errorf("LLM未配置")
	}
	strategyLog.Info("开始增强提示词, agent=%s, role=%s", input.AgentName, input.AgentRole)

	// 构建AI提示词
	aiPrompt := s.buildEnhancePrompt(input)

	// 调用LLM
	response, err := s.callLLM(ctx, aiPrompt)
	if err != nil {
		return nil, fmt.Errorf("调用LLM失败: %w", err)
	}

	// 解析结果
	result, err := s.parseEnhanceResponse(response)
	if err != nil {
		return nil, fmt.Errorf("解析结果失败: %w", err)
	}

	strategyLog.Info("提示词增强完成")
	return result, nil
}

// buildEnhancePrompt 构建增强提示词的AI提示
func (s *StrategyService) buildEnhancePrompt(input EnhancePromptInput) string {
	var sb strings.Builder
	sb.WriteString("你是一位专业的 AI Agent 提示词工程师，擅长将简单的提示词扩展为结构化、专业的系统指令。\n\n")

	sb.WriteString("## 任务\n")
	sb.WriteString("将用户提供的原始提示词，扩展为一个完整、结构化的 Agent 系统指令。\n\n")

	sb.WriteString("## Agent 信息\n")
	fmt.Fprintf(&sb, "- 名称：%s\n", input.AgentName)
	fmt.Fprintf(&sb, "- 角色：%s\n", input.AgentRole)
	sb.WriteString("\n")

	sb.WriteString("## 原始提示词\n")
	sb.WriteString(input.OriginalPrompt)
	sb.WriteString("\n\n")

	sb.WriteString("## 增强要求\n")
	sb.WriteString("1. 保持原始意图，但使其更加清晰、专业\n")
	sb.WriteString("2. 添加结构化的分析框架或工作流程\n")
	sb.WriteString("3. 明确输出格式和要求\n")
	sb.WriteString("4. 添加角色定位和核心职责\n")
	sb.WriteString("5. 使用 Markdown 格式组织内容\n")
	sb.WriteString("6. 保持简洁，避免冗余\n\n")

	sb.WriteString("## 输出格式（纯JSON）\n")
	sb.WriteString("```json\n")
	sb.WriteString(`{
  "enhancedPrompt": "增强后的完整提示词（使用Markdown格式）"
}`)
	sb.WriteString("\n```")

	return sb.String()
}

// parseEnhanceResponse 解析增强响应
func (s *StrategyService) parseEnhanceResponse(response string) (*EnhancePromptResult, error) {
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("未找到有效JSON")
	}

	var result EnhancePromptResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w", err)
	}

	return &result, nil
}
