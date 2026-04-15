# release-tools

Shared release automation for Go repositories using Goreleaser, Codeberg, and
Make.

Repos should consume this toolkit from a pinned runtime checkout such as
`.tmp/release-tools/current` and include `make/release-tools.mk` from there.

The shared Make frontend validates required release configuration before running
release commands. Consumers should set at least `RELEASE_PROJECT` and
`RELEASE_OWNER`, and `release-tag` also requires `VERSION`.

This repository also ships ready-to-copy consumer examples for the runtime
bootstrap flow:

- `docs/README.md`
- `examples/bootstrap-release-tools.sh`
- `examples/Makefile.release-tools`
- `examples/.release-tools-version`
- `examples/forgejo-release.yml`

See these docs:

- `docs/README.md` for the docs index
- `docs/usage.md` for the integration contract and consumer setup guide
- `docs/agent-release-flow.md` for the reusable release pattern and rationale
