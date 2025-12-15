/**
 * E2E Test Runner - Adapters Index
 *
 * Re-exports all adapters for convenient imports
 */

export { BaseAdapter, runAdapterAssertions, captureValues } from './base.adapter';
export { HTTPAdapter } from './http.adapter';
export type { HTTPRequestParams, HTTPResponse, HTTPAssertion, JSONPathAssertion } from './http.adapter';
export { PostgreSQLAdapter } from './postgresql.adapter';
export type { PostgreSQLAssertion } from './postgresql.adapter';
export { RedisAdapter } from './redis.adapter';
export type { RedisAssertion } from './redis.adapter';
export { MongoDBAdapter } from './mongodb.adapter';
export type { MongoDBAssertion } from './mongodb.adapter';
export { EventHubAdapter } from './eventhub.adapter';
export type { EventHubAssertion } from './eventhub.adapter';
export {
  AdapterRegistry,
  createAdapterRegistry,
  parseAdapterType,
  getRequiredAdapters,
} from './adapter-registry';
export type { AdapterRegistryOptions } from './adapter-registry';
