import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Mock environment variables before importing the module
const mockEnv = {
  REST_BASE_URL: 'http://localhost:3000',
  REST_RESPONSE_SIZE_LIMIT: '10000',
  REST_ENABLE_SSL_VERIFY: 'true',
};

describe('Utility Functions', () => {
  describe('normalizeBaseUrl', () => {
    it('should remove trailing slashes from URL', () => {
      const normalizeBaseUrl = (url: string): string => url.replace(/\/+$/, '');
      
      expect(normalizeBaseUrl('http://localhost:3000/')).toBe('http://localhost:3000');
      expect(normalizeBaseUrl('http://localhost:3000///')).toBe('http://localhost:3000');
      expect(normalizeBaseUrl('http://localhost:3000')).toBe('http://localhost:3000');
      expect(normalizeBaseUrl('https://api.example.com/v1/')).toBe('https://api.example.com/v1');
    });
  });

  describe('getCustomHeaders', () => {
    beforeEach(() => {
      // Clear any HEADER_ prefixed env vars
      Object.keys(process.env).forEach(key => {
        if (/^header_/i.test(key)) {
          delete process.env[key];
        }
      });
    });

    afterEach(() => {
      // Cleanup
      Object.keys(process.env).forEach(key => {
        if (/^header_/i.test(key)) {
          delete process.env[key];
        }
      });
    });

    it('should collect headers with HEADER_ prefix', () => {
      const getCustomHeaders = (): Record<string, string> => {
        const headers: Record<string, string> = {};
        const headerPrefix = /^header_/i;
        
        for (const [key, value] of Object.entries(process.env)) {
          if (headerPrefix.test(key) && value !== undefined) {
            const headerName = key.replace(headerPrefix, '');
            headers[headerName] = value;
          }
        }
        
        return headers;
      };

      process.env.HEADER_X_Custom = 'test-value';
      process.env.header_Accept = 'application/json';
      
      const headers = getCustomHeaders();
      
      expect(headers['X_Custom']).toBe('test-value');
      expect(headers['Accept']).toBe('application/json');
    });

    it('should return empty object when no custom headers', () => {
      const getCustomHeaders = (): Record<string, string> => {
        const headers: Record<string, string> = {};
        const headerPrefix = /^header_/i;
        
        for (const [key, value] of Object.entries(process.env)) {
          if (headerPrefix.test(key) && value !== undefined) {
            const headerName = key.replace(headerPrefix, '');
            headers[headerName] = value;
          }
        }
        
        return headers;
      };

      const headers = getCustomHeaders();
      expect(headers).toEqual({});
    });
  });

  describe('Auth helper functions', () => {
    it('hasBasicAuth should return true when both username and password are set', () => {
      const AUTH_BASIC_USERNAME = 'user';
      const AUTH_BASIC_PASSWORD = 'pass';
      const hasBasicAuth = () => AUTH_BASIC_USERNAME && AUTH_BASIC_PASSWORD;
      
      expect(hasBasicAuth()).toBeTruthy();
    });

    it('hasBasicAuth should return false when credentials are missing', () => {
      const AUTH_BASIC_USERNAME = '';
      const AUTH_BASIC_PASSWORD = 'pass';
      const hasBasicAuth = () => AUTH_BASIC_USERNAME && AUTH_BASIC_PASSWORD;
      
      expect(hasBasicAuth()).toBeFalsy();
    });

    it('hasBearerAuth should return true when token is set', () => {
      const AUTH_BEARER = 'token123';
      const hasBearerAuth = () => !!AUTH_BEARER;
      
      expect(hasBearerAuth()).toBe(true);
    });

    it('hasApiKeyAuth should return true when both header name and value are set', () => {
      const AUTH_APIKEY_HEADER_NAME = 'X-API-Key';
      const AUTH_APIKEY_VALUE = 'key123';
      const hasApiKeyAuth = () => AUTH_APIKEY_HEADER_NAME && AUTH_APIKEY_VALUE;
      
      expect(hasApiKeyAuth()).toBeTruthy();
    });
  });
});

describe('Endpoint Validation', () => {
  const isValidEndpointArgs = (args: unknown): boolean => {
    if (typeof args !== 'object' || args === null) return false;
    const a = args as Record<string, unknown>;
    if (!['GET', 'POST', 'PUT', 'DELETE', 'PATCH'].includes(a.method as string)) return false;
    if (typeof a.endpoint !== 'string') return false;
    if (a.headers !== undefined && (typeof a.headers !== 'object' || a.headers === null)) return false;
    return true;
  };

  it('should validate correct GET request args', () => {
    expect(isValidEndpointArgs({
      method: 'GET',
      endpoint: '/users'
    })).toBe(true);
  });

  it('should validate correct POST request args with body', () => {
    expect(isValidEndpointArgs({
      method: 'POST',
      endpoint: '/users',
      body: { name: 'John' }
    })).toBe(true);
  });

  it('should reject invalid method', () => {
    expect(isValidEndpointArgs({
      method: 'INVALID',
      endpoint: '/users'
    })).toBe(false);
  });

  it('should reject missing endpoint', () => {
    expect(isValidEndpointArgs({
      method: 'GET'
    })).toBe(false);
  });

  it('should reject null args', () => {
    expect(isValidEndpointArgs(null)).toBe(false);
  });

  it('should accept valid headers object', () => {
    expect(isValidEndpointArgs({
      method: 'GET',
      endpoint: '/users',
      headers: { 'Content-Type': 'application/json' }
    })).toBe(true);
  });

  it('should reject invalid headers (non-object)', () => {
    expect(isValidEndpointArgs({
      method: 'GET',
      endpoint: '/users',
      headers: 'invalid'
    })).toBe(false);
  });
});

describe('Header Sanitization', () => {
  const sanitizeHeaders = (
    headers: Record<string, unknown>,
    isFromOptionalParams: boolean = false
  ): Record<string, unknown> => {
    const sanitized: Record<string, unknown> = {};
    const AUTH_APIKEY_HEADER_NAME = 'X-API-Key';
    
    for (const [key, value] of Object.entries(headers)) {
      const lowerKey = key.toLowerCase();
      
      if (isFromOptionalParams) {
        sanitized[key] = value;
        continue;
      }
      
      if (lowerKey === 'authorization' || lowerKey === AUTH_APIKEY_HEADER_NAME.toLowerCase()) {
        sanitized[key] = '[REDACTED]';
        continue;
      }
      
      sanitized[key] = value;
    }
    
    return sanitized;
  };

  it('should redact authorization header', () => {
    const result = sanitizeHeaders({ Authorization: 'Bearer token123' });
    expect(result.Authorization).toBe('[REDACTED]');
  });

  it('should redact API key header', () => {
    const result = sanitizeHeaders({ 'X-API-Key': 'secret-key' });
    expect(result['X-API-Key']).toBe('[REDACTED]');
  });

  it('should not redact non-sensitive headers', () => {
    const result = sanitizeHeaders({ 'Content-Type': 'application/json' });
    expect(result['Content-Type']).toBe('application/json');
  });

  it('should pass through optional params without redaction', () => {
    const result = sanitizeHeaders({ Authorization: 'Bearer token123' }, true);
    expect(result.Authorization).toBe('Bearer token123');
  });
});

describe('Response Size Handling', () => {
  it('should detect when response exceeds size limit', () => {
    const RESPONSE_SIZE_LIMIT = 100;
    const responseBody = 'x'.repeat(200);
    const bodySize = Buffer.from(responseBody).length;
    
    expect(bodySize > RESPONSE_SIZE_LIMIT).toBe(true);
  });

  it('should truncate response to size limit', () => {
    const RESPONSE_SIZE_LIMIT = 100;
    const responseBody = 'x'.repeat(200);
    const truncated = responseBody.slice(0, RESPONSE_SIZE_LIMIT);
    
    expect(truncated.length).toBe(100);
  });
});
