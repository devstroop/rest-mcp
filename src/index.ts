#!/usr/bin/env node
import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import {
  CallToolRequestSchema,
  ErrorCode,
  ListToolsRequestSchema,
  ListResourcesRequestSchema,
  ReadResourceRequestSchema,
  McpError,
} from '@modelcontextprotocol/sdk/types.js';
import axios, { AxiosInstance, AxiosRequestConfig, Method } from 'axios';
import { VERSION, SERVER_NAME } from './version.js';

if (!process.env.REST_BASE_URL) {
  throw new Error('REST_BASE_URL environment variable is required');
}

// Default response size limit: 10KB (10000 bytes)
const RESPONSE_SIZE_LIMIT = process.env.REST_RESPONSE_SIZE_LIMIT 
  ? parseInt(process.env.REST_RESPONSE_SIZE_LIMIT, 10)
  : 10000;

if (isNaN(RESPONSE_SIZE_LIMIT) || RESPONSE_SIZE_LIMIT <= 0) {
  throw new Error('REST_RESPONSE_SIZE_LIMIT must be a positive number');
}

// Default request timeout: 30 seconds
const REST_TIMEOUT = process.env.REST_TIMEOUT
  ? parseInt(process.env.REST_TIMEOUT, 10)
  : 30000;

if (isNaN(REST_TIMEOUT) || REST_TIMEOUT <= 0) {
  throw new Error('REST_TIMEOUT must be a positive number');
}

const AUTH_BASIC_USERNAME = process.env.AUTH_BASIC_USERNAME;
const AUTH_BASIC_PASSWORD = process.env.AUTH_BASIC_PASSWORD;
const AUTH_BEARER = process.env.AUTH_BEARER;
const AUTH_APIKEY_HEADER_NAME = process.env.AUTH_APIKEY_HEADER_NAME;
const AUTH_APIKEY_VALUE = process.env.AUTH_APIKEY_VALUE;
const REST_ENABLE_SSL_VERIFY = process.env.REST_ENABLE_SSL_VERIFY !== 'false';

// Type for request/response body (JSON-serializable values)
type JsonValue = string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue };
type RequestBody = JsonValue | undefined;

// Headers that are safe to display values for (non-sensitive)
const SAFE_HEADERS = new Set([
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

interface EndpointArgs {
  method: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH';
  endpoint: string;
  body?: RequestBody;
  headers?: Record<string, string>;
  host?: string;
}

interface ValidationResult {
  isError: boolean;
  messages: string[];
  truncated?: {
    originalSize: number;
    returnedSize: number;
    truncationPoint: number;
    sizeLimit: number;
  };
}

// Function to sanitize headers by removing sensitive values and non-approved headers
const sanitizeHeaders = (
  headers: Record<string, string | undefined>,
  isFromOptionalParams: boolean = false
): Record<string, string> => {
  const sanitized: Record<string, string> = {};
  
  for (const [key, value] of Object.entries(headers)) {
    if (value === undefined) continue;
    
    const lowerKey = key.toLowerCase();
    
    // Always redact auth headers, even from optional parameters
    if (
      lowerKey === 'authorization' ||
      (AUTH_APIKEY_HEADER_NAME && lowerKey === AUTH_APIKEY_HEADER_NAME.toLowerCase())
    ) {
      sanitized[key] = '[REDACTED]';
      continue;
    }
    
    // Include headers from optional parameters (non-auth)
    if (isFromOptionalParams) {
      sanitized[key] = value;
      continue;
    }
    
    // For headers from config/env
    const customHeaders = getCustomHeaders();
    if (key in customHeaders) {
      // Show value only for headers that are in the approved list
      sanitized[key] = SAFE_HEADERS.has(lowerKey) ? value : '[REDACTED]';
    }
  }
  
  return sanitized;
};

interface ResponseObject {
  request: {
    url: string;
    method: string;
    headers: Record<string, string>;
    body: RequestBody;
    authMethod: string;
  };
  response: {
    statusCode: number;
    statusText: string;
    timing: string;
    headers: Record<string, string>;
    body: JsonValue;
  };
  validation: ValidationResult;
}

const normalizeBaseUrl = (url: string): string => url.replace(/\/+$/, '');

const isValidEndpointArgs = (args: unknown): args is EndpointArgs => {
  if (typeof args !== 'object' || args === null) return false;
  const a = args as Record<string, unknown>;
  if (!['GET', 'POST', 'PUT', 'DELETE', 'PATCH'].includes(a.method as string)) return false;
  if (typeof a.endpoint !== 'string') return false;
  if (a.headers !== undefined && (typeof a.headers !== 'object' || a.headers === null)) return false;
  
  // Check if endpoint contains a full URL
  const urlPattern = /^(https?:\/\/|www\.)/i;
  if (urlPattern.test(a.endpoint)) {
    throw new McpError(
      ErrorCode.InvalidParams,
      `Invalid endpoint format. Do not include full URLs. Instead of "${a.endpoint}", use just the path (e.g. "/api/users"). ` +
      `Your path will be resolved to: ${process.env.REST_BASE_URL}${(a.endpoint).replace(/^\/+|\/+$/g, '')}. ` +
      `To test a different base URL, update the REST_BASE_URL environment variable.`
    );
  }
  // Validate .host if present
  if (a.host !== undefined) {
    try {
      const url = new URL(a.host as string);
      if (!/^https?:$/.test(url.protocol)) {
        throw new Error();
      }
      // Remove trailing slash if present
      const normalizedHost = url.pathname.endsWith('/') && url.pathname !== '/'
        ? url.origin + url.pathname.replace(/\/+$/, '')
        : url.origin + url.pathname;
      (args as { host: string }).host = normalizedHost;
    } catch (e) {
      throw new McpError(ErrorCode.InvalidParams, `Invalid host format. The 'host' argument must be a valid URL starting with http:// or https://, e.g. "https://example.com" or "http://localhost:3001/api/v1". Received: "${a.host}"`);
    }
  }
  
  return true;
};

const hasBasicAuth = () => AUTH_BASIC_USERNAME && AUTH_BASIC_PASSWORD;
const hasBearerAuth = () => !!AUTH_BEARER;
const hasApiKeyAuth = () => AUTH_APIKEY_HEADER_NAME && AUTH_APIKEY_VALUE;

/**
 * Generate the tool description for test_request
 */
function getToolDescription(): string {
  const baseUrl = normalizeBaseUrl(process.env.REST_BASE_URL!);
  const sslStatus = REST_ENABLE_SSL_VERIFY ? 'enabled' : 'disabled';
  
  // Authentication status
  let authStatus: string;
  if (hasBasicAuth()) {
    authStatus = `Basic Auth with username: ${AUTH_BASIC_USERNAME}`;
  } else if (hasBearerAuth()) {
    authStatus = 'Bearer token authentication configured';
  } else if (hasApiKeyAuth()) {
    authStatus = `API Key using header: ${AUTH_APIKEY_HEADER_NAME}`;
  } else {
    authStatus = 'No authentication configured';
  }
  
  // Custom headers status
  const customHeaders = getCustomHeaders();
  let headersStatus: string;
  if (Object.keys(customHeaders).length === 0) {
    headersStatus = 'No custom headers defined (see config resource for headers)';
  } else {
    const headerList = Object.entries(customHeaders)
      .map(([name, value]) => {
        const lowerName = name.toLowerCase();
        return SAFE_HEADERS.has(lowerName) ? `${name}(${value})` : name;
      })
      .join(', ');
    headersStatus = `Custom headers defined: ${headerList} (see config resource for headers)`;
  }
  
  return [
    `Test a REST API endpoint and get detailed response information.`,
    `Base URL: ${baseUrl}`,
    `SSL Verification ${sslStatus} (see config resource for SSL settings)`,
    `Authentication: ${authStatus}`,
    headersStatus,
    `The tool automatically:`,
    `- Normalizes endpoints (adds leading slash, removes trailing slashes)`,
    `- Handles authentication header injection`,
    `- Applies custom headers from HEADER_* environment variables`,
    `- Accepts any HTTP status code as valid`,
    `- Limits response size to ${RESPONSE_SIZE_LIMIT} bytes (see config resource for size limit settings)`,
    `- Request timeout: ${REST_TIMEOUT}ms`,
    `- Returns detailed response information including: Full URL, status, headers, body, timing, validation`,
    `Error Handling: Network errors are caught and returned with descriptive messages`,
    `See the config resource for all configuration options.`,
  ].join(' | ');
}

// Collect custom headers from environment variables
const getCustomHeaders = (): Record<string, string> => {
  const headers: Record<string, string> = {};
  const headerPrefix = /^header_/i;  // Case-insensitive match for 'header_'
  
  for (const [key, value] of Object.entries(process.env)) {
    if (headerPrefix.test(key) && value !== undefined) {
      // Extract header name after the prefix, preserving case
      const headerName = key.replace(headerPrefix, '');
      headers[headerName] = value;
    }
  }
  
  return headers;
};

class RestTester {
  private server!: Server;
  private axiosInstance!: AxiosInstance;

  private async setupServer() {
    this.server = new Server(
      {
        name: SERVER_NAME,
        version: VERSION,
      },
      {
        capabilities: {
          tools: {},
          resources: {},
        },
      }
    );

    const https = await import('https').catch(() => {
      throw new Error('Failed to import https module');
    });
    this.axiosInstance = axios.create({
      baseURL: normalizeBaseUrl(process.env.REST_BASE_URL!),
      timeout: REST_TIMEOUT,
      validateStatus: () => true, // Allow any status code
      httpsAgent: REST_ENABLE_SSL_VERIFY ? undefined : new https.Agent({ // Disable SSL verification only when explicitly set to false
        rejectUnauthorized: false
      })
    });

    this.setupToolHandlers();
    this.setupResourceHandlers();
    
    this.server.onerror = (error) => console.error('[MCP Error]', error);
    process.on('SIGINT', async () => {
      await this.server.close();
      process.exit(0);
    });
  }

  private setupResourceHandlers() {
    this.server.setRequestHandler(ListResourcesRequestSchema, async () => ({
      resources: [
        {
          uri: `${SERVER_NAME}://examples`,
          name: 'REST API Usage Examples',
          description: 'Detailed examples of using the REST API testing tool',
          mimeType: 'text/markdown'
        },
        {
          uri: `${SERVER_NAME}://response-format`,
          name: 'Response Format Documentation',
          description: 'Documentation of the response format and structure',
          mimeType: 'text/markdown'
        },
        {
          uri: `${SERVER_NAME}://config`,
          name: 'Configuration Documentation',
          description: 'Documentation of all configuration options and how to use them',
          mimeType: 'text/markdown'
        }
      ]
    }));

    this.server.setRequestHandler(ReadResourceRequestSchema, async (request) => {
      const uriPattern = new RegExp(`^${SERVER_NAME}://(.+)$`);
      const match = request.params.uri.match(uriPattern);
      
      if (!match) {
        throw new McpError(
          ErrorCode.InvalidRequest,
          `Invalid resource URI format: ${request.params.uri}`
        );
      }

      const resource = match[1];

      try {
        const [fs, path, url] = await Promise.all([
          import('fs'),
          import('path'),
          import('url'),
        ]);
        
        const __filename = url.fileURLToPath(import.meta.url);
        const __dirname = path.dirname(__filename);
        
        // In the built app, resources are in build/resources
        // In development, they're in src/resources
        const resourcePath = path.join(__dirname, 'resources', `${resource}.md`);
        const content = await fs.promises.readFile(resourcePath, 'utf8');

        return {
          contents: [{
            uri: request.params.uri,
            mimeType: 'text/markdown',
            text: content
          }]
        };
      } catch (error) {
        throw new McpError(
          ErrorCode.InvalidRequest,
          `Resource not found: ${resource}`
        );
      }
    });
  }

  private setupToolHandlers() {
    this.server.setRequestHandler(ListToolsRequestSchema, async () => ({
      tools: [
        {
          name: 'test_request',
          description: getToolDescription(),
          inputSchema: {
            type: 'object',
            properties: {
              method: {
                type: 'string',
                enum: ['GET', 'POST', 'PUT', 'DELETE', 'PATCH'],
                description: 'HTTP method to use',
              },
              endpoint: {
                type: 'string',
                description: `Endpoint path (e.g. "/users"). Do not include full URLs - only the path. Example: "/api/users" will resolve to "${normalizeBaseUrl(process.env.REST_BASE_URL!)}/api/users"`,
              },
              body: {
                type: 'object',
                description: 'Optional request body for POST/PUT requests',
              },
              headers: {
                type: 'object',
                description: 'Optional request headers for one-time use. IMPORTANT: Do not use for sensitive data like API keys - those should be configured via environment variables. This parameter is intended for dynamic, non-sensitive headers that may be needed for specific requests.',
                additionalProperties: {
                  type: 'string'
                }
              },
              host: {
                type: 'string',
                description: `Optional override for the base URL. Must be a valid URL starting with http:// or https://. Example: "https://api.example.com/v1" with endpoint "/users" resolves to "https://api.example.com/v1/users"`,
              }
            },
            required: ['method', 'endpoint'],
          },
        },
      ],
    }));

    this.server.setRequestHandler(CallToolRequestSchema, async (request) => {
      if (request.params.name !== 'test_request') {
        throw new McpError(
          ErrorCode.MethodNotFound,
          `Unknown tool: ${request.params.name}`
        );
      }

      if (!isValidEndpointArgs(request.params.arguments)) {
        throw new McpError(
          ErrorCode.InvalidParams,
          'Invalid test endpoint arguments'
        );
      }

      // Ensure endpoint starts with / and remove any trailing slashes
      const normalizedEndpoint = `/${request.params.arguments.endpoint.replace(/^\/+|\/+$/g, '')}`;
      
      const fullUrl = `${request.params.arguments.host || process.env.REST_BASE_URL}${normalizedEndpoint}`;
      // Initialize request config
      const config: AxiosRequestConfig = {
          method: request.params.arguments.method as Method,
          url: fullUrl,
          headers: {},
        };

      // Add request body for POST/PUT/PATCH
      if (['POST', 'PUT', 'PATCH'].includes(request.params.arguments.method) && request.params.arguments.body) {
        config.data = request.params.arguments.body;
      }

      // Apply headers in order of priority (lowest to highest)
      
      // 1. Custom global headers (lowest priority)
      const customHeaders = getCustomHeaders();
      config.headers = {
        ...customHeaders,
        ...config.headers,
        ...(request.params.arguments.headers || {}) // Request-specific headers (middle priority)
      };

      // 3. Authentication headers (highest priority)
      if (hasBasicAuth()) {
        const base64Credentials = Buffer.from(`${AUTH_BASIC_USERNAME}:${AUTH_BASIC_PASSWORD}`).toString('base64');
        config.headers = {
          ...config.headers,
          'Authorization': `Basic ${base64Credentials}`
        };
      } else if (hasBearerAuth()) {
        config.headers = {
          ...config.headers,
          'Authorization': `Bearer ${AUTH_BEARER}`
        };
      } else if (hasApiKeyAuth()) {
        config.headers = {
          ...config.headers,
          [AUTH_APIKEY_HEADER_NAME as string]: AUTH_APIKEY_VALUE
        };
      }

      try {
        const startTime = Date.now();
        const response = await this.axiosInstance.request(config);
        const endTime = Date.now();

        // Determine auth method used
        let authMethod = 'none';
        if (hasBasicAuth()) authMethod = 'basic';
        else if (hasBearerAuth()) authMethod = 'bearer';
        else if (hasApiKeyAuth()) authMethod = 'apikey';

        // Prepare response object
        const responseObj: ResponseObject = {
          request: {
            url: fullUrl,
            method: config.method || 'GET',
            headers: {
              ...sanitizeHeaders(config.headers as Record<string, string | undefined>, false),
              ...sanitizeHeaders(request.params.arguments.headers || {}, true)
            },
            body: config.data,
            authMethod
          },
          response: {
            statusCode: response.status,
            statusText: response.statusText,
            timing: `${endTime - startTime}ms`,
            headers: sanitizeHeaders(response.headers as Record<string, any>, false),
            body: response.data,
          },
          validation: {
            isError: response.status >= 400,
            messages: response.status >= 400 ? 
              [`Request failed with status ${response.status}`] : 
              ['Request completed successfully']
          }
        };

        // Check response body size independently
        const bodyStr = typeof response.data === 'string' 
          ? response.data 
          : JSON.stringify(response.data);
        const bodySize = Buffer.from(bodyStr).length;

        if (bodySize > RESPONSE_SIZE_LIMIT) {
          // Simply truncate to the size limit
          responseObj.response.body = bodyStr.slice(0, RESPONSE_SIZE_LIMIT);
          responseObj.validation.messages.push(
            `Response truncated: ${RESPONSE_SIZE_LIMIT} of ${bodySize} bytes returned due to size limit (${RESPONSE_SIZE_LIMIT} bytes)`
          );
          responseObj.validation.truncated = {
            originalSize: bodySize,
            returnedSize: RESPONSE_SIZE_LIMIT,
            truncationPoint: RESPONSE_SIZE_LIMIT,
            sizeLimit: RESPONSE_SIZE_LIMIT
          };
        }

        return {
          content: [
            {
              type: 'text',
              text: JSON.stringify(responseObj, null, 2),
            },
          ],
        };
      } catch (error) {
        if (axios.isAxiosError(error)) {
          return {
            content: [
              {
                type: 'text',
              text: JSON.stringify({
                error: {
                  message: error.message,
                  code: error.code,
                  request: {
                    url: `${process.env.REST_BASE_URL}${normalizedEndpoint}`,
                    method: config.method,
                    headers: {
                      ...sanitizeHeaders(config.headers as Record<string, string | undefined>, false),
                      ...sanitizeHeaders(request.params.arguments.headers || {}, true)
                    },
                    body: config.data
                  }
                }
              }, null, 2),
              },
            ],
            isError: true,
          };
        }
        throw error;
      }
    });
  }

  async run() {
    await this.setupServer();
    const transport = new StdioServerTransport();
    await this.server.connect(transport);
    console.error('REST API Tester MCP server running on stdio');
  }
}

const server = new RestTester();
server.run().catch(console.error);
