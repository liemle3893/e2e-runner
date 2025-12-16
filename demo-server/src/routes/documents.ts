import { Router, Request, Response } from 'express';
import * as mongodb from '../services/mongodb.js';

const router = Router();

const COLLECTION_NAME = 'documents';

interface Document {
    _id?: string;
    title: string;
    content: string;
    tags?: string[];
    createdAt: Date;
    updatedAt: Date;
}

interface CreateDocumentRequest {
    title: string;
    content: string;
    tags?: string[];
}

/**
 * POST /documents - Create a new document.
 */
router.post('/', async (req: Request, res: Response) => {
    try {
        const { title, content, tags } = req.body as CreateDocumentRequest;

        if (!title || !content) {
            res.status(400).json({ error: 'title and content are required' });
            return;
        }

        const now = new Date();
        const document: Omit<Document, '_id'> = {
            title,
            content,
            tags: tags || [],
            createdAt: now,
            updatedAt: now,
        };

        const result = await mongodb.insertOne(COLLECTION_NAME, document);

        res.status(201).json({
            id: result.insertedId,
            ...document,
        });
    } catch (error) {
        console.error('Error creating document:', error);
        res.status(500).json({ error: 'Failed to create document' });
    }
});

/**
 * GET /documents - List all documents (with optional filter).
 */
router.get('/', async (req: Request, res: Response) => {
    try {
        const { tag, title } = req.query;

        const filter: Record<string, unknown> = {};
        if (tag) {
            filter.tags = tag;
        }
        if (title) {
            filter.title = { $regex: title, $options: 'i' };
        }

        const documents = await mongodb.find<Document>(COLLECTION_NAME, filter);

        res.json(documents.map(doc => ({
            id: doc._id,
            title: doc.title,
            content: doc.content,
            tags: doc.tags,
            createdAt: doc.createdAt,
            updatedAt: doc.updatedAt,
        })));
    } catch (error) {
        console.error('Error listing documents:', error);
        res.status(500).json({ error: 'Failed to list documents' });
    }
});

/**
 * GET /documents/:id - Get a document by ID.
 */
router.get('/:id', async (req: Request, res: Response) => {
    try {
        const { id } = req.params;
        const document = await mongodb.findById<Document>(COLLECTION_NAME, id);

        if (!document) {
            res.status(404).json({ error: 'Document not found' });
            return;
        }

        res.json({
            id: document._id,
            title: document.title,
            content: document.content,
            tags: document.tags,
            createdAt: document.createdAt,
            updatedAt: document.updatedAt,
        });
    } catch (error) {
        console.error('Error getting document:', error);
        res.status(500).json({ error: 'Failed to get document' });
    }
});

/**
 * DELETE /documents/:id - Delete a document.
 */
router.delete('/:id', async (req: Request, res: Response) => {
    try {
        const { id } = req.params;
        const result = await mongodb.deleteById(COLLECTION_NAME, id);

        if (result.deletedCount === 0) {
            res.status(404).json({ error: 'Document not found' });
            return;
        }

        res.status(204).send();
    } catch (error) {
        console.error('Error deleting document:', error);
        res.status(500).json({ error: 'Failed to delete document' });
    }
});

/**
 * DELETE /documents - Delete documents by filter.
 */
router.delete('/', async (req: Request, res: Response) => {
    try {
        const { tag, title } = req.query;

        const filter: Record<string, unknown> = {};
        if (tag) {
            filter.tags = tag;
        }
        if (title) {
            filter.title = title;
        }

        if (Object.keys(filter).length === 0) {
            res.status(400).json({ error: 'Filter required (tag or title)' });
            return;
        }

        const result = await mongodb.deleteMany(COLLECTION_NAME, filter);

        res.json({ deletedCount: result.deletedCount });
    } catch (error) {
        console.error('Error deleting documents:', error);
        res.status(500).json({ error: 'Failed to delete documents' });
    }
});

export default router;
