export interface Stock {
  symbol: string;
  name: string;
  price: number;
  change: number;
  changePercent: number;
  volume: number;
  amount: number;
  marketCap: string;
  sector: string;
  open: number;
  high: number;
  low: number;
  preClose: number;
}

// 股票持仓信息
export interface StockPosition {
  shares: number;    // 持仓数量
  costPrice: number; // 成本价
}

export interface KLineData {
  time: string;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
  avg?: number; // For intraday average price line
  // 均线数据
  ma5?: number;
  ma10?: number;
  ma20?: number;
}

export interface OrderBookItem {
  price: number;
  size: number;
  total: number;
  percent: number; // For visual bar depth
}

export interface OrderBook {
  bids: OrderBookItem[];
  asks: OrderBookItem[];
}

export enum AgentRole {
  BULL = '多头分析师',
  BEAR = '空头怀疑论者',
  QUANT = '技术量化专家',
  MACRO = '宏观经济学家',
  NEWS = '市场情报员'
}

export interface Agent {
  id: string;
  name: string;
  role: AgentRole;
  avatar: string;
  color: string;
}

export interface ChatMessage {
  id: string;
  agentId: string;
  agentName?: string;
  role?: string;
  content: string;
  timestamp: number;
  replyTo?: string;
  mentions?: string[];
  round?: number;        // 讨论轮次
  msgType?: MsgType;     // 消息类型
}

// 消息类型
export type MsgType = 'opening' | 'opinion' | 'summary';

export type TimePeriod = '1m' | '1d' | '1w' | '1mo';

// 快讯数据结构
export interface Telegraph {
  time: string;
  content: string;
  url: string;
}

// MCP 传输类型
export type MCPTransportType = 'http' | 'sse' | 'command';

// MCP 服务器配置
export interface MCPServerConfig {
  id: string;
  name: string;
  transportType: MCPTransportType;
  endpoint: string;
  command: string;
  args: string[];
  toolFilter: string[];
  enabled: boolean;
}

// 大盘指数数据
export interface MarketIndex {
  code: string;          // 指数代码
  name: string;          // 指数名称
  price: number;         // 当前点位
  change: number;        // 涨跌点数
  changePercent: number; // 涨跌幅(%)
  volume: number;        // 成交量(手)
  amount: number;        // 成交额(万元)
}

// 市场状态
export interface MarketStatus {
  status: string;        // trading, closed, pre_market, lunch_break
  statusText: string;    // 中文状态描述
  isTradeDay: boolean;   // 是否交易日
  holidayName: string;   // 节假日名称
}
