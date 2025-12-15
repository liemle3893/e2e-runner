/**
 * E2E Test Runner - Logger Utility
 */

import type { Logger } from '../types';

export type LogLevel = 'debug' | 'info' | 'warn' | 'error' | 'silent';

export interface LoggerOptions {
  level: LogLevel;
  prefix?: string;
  useColors?: boolean;
  timestamp?: boolean;
}

const LOG_LEVELS: Record<LogLevel, number> = {
  debug: 0,
  info: 1,
  warn: 2,
  error: 3,
  silent: 4,
};

// ANSI color codes
const COLORS = {
  reset: '\x1b[0m',
  dim: '\x1b[2m',
  red: '\x1b[31m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  magenta: '\x1b[35m',
  cyan: '\x1b[36m',
  gray: '\x1b[90m',
};

/**
 * Create a logger instance
 */
export function createLogger(options: Partial<LoggerOptions> = {}): Logger {
  const config: LoggerOptions = {
    level: options.level ?? 'info',
    prefix: options.prefix ?? '',
    useColors: options.useColors ?? process.stdout.isTTY,
    timestamp: options.timestamp ?? false,
  };

  const shouldLog = (level: LogLevel): boolean => {
    return LOG_LEVELS[level] >= LOG_LEVELS[config.level];
  };

  const formatTimestamp = (): string => {
    if (!config.timestamp) return '';
    const now = new Date();
    return `[${now.toISOString().substring(11, 23)}] `;
  };

  const formatPrefix = (): string => {
    return config.prefix ? `[${config.prefix}] ` : '';
  };

  const colorize = (text: string, color: keyof typeof COLORS): string => {
    if (!config.useColors) return text;
    return `${COLORS[color]}${text}${COLORS.reset}`;
  };

  const formatMessage = (
    level: LogLevel,
    message: string,
    args: unknown[]
  ): string => {
    const timestamp = formatTimestamp();
    const prefix = formatPrefix();
    const levelTag = getLevelTag(level);

    let formattedMessage = `${timestamp}${prefix}${levelTag} ${message}`;

    // Format additional arguments
    if (args.length > 0) {
      const argsStr = args
        .map((arg) => {
          if (typeof arg === 'object') {
            try {
              return JSON.stringify(arg, null, 2);
            } catch {
              return String(arg);
            }
          }
          return String(arg);
        })
        .join(' ');
      formattedMessage += ` ${argsStr}`;
    }

    return formattedMessage;
  };

  const getLevelTag = (level: LogLevel): string => {
    switch (level) {
      case 'debug':
        return colorize('DEBUG', 'gray');
      case 'info':
        return colorize('INFO ', 'blue');
      case 'warn':
        return colorize('WARN ', 'yellow');
      case 'error':
        return colorize('ERROR', 'red');
      default:
        return level.toUpperCase().padEnd(5);
    }
  };

  return {
    debug(message: string, ...args: unknown[]): void {
      if (shouldLog('debug')) {
        console.log(formatMessage('debug', message, args));
      }
    },

    info(message: string, ...args: unknown[]): void {
      if (shouldLog('info')) {
        console.log(formatMessage('info', message, args));
      }
    },

    warn(message: string, ...args: unknown[]): void {
      if (shouldLog('warn')) {
        console.warn(formatMessage('warn', message, args));
      }
    },

    error(message: string, ...args: unknown[]): void {
      if (shouldLog('error')) {
        console.error(formatMessage('error', message, args));
      }
    },
  };
}

/**
 * Create a child logger with a prefix
 */
export function createChildLogger(parent: Logger, prefix: string): Logger {
  return {
    debug: (message, ...args) => parent.debug(`[${prefix}] ${message}`, ...args),
    info: (message, ...args) => parent.info(`[${prefix}] ${message}`, ...args),
    warn: (message, ...args) => parent.warn(`[${prefix}] ${message}`, ...args),
    error: (message, ...args) => parent.error(`[${prefix}] ${message}`, ...args),
  };
}

/**
 * Create a silent logger (no output)
 */
export function createSilentLogger(): Logger {
  const noop = () => {};
  return {
    debug: noop,
    info: noop,
    warn: noop,
    error: noop,
  };
}

/**
 * Default logger instance
 */
export const defaultLogger = createLogger();
