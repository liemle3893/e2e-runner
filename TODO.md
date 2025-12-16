# E2E Test Runner - Implementation TODO

This document outlines features to be implemented, organized by priority.

## Priority Levels

- **P0**: Critical - Core functionality gaps
- **P1**: High - Important features for production use
- **P2**: Medium - Nice-to-have improvements
- **P3**: Low - Future enhancements

---

## P0 - Critical

### 1. Lifecycle Hooks

**Status**: Not implemented
**Files to modify**: `src/core/test-orchestrator.ts`, `src/core/config-loader.ts`, `src/types.ts`

**Description**: Support global lifecycle hooks for setup/teardown across all tests.

**Configuration** (`e2e.config.yaml`):
```yaml
hooks:
  beforeAll: "./hooks/global-setup.ts"   # Run once before all tests
  afterAll: "./hooks/global-teardown.ts" # Run once after all tests
  beforeEach: "./hooks/test-setup.ts"    # Run before each test
  afterEach: "./hooks/test-teardown.ts"  # Run after each test
```

**Implementation Steps**:

1. **Add types** (`src/types.ts`):
   ```typescript
   interface HooksConfig {
     beforeAll?: string;
     afterAll?: string;
     beforeEach?: string;
     afterEach?: string;
   }

   interface E2EConfig {
     // ... existing fields
     hooks?: HooksConfig;
   }
   ```

2. **Create hook loader** (`src/core/hook-loader.ts`):
   ```typescript
   import { pathToFileURL } from 'url';

   interface Hook {
     run: (context: HookContext) => Promise<void>;
   }

   interface HookContext {
     config: E2EConfig;
     environment: string;
     adapters: Record<string, Adapter>;
     variables: Record<string, unknown>;
   }

   export async function loadHook(hookPath: string): Promise<Hook> {
     const absolutePath = path.resolve(hookPath);
     const module = await import(pathToFileURL(absolutePath).href);
     return module.default || module;
   }
   ```

3. **Modify test-orchestrator.ts**:
   - Load hooks on initialization
   - Call `beforeAll` before test execution starts
   - Call `beforeEach` before each test's setup phase
   - Call `afterEach` after each test's teardown phase
   - Call `afterAll` after all tests complete

4. **Example hook file** (`hooks/global-setup.ts`):
   ```typescript
   export default {
     async run(context) {
       console.log('Global setup running...');
       // Initialize shared resources
       await context.adapters.postgresql.execute({
         sql: 'TRUNCATE test_data CASCADE'
       });
     }
   };
   ```

**Estimated effort**: 4-6 hours

---

### 2. Test Dependencies

**Status**: Partially implemented (field exists but not enforced)
**Files to modify**: `src/core/test-orchestrator.ts`, `src/core/test-discovery.ts`

**Description**: Allow tests to declare dependencies on other tests.

**YAML Syntax**:
```yaml
name: TC-ORDER-002
depends:
  - TC-USER-001    # This test must pass first
  - TC-PRODUCT-001

execute:
  # ...
```

**Implementation Steps**:

1. **Build dependency graph** in `test-discovery.ts`:
   ```typescript
   interface TestNode {
     test: UnifiedTestDefinition;
     dependencies: string[];
     dependents: string[];
   }

   function buildDependencyGraph(tests: UnifiedTestDefinition[]): Map<string, TestNode> {
     const graph = new Map<string, TestNode>();

     // Build nodes
     for (const test of tests) {
       graph.set(test.name, {
         test,
         dependencies: test.depends || [],
         dependents: []
       });
     }

     // Build reverse dependencies
     for (const [name, node] of graph) {
       for (const dep of node.dependencies) {
         const depNode = graph.get(dep);
         if (depNode) {
           depNode.dependents.push(name);
         }
       }
     }

     return graph;
   }
   ```

2. **Topological sort for execution order**:
   ```typescript
   function topologicalSort(graph: Map<string, TestNode>): string[] {
     const visited = new Set<string>();
     const result: string[] = [];

     function visit(name: string) {
       if (visited.has(name)) return;
       visited.add(name);

       const node = graph.get(name);
       if (node) {
         for (const dep of node.dependencies) {
           visit(dep);
         }
         result.push(name);
       }
     }

     for (const name of graph.keys()) {
       visit(name);
     }

     return result;
   }
   ```

3. **Modify orchestrator** to:
   - Sort tests by dependency order
   - Skip dependent tests if dependency fails
   - Report dependency failures clearly

4. **Handle circular dependencies**:
   ```typescript
   function detectCycles(graph: Map<string, TestNode>): string[] | null {
     // Detect and report circular dependencies
   }
   ```

**Estimated effort**: 3-4 hours

---

## P1 - High Priority

### 3. Watch Mode

**Status**: CLI flag exists but not implemented
**Files to modify**: `src/cli/run.ts`, new file `src/core/watcher.ts`

**Description**: Re-run tests when files change.

**CLI**:
```bash
e2e run --watch
e2e run --watch --grep "user"
```

**Implementation Steps**:

1. **Add watcher** (`src/core/watcher.ts`):
   ```typescript
   import chokidar from 'chokidar';

   interface WatcherOptions {
     testDir: string;
     configPath: string;
     onChange: (changedFiles: string[]) => void;
   }

   export function createWatcher(options: WatcherOptions) {
     const watcher = chokidar.watch([
       `${options.testDir}/**/*.test.yaml`,
       `${options.testDir}/**/*.test.ts`,
       options.configPath
     ], {
       ignoreInitial: true,
       awaitWriteFinish: {
         stabilityThreshold: 300
       }
     });

     let changedFiles: string[] = [];
     let debounceTimer: NodeJS.Timeout;

     watcher.on('change', (path) => {
       changedFiles.push(path);
       clearTimeout(debounceTimer);
       debounceTimer = setTimeout(() => {
         options.onChange([...changedFiles]);
         changedFiles = [];
       }, 500);
     });

     return watcher;
   }
   ```

2. **Integrate with CLI** (`src/cli/run.ts`):
   - Start watcher if `--watch` flag is set
   - Re-run affected tests on file change
   - Clear console between runs
   - Show "watching for changes..." message

3. **Smart test selection**:
   - If test file changed, run that test
   - If config changed, run all tests
   - If shared fixture changed, run dependent tests

**Dependencies**: `chokidar` package

**Estimated effort**: 3-4 hours

---

### 4. Step-by-Step Interactive Mode

**Status**: CLI flag exists but not implemented
**Files to modify**: `src/cli/run.ts`, new file `src/core/interactive.ts`

**Description**: Pause after each step for debugging.

**CLI**:
```bash
e2e run --step-by-step TC-USER-001
```

**Implementation Steps**:

1. **Create interactive controller** (`src/core/interactive.ts`):
   ```typescript
   import readline from 'readline';

   export class InteractiveController {
     private rl: readline.Interface;

     constructor() {
       this.rl = readline.createInterface({
         input: process.stdin,
         output: process.stdout
       });
     }

     async promptContinue(stepInfo: StepInfo): Promise<'continue' | 'skip' | 'abort'> {
       console.log('\n--- Step completed ---');
       console.log(`Step: ${stepInfo.description}`);
       console.log(`Result: ${JSON.stringify(stepInfo.result, null, 2)}`);
       console.log('\nPress Enter to continue, "s" to skip next, "q" to quit');

       return new Promise((resolve) => {
         this.rl.question('> ', (answer) => {
           if (answer === 'q') resolve('abort');
           else if (answer === 's') resolve('skip');
           else resolve('continue');
         });
       });
     }
   }
   ```

2. **Integrate with step executor**:
   - Inject interactive controller
   - Pause after each step execution
   - Display step result and captured values
   - Allow user to continue, skip, or abort

**Estimated effort**: 2-3 hours

---

### 5. HTTP Traffic Capture

**Status**: CLI flag exists but not implemented
**Files to modify**: `src/adapters/http.adapter.ts`, new file `src/core/traffic-capture.ts`

**Description**: Record HTTP request/response for debugging.

**CLI**:
```bash
e2e run --capture-traffic -o ./traffic/
```

**Implementation Steps**:

1. **Create traffic capture module** (`src/core/traffic-capture.ts`):
   ```typescript
   interface TrafficEntry {
     timestamp: string;
     testName: string;
     stepId: string;
     request: {
       method: string;
       url: string;
       headers: Record<string, string>;
       body?: unknown;
     };
     response: {
       status: number;
       headers: Record<string, string>;
       body?: unknown;
       duration: number;
     };
   }

   export class TrafficCapture {
     private entries: TrafficEntry[] = [];

     record(entry: TrafficEntry) {
       this.entries.push(entry);
     }

     async save(outputPath: string) {
       await fs.writeFile(
         outputPath,
         JSON.stringify(this.entries, null, 2)
       );
     }
   }
   ```

2. **Integrate with HTTP adapter**:
   - Capture full request details before sending
   - Capture full response after receiving
   - Calculate request duration
   - Store with test/step context

3. **Generate HAR format** (optional):
   - Convert traffic to HAR format for browser dev tools

**Estimated effort**: 3-4 hours

---

### 6. TypeScript Test DSL Enhancement

**Status**: Basic TypeScript support exists
**Files to modify**: `src/core/ts-loader.ts`, new file `src/dsl/test-builder.ts`

**Description**: Fluent API for writing TypeScript tests.

**Target API**:
```typescript
import { test, http, postgresql } from '@liemle3893/e2e-runner';

export default test('TC-USER-001')
  .description('User CRUD operations')
  .priority('P0')
  .tags('user', 'crud')
  .setup(async (ctx) => {
    await ctx.postgresql.execute({
      sql: 'DELETE FROM users WHERE email LIKE $1',
      params: ['test-%@example.com']
    });
  })
  .execute(async (ctx) => {
    const response = await ctx.http.post('/users', {
      body: { email: 'test@example.com', name: 'Test' }
    });

    ctx.capture('userId', response.body.id);

    expect(response.status).toBe(201);
    expect(response.body.name).toEqual('Test');
  })
  .verify(async (ctx) => {
    const user = await ctx.postgresql.queryOne({
      sql: 'SELECT * FROM users WHERE id = $1',
      params: [ctx.captured.userId]
    });

    expect(user.email).toBe('test@example.com');
  })
  .teardown(async (ctx) => {
    await ctx.http.delete(`/users/${ctx.captured.userId}`);
  });
```

**Implementation Steps**:

1. **Create test builder** (`src/dsl/test-builder.ts`):
   ```typescript
   export function test(name: string): TestBuilder {
     return new TestBuilder(name);
   }

   class TestBuilder {
     private definition: Partial<UnifiedTestDefinition> = {};

     constructor(name: string) {
       this.definition.name = name;
     }

     description(desc: string): this { /* ... */ }
     priority(p: Priority): this { /* ... */ }
     tags(...tags: string[]): this { /* ... */ }
     setup(fn: SetupFn): this { /* ... */ }
     execute(fn: ExecuteFn): this { /* ... */ }
     verify(fn: VerifyFn): this { /* ... */ }
     teardown(fn: TeardownFn): this { /* ... */ }

     build(): UnifiedTestDefinition { /* ... */ }
   }
   ```

2. **Create adapter context** (`src/dsl/context.ts`):
   ```typescript
   interface TestContext {
     http: HttpClient;
     postgresql: PostgreSQLClient;
     redis: RedisClient;
     mongodb: MongoDBClient;
     eventhub: EventHubClient;
     captured: Record<string, unknown>;
     capture(key: string, value: unknown): void;
   }
   ```

3. **Export from package**:
   ```typescript
   // src/index.ts
   export { test } from './dsl/test-builder';
   export { expect, assert } from './assertions';
   ```

**Estimated effort**: 6-8 hours

---

## P2 - Medium Priority

### 7. Additional Assertion Operators

**Status**: Partially implemented
**Files to modify**: `src/assertions/matchers.ts`

**Missing operators**:

| Operator | Description | Implementation |
|----------|-------------|----------------|
| `startsWith` | String starts with | `value.startsWith(expected)` |
| `endsWith` | String ends with | `value.endsWith(expected)` |
| `isEmpty` | Empty string/array/object | `length === 0` |
| `isNotEmpty` | Not empty | `length > 0` |
| `hasKey` | Object has key | `key in object` |
| `arrayContains` | Array includes item | `array.includes(item)` |
| `matchesSchema` | JSON Schema validation | Use `ajv` library |

**Implementation**:

```typescript
// src/assertions/matchers.ts

startsWith(prefix: string): this {
  if (typeof this.value !== 'string' || !this.value.startsWith(prefix)) {
    throw new AssertionError(
      `Expected "${this.value}" to start with "${prefix}"`
    );
  }
  return this;
}

endsWith(suffix: string): this {
  if (typeof this.value !== 'string' || !this.value.endsWith(suffix)) {
    throw new AssertionError(
      `Expected "${this.value}" to end with "${suffix}"`
    );
  }
  return this;
}

isEmpty(): this {
  const length = this.getLength();
  if (length !== 0) {
    throw new AssertionError(`Expected empty but got length ${length}`);
  }
  return this;
}

matchesSchema(schema: object): this {
  const ajv = new Ajv();
  const validate = ajv.compile(schema);
  if (!validate(this.value)) {
    throw new AssertionError(
      `Schema validation failed: ${JSON.stringify(validate.errors)}`
    );
  }
  return this;
}
```

**YAML support**:
```yaml
assert:
  json:
    - path: "$.email"
      startsWith: "test-"
      endsWith: "@example.com"
    - path: "$.items"
      isNotEmpty: true
    - path: "$.data"
      matchesSchema:
        type: object
        required: [id, name]
```

**Estimated effort**: 2-3 hours

---

### 8. Custom Matchers

**Status**: Not implemented
**Files to modify**: `src/assertions/matchers.ts`, new file `src/assertions/custom-matchers.ts`

**Description**: Allow users to define custom assertion matchers.

**Configuration** (`e2e.config.yaml`):
```yaml
matchers:
  - "./matchers/custom-matchers.ts"
```

**Custom matcher example**:
```typescript
// matchers/custom-matchers.ts
export default {
  toBeValidEmail(value: string): boolean {
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    return emailRegex.test(value);
  },

  toBeValidUUID(value: string): boolean {
    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
    return uuidRegex.test(value);
  }
};
```

**Usage in YAML**:
```yaml
assert:
  json:
    - path: "$.email"
      toBeValidEmail: true
    - path: "$.id"
      toBeValidUUID: true
```

**Estimated effort**: 3-4 hours

---

### 9. Parallel Test Groups

**Status**: Not implemented
**Files to modify**: `src/core/test-orchestrator.ts`, `src/types.ts`

**Description**: Group tests that can run in parallel vs. must run sequentially.

**YAML syntax**:
```yaml
name: TC-USER-001
group: user-tests        # Tests in same group run sequentially
parallelGroup: database  # Groups with same parallelGroup can run together
```

**Implementation**:
- Group tests by `group` field
- Execute groups based on `parallelGroup`
- Within a group, respect `depends` ordering

**Estimated effort**: 4-5 hours

---

### 10. Report History and Trends

**Status**: Not implemented
**Files to create**: `src/reporters/history.reporter.ts`, `src/utils/history-store.ts`

**Description**: Track test results over time for trend analysis.

**Features**:
- Store results in SQLite database
- Track pass/fail rates over time
- Identify flaky tests
- Generate trend reports

**Configuration**:
```yaml
reporters:
  - type: history
    database: "./reports/history.db"
    retention: 30  # days
```

**Implementation**:

```typescript
// src/utils/history-store.ts
import Database from 'better-sqlite3';

export class HistoryStore {
  private db: Database.Database;

  constructor(dbPath: string) {
    this.db = new Database(dbPath);
    this.initSchema();
  }

  private initSchema() {
    this.db.exec(`
      CREATE TABLE IF NOT EXISTS test_runs (
        id INTEGER PRIMARY KEY,
        run_id TEXT,
        timestamp TEXT,
        environment TEXT,
        total INTEGER,
        passed INTEGER,
        failed INTEGER,
        skipped INTEGER,
        duration INTEGER
      );

      CREATE TABLE IF NOT EXISTS test_results (
        id INTEGER PRIMARY KEY,
        run_id TEXT,
        test_name TEXT,
        status TEXT,
        duration INTEGER,
        error_message TEXT
      );
    `);
  }

  recordRun(run: TestRun) { /* ... */ }
  getHistory(testName: string, days: number) { /* ... */ }
  getFlakyTests(threshold: number) { /* ... */ }
}
```

**Estimated effort**: 6-8 hours

---

## P3 - Low Priority

### 11. Read-Only Adapter Mode

**Status**: Not implemented
**Files to modify**: All adapter files

**Description**: Prevent write operations in certain environments.

**Configuration**:
```yaml
environments:
  production:
    readOnly: true    # Disables write operations
```

**Implementation**:
- Add `readOnly` flag to environment config
- Check flag before execute/insert/update/delete operations
- Throw clear error if write attempted in read-only mode

**Estimated effort**: 2 hours

---

### 12. Test Templates

**Status**: Not implemented
**Files to create**: `src/core/template-loader.ts`

**Description**: Reusable test templates.

**Template file** (`templates/crud-test.template.yaml`):
```yaml
name: "TC-{{resource}}-CRUD"
description: "CRUD test for {{resource}}"

variables:
  endpoint: "/{{resource}}"

setup:
  - adapter: postgresql
    action: execute
    sql: "DELETE FROM {{table}} WHERE id LIKE 'test-%'"

execute:
  - adapter: http
    action: request
    method: POST
    url: "{{baseUrl}}{{endpoint}}"
    body: "{{createBody}}"
    capture:
      id: "$.id"
    assert:
      status: 201
```

**Usage**:
```yaml
template: crud-test
parameters:
  resource: users
  table: users
  createBody:
    email: "test@example.com"
    name: "Test User"
```

**Estimated effort**: 4-5 hours

---

### 13. Plugin System

**Status**: Not implemented
**Files to create**: `src/plugins/plugin-loader.ts`

**Description**: Allow third-party plugins for custom adapters/reporters.

**Configuration**:
```yaml
plugins:
  - "@company/e2e-custom-adapter"
  - "./plugins/my-reporter"
```

**Plugin interface**:
```typescript
interface E2EPlugin {
  name: string;
  adapters?: Record<string, AdapterFactory>;
  reporters?: Record<string, ReporterFactory>;
  matchers?: Record<string, MatcherFn>;
  functions?: Record<string, BuiltinFn>;
}
```

**Estimated effort**: 8-10 hours

---

### 14. GraphQL Adapter

**Status**: Not implemented
**Files to create**: `src/adapters/graphql.adapter.ts`

**Description**: Native GraphQL query support.

**YAML syntax**:
```yaml
- adapter: graphql
  action: query
  query: |
    query GetUser($id: ID!) {
      user(id: $id) {
        id
        name
        email
      }
    }
  variables:
    id: "{{captured.user_id}}"
  assert:
    - path: "$.data.user.name"
      equals: "Test User"
```

**Implementation**:
- Use `graphql-request` or native fetch
- Support query, mutation, subscription
- Handle GraphQL errors

**Estimated effort**: 4-5 hours

---

### 15. gRPC Adapter

**Status**: Not implemented
**Files to create**: `src/adapters/grpc.adapter.ts`

**Description**: gRPC service testing support.

**Configuration**:
```yaml
adapters:
  grpc:
    protoPath: "./protos/service.proto"
    address: "localhost:50051"
```

**YAML syntax**:
```yaml
- adapter: grpc
  action: call
  service: UserService
  method: GetUser
  request:
    id: "{{user_id}}"
  capture:
    user_name: "name"
  assert:
    - path: "name"
      equals: "Test User"
```

**Estimated effort**: 6-8 hours

---

## Implementation Order Recommendation

1. **Phase 1** (Core improvements):
   - Lifecycle Hooks (P0)
   - Test Dependencies (P0)
   - Additional Assertion Operators (P2)

2. **Phase 2** (Developer experience):
   - Watch Mode (P1)
   - Step-by-Step Mode (P1)
   - HTTP Traffic Capture (P1)

3. **Phase 3** (Advanced features):
   - TypeScript DSL Enhancement (P1)
   - Custom Matchers (P2)
   - Report History (P2)

4. **Phase 4** (Extensibility):
   - Plugin System (P3)
   - Test Templates (P3)
   - New Adapters (P3)

---

## Contributing

When implementing a feature:

1. Create a branch: `feature/<feature-name>`
2. Update tests in `tests/` directory
3. Update documentation in `docs/`
4. Submit PR with description of changes

## Notes

- All time estimates assume familiarity with the codebase
- Some features may require additional dependencies
- Backward compatibility should be maintained for YAML syntax
