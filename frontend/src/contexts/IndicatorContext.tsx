import React, { createContext, useContext, useState, useEffect, useCallback, ReactNode } from 'react';
import { getConfig } from '../services/configService';

// ========== 类型定义 ==========

export interface MAConfig {
  enabled: boolean;
  periods: number[];
}

export interface EMAConfig {
  enabled: boolean;
  periods: number[];
}

export interface BOLLConfig {
  enabled: boolean;
  period: number;
  multiplier: number;
}

export interface MACDConfig {
  enabled: boolean;
  fast: number;
  slow: number;
  signal: number;
}

export interface RSIConfig {
  enabled: boolean;
  period: number;
}

export interface KDJConfig {
  enabled: boolean;
  period: number;
  k: number;
  d: number;
}

export interface IndicatorConfig {
  ma: MAConfig;
  ema: EMAConfig;
  boll: BOLLConfig;
  macd: MACDConfig;
  rsi: RSIConfig;
  kdj: KDJConfig;
}

export type IndicatorType = keyof IndicatorConfig;

// ========== 默认值 ==========

export const DEFAULT_INDICATORS: IndicatorConfig = {
  ma:   { enabled: true, periods: [5, 10, 20] },
  ema:  { enabled: false, periods: [12, 26] },
  boll: { enabled: false, period: 20, multiplier: 2.0 },
  macd: { enabled: true, fast: 12, slow: 26, signal: 9 },
  rsi:  { enabled: false, period: 14 },
  kdj:  { enabled: false, period: 9, k: 3, d: 3 },
};

// ========== Context ==========

interface IndicatorContextType {
  config: IndicatorConfig;
  updateIndicator: <T extends IndicatorType>(type: T, partial: Partial<IndicatorConfig[T]>) => void;
  resetIndicator: (type: IndicatorType) => void;
}

const IndicatorContext = createContext<IndicatorContextType | undefined>(undefined);

export const IndicatorProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const [config, setConfig] = useState<IndicatorConfig>(DEFAULT_INDICATORS);

  // 从后端加载已保存的指标配置
  useEffect(() => {
    getConfig().then((appConfig) => {
      const saved = (appConfig as any).indicators as Partial<IndicatorConfig> | undefined;
      if (saved) {
        setConfig(prev => ({
          ma:   { ...prev.ma, ...saved.ma, periods: saved.ma?.periods ?? prev.ma.periods },
          ema:  { ...prev.ema, ...saved.ema, periods: saved.ema?.periods ?? prev.ema.periods },
          boll: { ...prev.boll, ...saved.boll },
          macd: { ...prev.macd, ...saved.macd },
          rsi:  { ...prev.rsi, ...saved.rsi },
          kdj:  { ...prev.kdj, ...saved.kdj },
        }));
      }
    }).catch(() => {});
  }, []);

  const updateIndicator = useCallback(<T extends IndicatorType>(
    type: T,
    partial: Partial<IndicatorConfig[T]>,
  ) => {
    setConfig(prev => ({
      ...prev,
      [type]: { ...prev[type], ...partial },
    }));
  }, []);

  const resetIndicator = useCallback((type: IndicatorType) => {
    setConfig(prev => ({
      ...prev,
      [type]: DEFAULT_INDICATORS[type],
    }));
  }, []);

  return (
    <IndicatorContext.Provider value={{ config, updateIndicator, resetIndicator }}>
      {children}
    </IndicatorContext.Provider>
  );
};

export const useIndicator = () => {
  const context = useContext(IndicatorContext);
  if (!context) throw new Error('useIndicator must be used within IndicatorProvider');
  return context;
};
