/**
 * E2E Test Runner - MongoDB Adapter
 *
 * MongoDB operations using native driver
 */

import { AdapterError, AssertionError } from '../errors';
import type { AdapterConfig, AdapterContext, AdapterStepResult, Logger } from '../types';
import { BaseAdapter } from './base.adapter';
import { runAssertion, type BaseAssertion } from '../assertions/assertion-runner';

// ============================================================================
// Types
// ============================================================================

export interface MongoDBAssertion extends BaseAssertion {
  path: string;
}

// ============================================================================
// MongoDB Adapter
// ============================================================================

export class MongoDBAdapter extends BaseAdapter {
  private client: import('mongodb').MongoClient | null = null;
  private db: import('mongodb').Db | null = null;

  constructor(config: AdapterConfig, logger: Logger) {
    super(config, logger);
  }

  get name(): string {
    return 'mongodb';
  }

  async connect(): Promise<void> {
    if (this.connected) return;

    try {
      const { MongoClient } = await import('mongodb');

      this.client = new MongoClient(this.config.connectionString as string);
      await this.client.connect();

      const dbName = this.config.database as string | undefined;
      this.db = this.client.db(dbName);

      // Test connection
      await this.db.command({ ping: 1 });

      this.connected = true;
      this.logger.info('MongoDB connected');
    } catch (error) {
      throw new AdapterError(
        'mongodb',
        'connect',
        `Failed to connect: ${error instanceof Error ? error.message : String(error)}`
      );
    }
  }

  async disconnect(): Promise<void> {
    if (this.client) {
      await this.client.close();
      this.client = null;
      this.db = null;
      this.connected = false;
      this.logger.info('MongoDB disconnected');
    }
  }

  async execute(
    action: string,
    params: Record<string, unknown>,
    ctx: AdapterContext
  ): Promise<AdapterStepResult> {
    if (!this.db) {
      throw new AdapterError('mongodb', action, 'Not connected');
    }

    this.logAction(action, { collection: params.collection, action });

    const start = Date.now();
    const collectionName = params.collection as string;
    const collection = this.db.collection(collectionName);

    try {
      let result: unknown;

      switch (action) {
        case 'insertOne': {
          const doc = params.document as Record<string, unknown>;
          const insertResult = await collection.insertOne(doc);
          result = { insertedId: insertResult.insertedId.toString() };
          break;
        }

        case 'insertMany': {
          const docs = params.documents as Record<string, unknown>[];
          const insertManyResult = await collection.insertMany(docs);
          result = { insertedIds: insertManyResult.insertedIds };
          break;
        }

        case 'findOne': {
          const filter = await this.normalizeFilter(params.filter as Record<string, unknown>);
          result = await collection.findOne(filter);
          break;
        }

        case 'find': {
          const filter = await this.normalizeFilter(params.filter as Record<string, unknown>);
          result = await collection.find(filter).toArray();
          break;
        }

        case 'updateOne': {
          const filter = await this.normalizeFilter(params.filter as Record<string, unknown>);
          const update = params.update as Record<string, unknown>;
          const updateResult = await collection.updateOne(filter, update);
          result = {
            matchedCount: updateResult.matchedCount,
            modifiedCount: updateResult.modifiedCount,
          };
          break;
        }

        case 'updateMany': {
          const filter = await this.normalizeFilter(params.filter as Record<string, unknown>);
          const update = params.update as Record<string, unknown>;
          const updateManyResult = await collection.updateMany(filter, update);
          result = {
            matchedCount: updateManyResult.matchedCount,
            modifiedCount: updateManyResult.modifiedCount,
          };
          break;
        }

        case 'deleteOne': {
          const filter = await this.normalizeFilter(params.filter as Record<string, unknown>);
          const deleteResult = await collection.deleteOne(filter);
          result = { deletedCount: deleteResult.deletedCount };
          break;
        }

        case 'deleteMany': {
          const filter = await this.normalizeFilter(params.filter as Record<string, unknown>);
          const deleteManyResult = await collection.deleteMany(filter);
          result = { deletedCount: deleteManyResult.deletedCount };
          break;
        }

        case 'count': {
          const filter = await this.normalizeFilter(params.filter as Record<string, unknown> || {});
          result = await collection.countDocuments(filter);
          break;
        }

        case 'aggregate': {
          const pipeline = params.pipeline as Record<string, unknown>[];
          result = await collection.aggregate(pipeline).toArray();
          break;
        }

        default:
          throw new AdapterError('mongodb', action, `Unknown action: ${action}`);
      }

      const duration = Date.now() - start;

      // Handle captures
      if (params.capture && result) {
        for (const [varName, path] of Object.entries(params.capture as Record<string, string>)) {
          ctx.capture(varName, this.getNestedValue(result, path));
        }
      }

      // Handle assertions
      if (params.assert && result) {
        this.runAssertions(result, params.assert as MongoDBAssertion[]);
      }

      this.logResult(action, true, duration);

      return this.successResult(result, duration);
    } catch (error) {
      const duration = Date.now() - start;
      this.logResult(action, false, duration);

      if (error instanceof AssertionError || error instanceof AdapterError) {
        throw error;
      }

      throw new AdapterError(
        'mongodb',
        action,
        error instanceof Error ? error.message : String(error)
      );
    }
  }

  async healthCheck(): Promise<boolean> {
    if (!this.db) return false;

    try {
      await this.db.command({ ping: 1 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Normalize filter - convert _id strings to ObjectId
   */
  private async normalizeFilter(
    filter: Record<string, unknown>
  ): Promise<Record<string, unknown>> {
    const { ObjectId } = await import('mongodb');
    const normalized = { ...filter };

    // Handle _id field
    if (typeof normalized._id === 'string' && ObjectId.isValid(normalized._id)) {
      normalized._id = new ObjectId(normalized._id);
    }

    // Handle $oid notation (from YAML)
    if (normalized._id && typeof normalized._id === 'object') {
      const idObj = normalized._id as Record<string, unknown>;
      if ('$oid' in idObj && typeof idObj.$oid === 'string') {
        normalized._id = new ObjectId(idObj.$oid);
      }
    }

    return normalized;
  }

  /**
   * Get nested value using dot notation
   */
  private getNestedValue(obj: unknown, path: string): unknown {
    if (!path) return obj;

    return path.split('.').reduce((current, key) => {
      if (current && typeof current === 'object') {
        return (current as Record<string, unknown>)[key];
      }
      return undefined;
    }, obj);
  }

  /**
   * Run assertions on MongoDB result using shared runner
   */
  private runAssertions(data: unknown, assertions: MongoDBAssertion[]): void {
    for (const assertion of assertions) {
      const value = this.getNestedValue(data, assertion.path);
      runAssertion(value, assertion, assertion.path);
    }
  }
}
