import { Router, Request, Response } from 'express';
import * as postgres from '../services/postgres.js';

const router = Router();

interface User {
    id: number;
    email: string;
    name: string;
    created_at: Date;
    updated_at: Date;
}

interface CreateUserRequest {
    email: string;
    name: string;
}

interface UpdateUserRequest {
    email?: string;
    name?: string;
}

/**
 * POST /users - Create a new user.
 */
router.post('/', async (req: Request, res: Response) => {
    try {
        const { email, name } = req.body as CreateUserRequest;

        if (!email || !name) {
            res.status(400).json({ error: 'email and name are required' });
            return;
        }

        const result = await postgres.query<User>(
            `INSERT INTO users (email, name) VALUES ($1, $2) RETURNING *`,
            [email, name]
        );

        res.status(201).json(result[0]);
    } catch (error) {
        console.error('Error creating user:', error);
        res.status(500).json({ error: 'Failed to create user' });
    }
});

/**
 * GET /users - List all users.
 */
router.get('/', async (_req: Request, res: Response) => {
    try {
        const users = await postgres.query<User>('SELECT * FROM users ORDER BY created_at DESC');
        res.json(users);
    } catch (error) {
        console.error('Error listing users:', error);
        res.status(500).json({ error: 'Failed to list users' });
    }
});

/**
 * GET /users/:id - Get a user by ID.
 */
router.get('/:id', async (req: Request, res: Response) => {
    try {
        const { id } = req.params;
        const user = await postgres.queryOne<User>(
            'SELECT * FROM users WHERE id = $1',
            [id]
        );

        if (!user) {
            res.status(404).json({ error: 'User not found' });
            return;
        }

        res.json(user);
    } catch (error) {
        console.error('Error getting user:', error);
        res.status(500).json({ error: 'Failed to get user' });
    }
});

/**
 * PUT /users/:id - Update a user.
 */
router.put('/:id', async (req: Request, res: Response) => {
    try {
        const { id } = req.params;
        const { email, name } = req.body as UpdateUserRequest;

        const updates: string[] = [];
        const values: unknown[] = [];
        let paramIndex = 1;

        if (email !== undefined) {
            updates.push(`email = $${paramIndex++}`);
            values.push(email);
        }
        if (name !== undefined) {
            updates.push(`name = $${paramIndex++}`);
            values.push(name);
        }

        if (updates.length === 0) {
            res.status(400).json({ error: 'No fields to update' });
            return;
        }

        updates.push(`updated_at = NOW()`);
        values.push(id);

        const result = await postgres.query<User>(
            `UPDATE users SET ${updates.join(', ')} WHERE id = $${paramIndex} RETURNING *`,
            values
        );

        if (result.length === 0) {
            res.status(404).json({ error: 'User not found' });
            return;
        }

        res.json(result[0]);
    } catch (error) {
        console.error('Error updating user:', error);
        res.status(500).json({ error: 'Failed to update user' });
    }
});

/**
 * DELETE /users/:id - Delete a user.
 */
router.delete('/:id', async (req: Request, res: Response) => {
    try {
        const { id } = req.params;
        const result = await postgres.execute(
            'DELETE FROM users WHERE id = $1',
            [id]
        );

        if (result.rowCount === 0) {
            res.status(404).json({ error: 'User not found' });
            return;
        }

        res.status(204).send();
    } catch (error) {
        console.error('Error deleting user:', error);
        res.status(500).json({ error: 'Failed to delete user' });
    }
});

export default router;
