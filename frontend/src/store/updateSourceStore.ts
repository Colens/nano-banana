import { create } from 'zustand';

/**
 * 更新源管理 Store
 * 用于防止 WebSocket 和轮询之间的竞态条件
 */
interface UpdateSourceState {
  source: 'websocket' | 'polling' | null;
  setSource: (source: 'websocket' | 'polling' | null) => void;
  clearSource: () => void;
}

export const useUpdateSourceStore = create<UpdateSourceState>((set) => ({
  source: null,
  setSource: (source) => set({ source }),
  clearSource: () => set({ source: null }),
}));

// 便捷函数（兼容旧代码）
export const setUpdateSource = (source: 'websocket' | 'polling' | null) => {
  useUpdateSourceStore.getState().setSource(source);
};

export const getUpdateSource = () => {
  return useUpdateSourceStore.getState().source;
};

export const clearUpdateSource = () => {
  useUpdateSourceStore.getState().clearSource();
};
