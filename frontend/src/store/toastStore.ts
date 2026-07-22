import { create } from 'zustand';

export type ToastType = 'success' | 'error' | 'info' | 'warning';

interface Toast {
  id: string;
  message: string;
  type: ToastType;
}

// 存储定时器 ID，用于清理
const toastTimers = new Map<string, ReturnType<typeof setTimeout>>();

interface ToastState {
  toasts: Toast[];
  addToast: (message: string, type?: ToastType) => void;
  removeToast: (id: string) => void;
  clearAll: () => void; // 新增：清空所有 toast
}

export const useToastStore = create<ToastState>((set) => ({
  toasts: [],
  addToast: (message, type = 'info') => {
    const id = Math.random().toString(36).substring(2, 9);
    set((state) => ({
      toasts: [...state.toasts, { id, message, type }],
    }));

    // 2秒后自动移除
    const timer = setTimeout(() => {
      set((state) => ({
        toasts: state.toasts.filter((t) => t.id !== id),
      }));
      toastTimers.delete(id);
    }, 2000);

    // 保存定时器 ID
    toastTimers.set(id, timer);
  },
  removeToast: (id) => {
    // 清除定时器（如果存在）
    const timer = toastTimers.get(id);
    if (timer) {
      clearTimeout(timer);
      toastTimers.delete(id);
    }
    set((state) => ({
      toasts: state.toasts.filter((t) => t.id !== id),
    }));
  },
  // 新增：清空所有 toast 和定时器
  clearAll: () => {
    // 清除所有定时器
    toastTimers.forEach((timer) => clearTimeout(timer));
    toastTimers.clear();
    // 清空 toast 列表
    set({ toasts: [] });
  },
}));

// 导出便捷调用方法
export const toast = {
  success: (msg: string) => useToastStore.getState().addToast(msg, 'success'),
  error: (msg: string) => useToastStore.getState().addToast(msg, 'error'),
  info: (msg: string) => useToastStore.getState().addToast(msg, 'info'),
  warning: (msg: string) => useToastStore.getState().addToast(msg, 'warning'),
};

// 页面卸载时清理所有定时器
if (typeof window !== 'undefined') {
  window.addEventListener('beforeunload', () => {
    toastTimers.forEach((timer) => clearTimeout(timer));
    toastTimers.clear();
  });
}
