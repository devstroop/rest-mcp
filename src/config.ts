/**
 * Configuration module for REST API Tester
 * Centralizes environment variable access and validation
 */

export interface Config {
  /** Base URL for all REST API requests */
  baseUrl: string;
  /** Maximum response size in bytes (default: 10000) */
  responseSizeLimit: number;
  /** Request timeout in milliseconds (default: 30000) */
  timeout: number;
  /** Whether to verify SSL certificates (default: true) */
  enableSslVerify: boolean;
  /** Basic auth username */
  basicUsername?: string;
  /** Basic auth password */
  basicPassword?: string;
  /** Bearer token for authentication */
  bearerToken?: string;
  /** API key header name */
  apiKeyHeaderName?: string;
  /** API key value */
  apiKeyValue?: string;
}

/**
 * List of headers that are safe to display values for (non-sensitive)
 */
export const SAFE_HEADERS = new Set([
  'accept',
  'accept-language',
  'content-type',
  'user-agent',
  'cache-control',
  'if-match',
  'if-none-match',
  'if-modified-since',
  'if-unmodified-since',
]);

/**
 * Parse and validate environment variables
 * @throws Error if required variables are missing or invalid
 */
export function loadConfig(): Config {
  const baseUrl = process.env.REST_BASE_URL;
  if (!baseUrl) {
    throw new Error('REST_BASE_URL environment variable is required');
  }

  const responseSizeLimit = process.env.REST_RESPONSE_SIZE_LIMIT
    ? parseInt(process.env.REST_RESPONSE_SIZE_LIMIT, 10)
    : 10000;

  if (isNaN(responseSizeLimit) || responseSizeLimit <= 0) {
    throw new Error('REST_RESPONSE_SIZE_LIMIT must be a positive number');
  }

  const timeout = process.env.REST_TIMEOUT
    ? parseInt(process.env.REST_TIMEOUT, 10)
    : 30000;

  if (isNaN(timeout) || timeout <= 0) {
    throw new Error('REST_TIMEOUT must be a positive number');
  }

  return {
    baseUrl,
    responseSizeLimit,
    timeout,
    enableSslVerify: process.env.REST_ENABLE_SSL_VERIFY !== 'false',
    basicUsername: process.env.AUTH_BASIC_USERNAME,
    basicPassword: process.env.AUTH_BASIC_PASSWORD,
    bearerToken: process.env.AUTH_BEARER,
    apiKeyHeaderName: process.env.AUTH_APIKEY_HEADER_NAME,
    apiKeyValue: process.env.AUTH_APIKEY_VALUE,
  };
}

/**
 * Collect custom headers from HEADER_* environment variables
 */
export function getCustomHeaders(): Record<string, string> {
  const headers: Record<string, string> = {};
  const headerPrefix = /^header_/i;

  for (const [key, value] of Object.entries(process.env)) {
    if (headerPrefix.test(key) && value !== undefined) {
      const headerName = key.replace(headerPrefix, '');
      headers[headerName] = value;
    }
  }

  return headers;
}

/**
 * Check if basic auth is configured
 */
export function hasBasicAuth(config: Config): boolean {
  return !!(config.basicUsername && config.basicPassword);
}

/**
 * Check if bearer auth is configured
 */
export function hasBearerAuth(config: Config): boolean {
  return !!config.bearerToken;
}

/**
 * Check if API key auth is configured
 */
export function hasApiKeyAuth(config: Config): boolean {
  return !!(config.apiKeyHeaderName && config.apiKeyValue);
}

/**
 * Get authentication method description for display
 */
export function getAuthDescription(config: Config): string {
  if (hasBasicAuth(config)) {
    return `Basic Auth with username: ${config.basicUsername}`;
  }
  if (hasBearerAuth(config)) {
    return 'Bearer token authentication configured';
  }
  if (hasApiKeyAuth(config)) {
    return `API Key using header: ${config.apiKeyHeaderName}`;
  }
  return 'No authentication configured';
}

/**
 * Normalize base URL by removing trailing slashes
 */
export function normalizeBaseUrl(url: string): string {
  return url.replace(/\/+$/, '');
}
