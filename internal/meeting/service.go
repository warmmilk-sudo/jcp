package meeting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/run-bigpig/jcp/internal/adk"
	"github.com/run-bigpig/jcp/internal/adk/mcp"
	"github.com/run-bigpig/jcp/internal/adk/tools"
	"github.com/run-bigpig/jcp/internal/logger"
	"github.com/run-bigpig/jcp/internal/models"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// 日志实例
var log = logger.New("Meeting")

// 超时配置常量
const (
	MeetingTimeout       = 5 * time.Minute  // 整个会议的最大时长
	AgentTimeout         = 90 * time.Second // 单个专家发言的最大时长
	ModeratorTimeout     = 60 * time.Second // 小韭菜分析/总结的最大时长
	ModelCreationTimeout = 10 * time.Second // 模型创建的最大时长
)

// 错误定义
var (
	ErrMeetingTimeout   = errors.New("会议超时，已返回部分结果")
	ErrAgentTimeout     = errors.New("专家响应超时")
	ErrModeratorTimeout = errors.New("小韭菜响应超时")
	ErrNoAIConfig       = errors.New("未配置 AI 服务")
	ErrNoAgents         = errors.New("没有可用的专家")
)

// Service 会议室服务，编排多专家并行分析
type Service struct {
	modelFactory *adk.ModelFactory
	toolRegistry *tools.Registry
	mcpManager   *mcp.Manager
}

// NewService 创建会议室服务
func NewService() *Service {
	return &Service{
		modelFactory: adk.NewModelFactory(),
	}
}

// NewServiceWithTools 创建带工具的会议室服务
func NewServiceWithTools(registry *tools.Registry) *Service {
	return &Service{
		modelFactory: adk.NewModelFactory(),
		toolRegistry: registry,
	}
}

// NewServiceFull 创建完整配置的会议室服务
func NewServiceFull(registry *tools.Registry, mcpMgr *mcp.Manager) *Service {
	return &Service{
		modelFactory: adk.NewModelFactory(),
		toolRegistry: registry,
		mcpManager:   mcpMgr,
	}
}

// ChatRequest 聊天请求
type ChatRequest struct {
	Stock        models.Stock          `json:"stock"`
	KLineData    []models.KLineData    `json:"klineData"`
	Agents       []models.AgentConfig  `json:"agents"`
	Query        string                `json:"query"`
	ReplyContent string                `json:"replyContent"`
	AllAgents    []models.AgentConfig  `json:"allAgents"` // 所有可用专家（智能模式用）
	Position     *models.StockPosition `json:"position"`  // 用户持仓信息
}

// ChatResponse 聊天响应
type ChatResponse struct {
	AgentID   string `json:"agentId"`
	AgentName string `json:"agentName"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Round     int    `json:"round"`
	MsgType   string `json:"msgType"` // opening/opinion/summary
}

// ResponseCallback 响应回调函数类型
// 每当有新的发言产生时调用，用于实时推送到前端
type ResponseCallback func(resp ChatResponse)

// ProgressEvent 进度事件（细粒度实时反馈）
type ProgressEvent struct {
	Type      string `json:"type"`      // thinking/tool_call/tool_result/streaming/agent_start/agent_done
	AgentID   string `json:"agentId"`   // 当前专家 ID
	AgentName string `json:"agentName"` // 当前专家名称
	Detail    string `json:"detail"`    // 工具名称或阶段描述
	Content   string `json:"content"`   // 流式文本片段或工具结果摘要
}

// ProgressCallback 进度回调函数类型
type ProgressCallback func(event ProgressEvent)

// SendMessage 发送会议消息，生成多专家回复（并行执行）
func (s *Service) SendMessage(ctx context.Context, aiConfig *models.AIConfig, req ChatRequest) ([]ChatResponse, error) {
	llm, err := s.modelFactory.CreateModel(ctx, aiConfig)
	if err != nil {
		log.Error("CreateModel error: %v", err)
		return nil, err
	}
	log.Info("model created successfully")

	return s.runAgentsParallel(ctx, llm, req)
}

// RunSmartMeeting 智能会议模式（小韭菜编排）
// 专家按顺序串行发言，后一个专家可以参考前面的发言内容
func (s *Service) RunSmartMeeting(ctx context.Context, aiConfig *models.AIConfig, req ChatRequest) ([]ChatResponse, error) {
	return s.RunSmartMeetingWithCallback(ctx, aiConfig, req, nil, nil)
}

// RunSmartMeetingWithCallback 智能会议模式（带实时回调）
// respCallback 在每个发言完成后调用
// progressCallback 在工具调用、流式输出等细粒度事件时调用
func (s *Service) RunSmartMeetingWithCallback(ctx context.Context, aiConfig *models.AIConfig, req ChatRequest, respCallback ResponseCallback, progressCallback ProgressCallback) ([]ChatResponse, error) {
	if aiConfig == nil {
		return nil, ErrNoAIConfig
	}
	if len(req.AllAgents) == 0 {
		return nil, ErrNoAgents
	}

	// 设置整个会议的超时上下文
	meetingCtx, meetingCancel := context.WithTimeout(ctx, MeetingTimeout)
	defer meetingCancel()

	// 创建模型（带超时）
	modelCtx, modelCancel := context.WithTimeout(meetingCtx, ModelCreationTimeout)
	llm, err := s.modelFactory.CreateModel(modelCtx, aiConfig)
	modelCancel()
	if err != nil {
		return nil, fmt.Errorf("create model error: %w", err)
	}

	var responses []ChatResponse
	moderator := NewModerator(llm)

	log.Info("stock: %s, query: %s, agents: %d", req.Stock.Symbol, req.Query, len(req.AllAgents))

	// 第0轮：小韭菜分析意图并选择专家（带超时）
	if progressCallback != nil {
		progressCallback(ProgressEvent{
			Type:      "agent_start",
			AgentID:   "moderator",
			AgentName: "小韭菜",
			Detail:    "分析问题意图",
		})
	}

	moderatorCtx, moderatorCancel := context.WithTimeout(meetingCtx, ModeratorTimeout)
	decision, err := moderator.Analyze(moderatorCtx, &req.Stock, req.Query, req.AllAgents)
	moderatorCancel()

	if err != nil {
		if progressCallback != nil {
			progressCallback(ProgressEvent{
				Type:      "agent_done",
				AgentID:   "moderator",
				AgentName: "小韭菜",
			})
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: 小韭菜分析超时", ErrModeratorTimeout)
		}
		return nil, fmt.Errorf("moderator analyze error: %w", err)
	}

	if progressCallback != nil {
		progressCallback(ProgressEvent{
			Type:      "agent_done",
			AgentID:   "moderator",
			AgentName: "小韭菜",
		})
	}

	log.Debug("decision: selected=%v, topic=%s", decision.Selected, decision.Topic)

	// 添加开场白并立即回调
	openingResp := ChatResponse{
		AgentID:   "moderator",
		AgentName: "小韭菜",
		Role:      "会议主持",
		Content:   decision.Opening,
		Round:     0,
		MsgType:   "opening",
	}
	responses = append(responses, openingResp)
	if respCallback != nil {
		respCallback(openingResp)
	}

	// 筛选被选中的专家（按小韭菜选择的顺序）
	selectedAgents := s.filterAgentsOrdered(req.AllAgents, decision.Selected)
	if len(selectedAgents) == 0 {
		return responses, nil
	}

	// 第1轮：专家串行发言，后一个参考前面的内容
	var history []DiscussionEntry
	builder := s.createBuilder(llm)

	for i, agentCfg := range selectedAgents {
		// 检查会议是否已超时
		select {
		case <-meetingCtx.Done():
			log.Warn("meeting timeout, got %d responses", len(responses))
			return responses, ErrMeetingTimeout
		default:
		}

		log.Debug("agent %d/%d: %s starting", i+1, len(selectedAgents), agentCfg.Name)

		// 发送专家开始事件
		if progressCallback != nil {
			progressCallback(ProgressEvent{
				Type:      "agent_start",
				AgentID:   agentCfg.ID,
				AgentName: agentCfg.Name,
				Detail:    agentCfg.Role,
			})
		}

		// 构建前面专家发言的上下文
		previousContext := s.buildPreviousContext(history)

		// 运行单个专家（带超时控制）
		agentCtx, agentCancel := context.WithTimeout(meetingCtx, AgentTimeout)
		content, err := s.runSingleAgentWithHistory(agentCtx, builder, &agentCfg, &req.Stock, req.Query, previousContext, progressCallback, req.Position)
		agentCancel()

		if err != nil {
			// 发送专家完成事件（即使失败）
			if progressCallback != nil {
				progressCallback(ProgressEvent{
					Type:      "agent_done",
					AgentID:   agentCfg.ID,
					AgentName: agentCfg.Name,
				})
			}
			if errors.Is(err, context.DeadlineExceeded) {
				log.Warn("agent %s timeout", agentCfg.ID)
			} else {
				log.Error("agent %s error: %v", agentCfg.ID, err)
			}
			continue
		}

		// 发送专家完成事件
		if progressCallback != nil {
			progressCallback(ProgressEvent{
				Type:      "agent_done",
				AgentID:   agentCfg.ID,
				AgentName: agentCfg.Name,
			})
		}

		// 添加到响应并立即回调
		resp := ChatResponse{
			AgentID:   agentCfg.ID,
			AgentName: agentCfg.Name,
			Role:      agentCfg.Role,
			Content:   content,
			Round:     1,
			MsgType:   "opinion",
		}
		responses = append(responses, resp)
		if respCallback != nil {
			respCallback(resp)
		}

		// 记录到历史
		history = append(history, DiscussionEntry{
			Round:     1,
			AgentID:   agentCfg.ID,
			AgentName: agentCfg.Name,
			Role:      agentCfg.Role,
			Content:   content,
		})

		log.Debug("agent %s done, content len: %d", agentCfg.ID, len(content))
	}

	// 最终轮：小韭菜总结（带超时）
	if progressCallback != nil {
		progressCallback(ProgressEvent{
			Type:      "agent_start",
			AgentID:   "moderator",
			AgentName: "小韭菜",
			Detail:    "总结讨论",
		})
	}

	summaryCtx, summaryCancel := context.WithTimeout(meetingCtx, ModeratorTimeout)
	summary, err := moderator.Summarize(summaryCtx, &req.Stock, req.Query, history)
	summaryCancel()

	if progressCallback != nil {
		progressCallback(ProgressEvent{
			Type:      "agent_done",
			AgentID:   "moderator",
			AgentName: "小韭菜",
		})
	}

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Warn("summary timeout, returning partial results")
		} else {
			log.Error("summary error: %v", err)
		}
		// 总结失败不影响返回已有结果
		return responses, nil
	}

	if summary != "" {
		summaryResp := ChatResponse{
			AgentID:   "moderator",
			AgentName: "小韭菜",
			Role:      "会议主持",
			Content:   summary,
			Round:     2,
			MsgType:   "summary",
		}
		responses = append(responses, summaryResp)
		if respCallback != nil {
			respCallback(summaryResp)
		}
	}

	return responses, nil
}

// runAgentsParallel 并行运行多个 Agent（带超时控制）
func (s *Service) runAgentsParallel(ctx context.Context, llm model.LLM, req ChatRequest) ([]ChatResponse, error) {
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		responses []ChatResponse
	)

	// 设置整体超时
	parallelCtx, cancel := context.WithTimeout(ctx, MeetingTimeout)
	defer cancel()

	builder := s.createBuilder(llm)
	log.Debug("running %d agents in parallel", len(req.Agents))

	for _, agentConfig := range req.Agents {
		wg.Add(1)
		go func(cfg models.AgentConfig) {
			defer wg.Done()

			// 单个 Agent 超时控制
			agentCtx, agentCancel := context.WithTimeout(parallelCtx, AgentTimeout)
			defer agentCancel()

			content, err := s.runSingleAgentWithContext(agentCtx, builder, &cfg, &req.Stock, req.Query, req.ReplyContent, req.Position)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					log.Warn("agent %s timeout", cfg.ID)
				} else {
					log.Error("agent %s error: %v", cfg.ID, err)
				}
				return
			}

			mu.Lock()
			responses = append(responses, ChatResponse{
				AgentID:   cfg.ID,
				AgentName: cfg.Name,
				Role:      cfg.Role,
				Content:   content,
			})
			mu.Unlock()
			log.Debug("agent %s done, content len: %d", cfg.ID, len(content))
		}(agentConfig)
	}

	wg.Wait()
	log.Info("all agents done, got %d responses", len(responses))
	return responses, nil
}

// runSingleAgentWithContext 运行单个 Agent（支持引用上下文）
func (s *Service) runSingleAgentWithContext(ctx context.Context, builder *adk.ExpertAgentBuilder, cfg *models.AgentConfig, stock *models.Stock, query string, replyContent string, position *models.StockPosition) (string, error) {
	agentInstance, err := builder.BuildAgentWithContext(cfg, stock, query, replyContent, position)
	if err != nil {
		return "", err
	}

	sessionService := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        "jcp",
		Agent:          agentInstance,
		SessionService: sessionService,
	})
	if err != nil {
		return "", err
	}

	sessionID := fmt.Sprintf("session-%s-%d", cfg.ID, time.Now().UnixNano())
	_, err = sessionService.Create(ctx, &session.CreateRequest{
		AppName:   "jcp",
		UserID:    "user",
		SessionID: sessionID,
	})
	if err != nil {
		return "", fmt.Errorf("create session error: %w", err)
	}

	userMsg := &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			genai.NewPartFromText(query),
		},
	}

	var content string
	runCfg := agent.RunConfig{}
	for event, err := range r.Run(ctx, "user", sessionID, userMsg, runCfg) {
		if err != nil {
			return "", err
		}
		if event != nil && event.LLMResponse.Content != nil {
			for _, part := range event.LLMResponse.Content.Parts {
				if part.Thought {
					continue
				}
				if part.Text != "" {
					content += part.Text
				}
			}
		}
	}

	return content, nil
}

// filterAgents 根据 ID 列表筛选专家
func (s *Service) filterAgents(all []models.AgentConfig, ids []string) []models.AgentConfig {
	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}
	var result []models.AgentConfig
	for _, a := range all {
		if idSet[a.ID] {
			result = append(result, a)
		}
	}
	return result
}

// filterAgentsOrdered 按指定顺序筛选专家（保持小韭菜选择的顺序）
func (s *Service) filterAgentsOrdered(all []models.AgentConfig, ids []string) []models.AgentConfig {
	agentMap := make(map[string]models.AgentConfig)
	for _, a := range all {
		agentMap[a.ID] = a
	}
	var result []models.AgentConfig
	for _, id := range ids {
		if agent, ok := agentMap[id]; ok {
			result = append(result, agent)
		}
	}
	return result
}

// buildPreviousContext 构建前面专家发言的上下文
func (s *Service) buildPreviousContext(history []DiscussionEntry) string {
	if len(history) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("【前面专家的发言】\n")
	for _, entry := range history {
		sb.WriteString(fmt.Sprintf("- %s（%s）：%s\n\n", entry.AgentName, entry.Role, entry.Content))
	}
	return sb.String()
}

// runSingleAgentWithHistory 运行单个专家（带历史上下文和进度回调）
func (s *Service) runSingleAgentWithHistory(
	ctx context.Context,
	builder *adk.ExpertAgentBuilder,
	cfg *models.AgentConfig,
	stock *models.Stock,
	query string,
	previousContext string,
	progressCallback ProgressCallback,
	position *models.StockPosition,
) (string, error) {
	// 使用带上下文的方法构建 Agent
	agentInstance, err := builder.BuildAgentWithContext(cfg, stock, query, previousContext, position)
	if err != nil {
		return "", err
	}

	sessionService := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        "jcp",
		Agent:          agentInstance,
		SessionService: sessionService,
	})
	if err != nil {
		return "", err
	}

	sessionID := fmt.Sprintf("session-%s-%d", cfg.ID, time.Now().UnixNano())
	_, err = sessionService.Create(ctx, &session.CreateRequest{
		AppName:   "jcp",
		UserID:    "user",
		SessionID: sessionID,
	})
	if err != nil {
		return "", fmt.Errorf("create session error: %w", err)
	}

	userMsg := &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			genai.NewPartFromText(query),
		},
	}

	var content string
	runCfg := agent.RunConfig{
		StreamingMode: agent.StreamingModeSSE,
	}
	for event, err := range r.Run(ctx, "user", sessionID, userMsg, runCfg) {
		if err != nil {
			return "", err
		}
		if event == nil || event.LLMResponse.Content == nil {
			continue
		}

		for _, part := range event.LLMResponse.Content.Parts {
			if part.Thought {
				continue
			}

			// 检测工具调用
			if part.FunctionCall != nil && progressCallback != nil {
				progressCallback(ProgressEvent{
					Type:      "tool_call",
					AgentID:   cfg.ID,
					AgentName: cfg.Name,
					Detail:    part.FunctionCall.Name,
				})
			}

			// 检测工具结果
			if part.FunctionResponse != nil && progressCallback != nil {
				progressCallback(ProgressEvent{
					Type:      "tool_result",
					AgentID:   cfg.ID,
					AgentName: cfg.Name,
					Detail:    part.FunctionResponse.Name,
				})
			}

			// 流式文本
			if part.Text != "" {
				content += part.Text
				if progressCallback != nil && event.LLMResponse.Partial {
					progressCallback(ProgressEvent{
						Type:      "streaming",
						AgentID:   cfg.ID,
						AgentName: cfg.Name,
						Content:   part.Text,
					})
				}
			}
		}
	}

	return content, nil
}

// createBuilder 创建 ExpertAgentBuilder
func (s *Service) createBuilder(llm model.LLM) *adk.ExpertAgentBuilder {
	if s.mcpManager != nil {
		return adk.NewExpertAgentBuilderFull(llm, s.toolRegistry, s.mcpManager)
	}
	if s.toolRegistry != nil {
		return adk.NewExpertAgentBuilderWithTools(llm, s.toolRegistry)
	}
	return adk.NewExpertAgentBuilder(llm)
}
