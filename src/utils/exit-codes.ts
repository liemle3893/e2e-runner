/**
 * E2E Test Runner - Exit Codes
 *
 * Standard exit codes for CLI operations
 */

export const EXIT_CODES = {
  /** All tests passed */
  SUCCESS: 0,

  /** One or more tests failed */
  TEST_FAILURE: 1,

  /** Configuration file error (missing, invalid, or parse error) */
  CONFIG_ERROR: 2,

  /** Adapter connection failed */
  CONNECTION_ERROR: 3,

  /** Test file validation error */
  VALIDATION_ERROR: 4,

  /** Test or operation timed out */
  TIMEOUT: 5,

  /** Command not found or fatal error */
  FATAL: 127,
} as const;

export type ExitCode = (typeof EXIT_CODES)[keyof typeof EXIT_CODES];

/**
 * Get exit code description
 */
export function getExitCodeDescription(code: ExitCode): string {
  switch (code) {
    case EXIT_CODES.SUCCESS:
      return 'All tests passed';
    case EXIT_CODES.TEST_FAILURE:
      return 'One or more tests failed';
    case EXIT_CODES.CONFIG_ERROR:
      return 'Configuration error';
    case EXIT_CODES.CONNECTION_ERROR:
      return 'Adapter connection failed';
    case EXIT_CODES.VALIDATION_ERROR:
      return 'Test file validation error';
    case EXIT_CODES.TIMEOUT:
      return 'Operation timed out';
    case EXIT_CODES.FATAL:
      return 'Fatal error';
    default:
      return 'Unknown error';
  }
}

/**
 * Map error code string to exit code
 */
export function errorCodeToExitCode(errorCode: string): ExitCode {
  switch (errorCode) {
    case 'CONFIG_ERROR':
      return EXIT_CODES.CONFIG_ERROR;
    case 'VALIDATION_ERROR':
      return EXIT_CODES.VALIDATION_ERROR;
    case 'CONNECTION_ERROR':
      return EXIT_CODES.CONNECTION_ERROR;
    case 'TIMEOUT_ERROR':
      return EXIT_CODES.TIMEOUT;
    case 'EXECUTION_ERROR':
    case 'ASSERTION_ERROR':
    case 'ADAPTER_ERROR':
      return EXIT_CODES.TEST_FAILURE;
    default:
      return EXIT_CODES.FATAL;
  }
}
