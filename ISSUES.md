# Project Issues & Improvement Suggestions

## Code Quality

### 1. Race Condition in Server Initialization ✅ VALIDATED
**Priority:** High  
**File:** `src/index.ts` (lines 181-182, 512-513)

The `setupServer` method is called both in the constructor and in `run()`, creating a potential race condition where server setup runs twice:

```typescript
constructor() {
  this.setupServer(); // Called here (line 182)
}

async run() {
  await this.setupServer(); // Called again here (line 512)
}
```

**Suggestion:** Remove the constructor call and rely only on the `run()` method for initialization.

---

### 2. Missing Type Safety for Environment Variables ✅ VALIDATED
**Priority:** Medium  
**File:** `src/index.ts` (lines 15-32)

Environment variables are accessed directly throughout the code without centralized validation or typing.

**Suggestion:** Create a `src/config.ts` module to:
- Centralize all environment variable access
- Validate required variables at startup
- Provide TypeScript types for configuration

---

### 3. Large Inline Tool Description ✅ VALIDATED
**Priority:** Low  
**File:** `src/index.ts` (lines 285-325)

The tool description is a massive inline string spanning ~40 lines with complex IIFE, making the code harder to read and maintain.

**Suggestion:** Extract to a separate constant or generate from a template.

---

### 4. Missing Error Handling for Dynamic Imports ✅ VALIDATED
**Priority:** Medium  
**File:** `src/index.ts` (lines 201, 253-254, 256-258)

Dynamic imports (`await import('https')`, `await import('fs')`, `await import('path')`, `await import('url')`) lack try-catch blocks.

**Suggestion:** Wrap dynamic imports in try-catch with meaningful error messages.

---

### 5. Missing Request Timeout Configuration ✅ VALIDATED
**Priority:** Medium  
**File:** `src/index.ts`

No timeout is configured for axios requests, which could cause hanging requests indefinitely.

**Suggestion:** Add a configurable `REST_TIMEOUT` environment variable (default: 30000ms).

---

### 6. Excessive Use of `any` Type ✅ NEW
**Priority:** Medium  
**File:** `src/index.ts` (lines 37, 55, 57, 58, 106, 113, 114, 121)

The codebase uses `any` type in 8+ places, reducing type safety:
- `body?: any` in EndpointArgs
- `Record<string, any>` in multiple places
- `args: any` in type guard

**Suggestion:** Replace with proper types:
- Define a `RequestBody` type or use `unknown`
- Use `Record<string, string>` where appropriate

---

### 7. Duplicate `safeHeaders` Set Definition ✅ NEW
**Priority:** Low  
**File:** `src/index.ts` (lines 79-89, 305-315)

The same `safeHeaders` Set is defined twice in different locations.

**Suggestion:** Extract to a shared constant at the module level.

---

### 8. Variable Shadowing: `lowerKey` ✅ NEW
**Priority:** Low  
**File:** `src/index.ts` (lines 61, 88)

`lowerKey` is declared twice in the same function scope:
```typescript
const lowerKey = key.toLowerCase();  // line 61
// ...
const lowerKey = key.toLowerCase();  // line 88 (shadows the first)
```

**Suggestion:** Remove the duplicate declaration or rename.

---

## Documentation Issues

### 9. Inconsistent Environment Variable Names in Documentation ✅ NEW
**Priority:** Medium  
**File:** `src/resources/config.md` (lines 46-49)

Documentation uses incorrect env var names:
- `REST_BASIC_USERNAME` / `REST_BASIC_PASSWORD` (in docs)
- Actual code uses: `AUTH_BASIC_USERNAME` / `AUTH_BASIC_PASSWORD`

**Suggestion:** Update documentation to match actual implementation.

---

### 10. Missing `host` Parameter Documentation ✅ NEW
**Priority:** Low  
**File:** `src/index.ts` (line 339-356)

The `host` parameter is supported in the code but not documented in the tool's `inputSchema`.

**Suggestion:** Add `host` property to inputSchema with proper description.

---

## Project Structure

### 11. Missing `.nvmrc` File ✅ VALIDATED
**Priority:** Low

No Node.js version lock file exists for consistent development environments.

**Suggestion:** Add `.nvmrc` with content `18`.

---

### 12. No Test Suite ✅ VALIDATED
**Priority:** High

No test files or testing framework is configured. The PR validation workflow (`.github/workflows/pr-validation.yml`) checks for tests but none exist.

**Suggestion:** Add:
- Vitest for unit testing (lighter than Jest, native ESM support)
- Test files in `src/__tests__/` or `tests/`
- Test scripts in `package.json`

---

### 13. Missing Linting Configuration ✅ VALIDATED
**Priority:** Medium

No ESLint or Prettier configuration exists for code style enforcement.

**Suggestion:** Add:
- `.eslintrc.js` with TypeScript support
- `.prettierrc` for formatting rules
- Lint scripts in `package.json`

---

### 14. Missing Source Maps ✅ VALIDATED
**Priority:** Low  
**File:** `tsconfig.json`

Source maps are not generated, making debugging harder.

**Suggestion:** Add `"sourceMap": true` to `compilerOptions`.

---

## Docker

### 15. Incorrect COPY Command in Dockerfile ✅ VALIDATED
**Priority:** High  
**File:** `Dockerfile` (line 28-29)

```dockerfile
COPY --from=builder /app/build /app/build
COPY --from=builder /app/package.json /app/package-lock.json /app/node_modules ./
```

Issues:
1. `node_modules` is incorrectly copied as a file path instead of a directory
2. After copying `/app/build` to `/app/build`, the second COPY overwrites `.` including the build folder

**Suggestion:** Fix to:
```dockerfile
COPY --from=builder /app/build ./build
COPY --from=builder /app/package.json /app/package-lock.json ./
COPY --from=builder /app/node_modules ./node_modules
```

---

## Unused Code

### 16. Unused Export in version.ts ✅ VALIDATED
**Priority:** Low  
**File:** `src/version.ts` (line 3)

`PACKAGE_NAME` is exported but never imported anywhere in the codebase.

**Suggestion:** Either remove the unused export from build script or utilize it (e.g., in logging or error messages).

---

## Security Considerations

### 17. Optional Headers May Expose Sensitive Data ✅ NEW
**Priority:** Medium  
**File:** `src/index.ts` (lines 63-66)

Headers from optional parameters are always included without redaction:
```typescript
if (isFromOptionalParams) {
  sanitized[key] = value;
  continue;
}
```

**Suggestion:** Apply the same auth header redaction logic to optional params.

---

## Summary

| # | Issue | Priority | Category | Status |
|---|-------|----------|----------|--------|
| 1 | Race condition in initialization | High | Code Quality | ✅ Resolved |
| 12 | No test suite | High | Project Structure | ✅ Resolved |
| 15 | Dockerfile COPY issue | High | Docker | ✅ Resolved |
| 2 | Missing env var type safety | Medium | Code Quality | ✅ Resolved |
| 4 | Missing error handling for imports | Medium | Code Quality | ✅ Resolved |
| 5 | Missing request timeout | Medium | Code Quality | ✅ Resolved |
| 6 | Excessive use of `any` type | Medium | Code Quality | ✅ Resolved |
| 9 | Inconsistent env var names in docs | Medium | Documentation | ✅ Resolved |
| 13 | Missing linting configuration | Medium | Project Structure | ✅ Resolved |
| 17 | Optional headers may expose secrets | Medium | Security | ✅ Resolved |
| 3 | Large inline tool description | Low | Code Quality | ✅ Resolved |
| 7 | Duplicate safeHeaders definition | Low | Code Quality | ✅ Resolved |
| 8 | Variable shadowing (lowerKey) | Low | Code Quality | ✅ Resolved |
| 10 | Missing host param in schema | Low | Documentation | ✅ Resolved |
| 11 | Missing .nvmrc | Low | Project Structure | ✅ Resolved |
| 14 | Missing source maps | Low | Project Structure | ✅ Resolved |
| 16 | Unused PACKAGE_NAME export | Low | Unused Code | ✅ Resolved |

**Total: 17 issues — All Resolved ✅**
