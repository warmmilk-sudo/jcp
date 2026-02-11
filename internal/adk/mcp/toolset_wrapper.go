// Package mcp 提供 MCP (Model Context Protocol) 集成功能
package mcp

import (
	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

// prefixedToolset 包装原始 toolset，为工具名称添加前缀支持
// 解决 ADK 中 LLM 返回带前缀工具名与 toolsDict key 不匹配的问题
type prefixedToolset struct {
	inner  tool.Toolset
	prefix string
}

// NewPrefixedToolset 创建带前缀支持的 toolset 包装器
func NewPrefixedToolset(inner tool.Toolset, prefix string) tool.Toolset {
	return &prefixedToolset{inner: inner, prefix: prefix}
}

func (p *prefixedToolset) Name() string {
	return p.inner.Name()
}

func (p *prefixedToolset) Tools(ctx agent.ReadonlyContext) ([]tool.Tool, error) {
	tools, err := p.inner.Tools(ctx)
	if err != nil {
		return nil, err
	}
	wrapped := make([]tool.Tool, len(tools))
	for i, t := range tools {
		wrapped[i] = &prefixedTool{inner: t, prefix: p.prefix}
	}
	log.Debug("获取工具列表: prefix=%s, 数量=%d", p.prefix, len(tools))
	return wrapped, nil
}

// functionTool 定义 MCP 工具需要实现的接口（与 ADK 内部接口一致）
type functionTool interface {
	tool.Tool
	Declaration() *genai.FunctionDeclaration
	Run(ctx tool.Context, args any) (map[string]any, error)
}

// prefixedTool 包装原始工具，确保 Name() 和 Declaration().Name 一致
type prefixedTool struct {
	inner  tool.Tool
	prefix string
}

func (p *prefixedTool) Name() string {
	return p.prefix + ":" + p.inner.Name()
}

func (p *prefixedTool) Description() string {
	return p.inner.Description()
}

func (p *prefixedTool) IsLongRunning() bool {
	return p.inner.IsLongRunning()
}

func (p *prefixedTool) Declaration() *genai.FunctionDeclaration {
	inner, ok := p.inner.(functionTool)
	if !ok {
		return nil
	}
	decl := inner.Declaration()
	if decl == nil {
		return nil
	}
	return &genai.FunctionDeclaration{
		Name:                 p.Name(), // 使用带前缀的名称
		Description:          decl.Description,
		ParametersJsonSchema: decl.ParametersJsonSchema,
		ResponseJsonSchema:   decl.ResponseJsonSchema,
	}
}

func (p *prefixedTool) Run(ctx tool.Context, args any) (map[string]any, error) {
	inner, ok := p.inner.(functionTool)
	if !ok {
		return nil, nil
	}
	log.Info("MCP 工具调用: %s", p.Name())
	result, err := inner.Run(ctx, args)
	if err != nil {
		log.Error("MCP 工具执行失败: %s, %v", p.Name(), err)
		return nil, err
	}
	return result, nil
}

// ProcessRequest 自己实现注册逻辑，确保 req.Tools 的 key 与 Declaration().Name 一致
func (p *prefixedTool) ProcessRequest(ctx tool.Context, req *model.LLMRequest) error {
	if req.Tools == nil {
		req.Tools = make(map[string]any)
	}

	name := p.Name()
	if _, ok := req.Tools[name]; ok {
		return nil // 已存在，跳过
	}
	req.Tools[name] = p

	decl := p.Declaration()
	if decl == nil {
		return nil
	}

	if req.Config == nil {
		req.Config = &genai.GenerateContentConfig{}
	}

	// 查找或创建 FunctionDeclarations
	var funcTool *genai.Tool
	for _, t := range req.Config.Tools {
		if t != nil && t.FunctionDeclarations != nil {
			funcTool = t
			break
		}
	}
	if funcTool == nil {
		req.Config.Tools = append(req.Config.Tools, &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{decl},
		})
	} else {
		funcTool.FunctionDeclarations = append(funcTool.FunctionDeclarations, decl)
	}

	return nil
}
