# Tryve Documentation

A YAML-driven end-to-end test runner with multi-adapter support for testing APIs,
databases, caches, and message queues. Single Go binary, no runtime dependencies.

## User Reference — `docs/sections/`

These are the maintained docs. They are embedded into the binary, printed by
`tryve doc <section>`, and installed into users' projects by
`tryve install --skills`, so an agent reads exactly what a human reads.

| Section | Contents | Command |
|---------|----------|---------|
| [Getting Started](./sections/getting-started.md) | Install and run your first test | `tryve doc getting-started` |
| [YAML Tests](./sections/yaml-test.md) | Test file syntax and structure | `tryve doc yaml-test` |
| [Assertions](./sections/assertions.md) | Every operator and assertion shape | `tryve doc assertions` |
| [Built-in Functions](./sections/built-in-functions.md) | `$uuid`, `$now`, `$jwt`, `$jsonPath`, … | `tryve doc built-in-functions` |
| [Configuration](./sections/config.md) | `e2e.config.yaml` reference | `tryve doc config` |
| [CLI](./sections/cli.md) | Commands and flags | `tryve doc cli` |
| [Examples](./sections/examples.md) | Common patterns and recipes | `tryve doc examples` |
| [Adapters](./sections/adapters/index.md) | Overview and comparison | `tryve doc adapters` |

Per-adapter references: [http](./sections/adapters/http.md) ·
[shell](./sections/adapters/shell.md) ·
[postgresql](./sections/adapters/postgresql.md) ·
[mongodb](./sections/adapters/mongodb.md) ·
[redis](./sections/adapters/redis.md) ·
[kafka](./sections/adapters/kafka.md) ·
[eventhub](./sections/adapters/eventhub.md)

Every section is registered in [`sections/index.json`](./sections/index.json),
which is what `tryve doc` reads.

## Other Documents

| Document | Contents |
|----------|----------|
| [Product Overview](./product-overview.md) | What Tryve is for and how it compares to other tools |
| [Go Port Design](./design/go-port.md) | The architecture decisions behind the current implementation |

## For Contributors

- [`CLAUDE.md`](../CLAUDE.md) / [`AGENTS.md`](../AGENTS.md) — repository layout,
  shared utilities, the adapter checklist, and the Documentation Sync Rule
- [`TODO.md`](../TODO.md) — backlog and known debt

## Changing Documentation

`docs/sections/` and `skills/e2e-runner/` are compiled into the binary by
`embed.go`. A change there only reaches users after `make build`.

Any change to CLI commands, adapters, configuration, assertions, built-in
functions, or YAML syntax must update all three of: the section markdown,
`sections/index.json`, and `skills/e2e-runner/SKILL.md`. See the Documentation
Sync Rule in `CLAUDE.md`.

Never document a feature that is not implemented. Test authors write against
these pages, and a documented-but-missing field means their assertions silently
do nothing.
