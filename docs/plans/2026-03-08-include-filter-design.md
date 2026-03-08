# Include Filter Design

## Problem

In a large monorepo, you may only want to sync specific subtrees (e.g., `project/src` but not `project/tmp`). The current config only supports `ignore` patterns, which means you must enumerate everything you *don't* want. An `include` list inverts this: state what you *do* want, then refine with `ignore`.

## Design Spirit

**Include is simple and explicit.** It names the directories you care about. Leave fine-grained filtering to `ignore`.

## TOML Config

```toml
[settings]
include = ["src", "docs/api"]   # path prefixes relative to sync.local
ignore  = [".git", ".DS_Store"] # applied within included paths
```

- `include = []` (default) means include everything — fully backwards compatible.
- When specified, only paths under those prefixes are synced.
- `ignore` further refines within included paths.
- Patterns are path prefixes relative to `sync.local`, not globs or basenames.

## Evaluation Order

```
file path relative to sync.local
  -> does it fall under an include prefix? (empty = yes to all)
    -> no  -> skip
    -> yes -> does it match an ignore pattern?
      -> yes -> skip
      -> no  -> sync it
```

## Changes

### Config (`internal/config/config.go`)

- Add `Include []string` to `Settings` struct with `toml:"include"` tag.
- Update `DefaultTOML()` to document `include = []`.

### Watcher (`internal/watcher/watcher.go`)

- Add `shouldInclude(path)` using path-prefix matching against the path relative to the watched root.
- In `addRecursive()`: skip directories that are neither a prefix of, nor prefixed by, an include path. (If include is `["src/api"]`, we must still traverse `src/` to reach `src/api/`.)
- In the event loop: check include before ignore. Reject events outside included prefixes.

### Syncer (`internal/syncer/syncer.go`)

When include patterns are present, emit rsync filter rules:

1. `--include=<prefix>/` for each ancestor directory needed for traversal.
2. `--include=<prefix>/**` for each include entry.
3. `--exclude=*` to block everything else.

Existing `--exclude` patterns from ignore lists move *before* the final `--exclude=*` so they can refine within included paths.

### No changes needed to

SSH config, log config, rsync flags (archive/compress/etc.), CLI entry point.
