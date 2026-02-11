import { GetStrategies, GetActiveStrategyID, SetActiveStrategy, AddStrategy, UpdateStrategy, DeleteStrategy, GenerateStrategy, EnhancePrompt, GetAgentConfigs, AddAgentConfig, UpdateAgentConfig, DeleteAgentConfig } from '../../wailsjs/go/main/App';

// 策略专属专家配置
export interface StrategyAgent {
  id: string;
  name: string;
  role: string;
  avatar: string;
  color: string;
  instruction: string;
  tools: string[];
  mcpServers: string[];
  enabled: boolean;
  aiConfigId: string;
}

export interface Strategy {
  id: string;
  name: string;
  description: string;
  icon: string;
  color: string;
  agents: StrategyAgent[];
  isBuiltin: boolean;
  source: string;
  sourceMeta: string;
  createdAt: number;
}

export interface GenerateStrategyRequest {
  prompt: string;
}

export interface GenerateStrategyResponse {
  success: boolean;
  error?: string;
  strategy?: Strategy;
  reasoning?: string;
}

// 获取所有策略
export const getStrategies = async (): Promise<Strategy[]> => {
  return await GetStrategies();
};

// 获取当前激活策略ID
export const getActiveStrategyID = async (): Promise<string> => {
  return await GetActiveStrategyID();
};

// 设置当前激活策略
export const setActiveStrategy = async (id: string): Promise<string> => {
  return await SetActiveStrategy(id);
};

// 添加策略
export const addStrategy = async (strategy: Strategy): Promise<string> => {
  return await AddStrategy(strategy as any);
};

// 更新策略
export const updateStrategy = async (strategy: Strategy): Promise<string> => {
  return await UpdateStrategy(strategy as any);
};

// 删除策略
export const deleteStrategy = async (id: string): Promise<string> => {
  return await DeleteStrategy(id);
};

// AI生成策略
export const generateStrategy = async (prompt: string): Promise<GenerateStrategyResponse> => {
  return await GenerateStrategy({ prompt });
};

// 提示词增强请求
export interface EnhancePromptRequest {
  originalPrompt: string;
  agentRole: string;
  agentName: string;
}

// 提示词增强响应
export interface EnhancePromptResponse {
  success: boolean;
  enhancedPrompt?: string;
  error?: string;
}

// 增强Agent提示词
export const enhancePrompt = async (req: EnhancePromptRequest): Promise<EnhancePromptResponse> => {
  return await EnhancePrompt(req);
};

// ========== Agent Config API ==========

export interface AgentConfig {
  id: string;
  name: string;
  role: string;
  avatar: string;
  color: string;
  instruction: string;
  tools: string[];
  mcpServers: string[];
  enabled: boolean;
  aiConfigId: string;
}

// 获取所有已启用的Agent配置
export const getAgentConfigs = async (): Promise<AgentConfig[]> => {
  return await GetAgentConfigs();
};

// 添加Agent配置
export const addAgentConfig = async (config: AgentConfig): Promise<string> => {
  return await AddAgentConfig(config);
};

// 更新Agent配置
export const updateAgentConfig = async (config: AgentConfig): Promise<string> => {
  return await UpdateAgentConfig(config);
};

// 删除Agent配置
export const deleteAgentConfig = async (id: string): Promise<string> => {
  return await DeleteAgentConfig(id);
};
