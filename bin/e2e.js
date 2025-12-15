#!/usr/bin/env node

/**
 * E2E Test Runner CLI Entry Point
 *
 * This file serves as the entry point for the `e2e` command when installed globally.
 */

const { main } = require('../dist/index.js');

main().catch((error) => {
    console.error('Fatal error:', error);
    process.exit(1);
});
