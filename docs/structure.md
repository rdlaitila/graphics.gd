# Repository structure

Top-level layout. Each subdirectory may have a `Readme.md` or doc comments
covering its internals.

- `cmd/` — executables (`gd` CLI, `gdnext` CLI, `gdnext-ci` CI driver).
- `product/` — canonical product metadata database. Pure data + lookup helpers; no `graphics.gd` imports allowed.
- `classdb/` — generated Godot class bindings.
- `variant/` — pure-Go vector/math types + variant glue.
- `shaders/` — write-shaders-in-Go support.
- `startup/` — engine startup + reload machinery.
- `internal/` — implementation details shared across the module.
- `examples/` — runnable example projects (CI canary lives in `examples/canarybird/`).
- `docs/` — project documentation, design rationale, style guide, agent instructions.
