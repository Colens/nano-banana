type LogLevel = 'log' | 'info' | 'warn' | 'error' | 'debug';

type FrontendLogEntry = {
  level: string;
  message: string;
  context?: string;
};

let initialized = false;

function isTauri() {
  return typeof window !== 'undefined' && Boolean((window as any).__TAURI_INTERNALS__);
}

function safeStringify(value: unknown) {
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function formatArgs(args: unknown[]) {
  return args
    .map((arg) => {
      if (arg instanceof Error) {
        return `${arg.name}: ${arg.message}${arg.stack ? `\n${arg.stack}` : ''}`;
      }
      if (typeof arg === 'string') return arg;
      return safeStringify(arg);
    })
    .join(' ');
}

function shouldLogVerbose(level: LogLevel) {
  if (level === 'warn' || level === 'error') return true;
  try {
    return localStorage.getItem('diagnostic.verbose') === '1';
  } catch {
    return false;
  }
}

export function setDiagnosticVerbose(enabled: boolean) {
  try {
    localStorage.setItem('diagnostic.verbose', enabled ? '1' : '0');
  } catch {}
}

export function getDiagnosticVerbose() {
  try {
    return localStorage.getItem('diagnostic.verbose') === '1';
  } catch {
    return false;
  }
}

export function initDiagnosticLogger() {
  if (initialized) return;
  initialized = true;

  if (!isTauri()) return;

  const original = {
    log: console.log.bind(console),
    info: console.info.bind(console),
    warn: console.warn.bind(console),
    error: console.error.bind(console),
    debug: console.debug ? console.debug.bind(console) : console.log.bind(console),
  };

  let internal = false;
  const queue: FrontendLogEntry[] = [];
  let flushTimer: number | null = null;

  const enqueue = (entry: FrontendLogEntry) => {
    if (internal) return;
    queue.push(entry);
    if (queue.length > 2000) {
      queue.splice(0, queue.length - 2000);
    }
    if (flushTimer !== null) return;
    flushTimer = window.setTimeout(flush, 500);
  };

  const flush = async () => {
    flushTimer = null;
    if (queue.length === 0) return;

    const batch = queue.splice(0, 200);
    internal = true;
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      await invoke('write_frontend_logs', { entries: batch });
    } catch {
      // 静默失败：避免因为日志链路问题影响主流程
    } finally {
      internal = false;
    }

    if (queue.length > 0) {
      flushTimer = window.setTimeout(flush, 500);
    }
  };

  const wrap = (level: LogLevel) => {
    const fn = original[level] ?? original.log;
    return (...args: unknown[]) => {
      try {
        if (shouldLogVerbose(level)) {
          enqueue({ level, message: formatArgs(args) });
        }
      } catch {}
      fn(...(args as any[]));
    };
  };

  console.log = wrap('log');
  console.info = wrap('info');
  console.warn = wrap('warn');
  console.error = wrap('error');
  console.debug = wrap('debug');

  window.addEventListener('error', (event) => {
    try {
      const err = event.error instanceof Error ? event.error : null;
      const msg = err
        ? `${err.name}: ${err.message}${err.stack ? `\n${err.stack}` : ''}`
        : String(event.message || 'Unknown error');
      enqueue({
        level: 'error',
        message: msg,
        context: safeStringify({
          filename: event.filename,
          lineno: event.lineno,
          colno: event.colno,
        }),
      });
    } catch {}
  });

  window.addEventListener('unhandledrejection', (event) => {
    try {
      const reason = (event as PromiseRejectionEvent).reason;
      const msg =
        reason instanceof Error
          ? `${reason.name}: ${reason.message}${reason.stack ? `\n${reason.stack}` : ''}`
          : safeStringify(reason);
      enqueue({ level: 'error', message: `UnhandledRejection: ${msg}` });
    } catch {}
  });

  enqueue({
    level: 'info',
    message: 'diagnostic logger initialized',
    context: safeStringify({ ua: navigator.userAgent }),
  });
}

