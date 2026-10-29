# Roundtrip integration tests

The roundtrip suite exercises `hunkyard pull` and `hunkyard push` against real GitHub pull requests. The tests live in `internal/session` and drive the compiled Hunkyard CLI; the GitHub API is used only for fixture lifecycle and remote-state assertions.

| Provider | Build tag | Project variable | CLI | Runner |
| --- | --- | --- | --- | --- |
| GitHub | `e2e_github` | `HUNKYARD_ROUNDTRIP_REPO` | `gh` | `make e2e-roundtrip` |

Each scenario:

1. Creates a unique branch and opens a PR against the sandbox's initialized default branch.
2. Drives public `hunkyard comment`, `hunkyard pull`, and `hunkyard push` commands through a state transition.
3. Asserts both local review-file state and live provider comment/thread state.
4. Closes the PR and deletes the temporary branch in `t.Cleanup`.

## One-time setup

Use throwaway repositories named `<owner>/hunkyard-roundtrip-sandbox`. The shared default is `hunkyard-md/hunkyard-roundtrip-sandbox`.

- Export `HUNKYARD_ROUNDTRIP_REPO=<owner>/hunkyard-roundtrip-sandbox` and confirm `gh auth status`. The harness clones over SSH by default; set `HUNKYARD_ROUNDTRIP_CLONE_URL` to override it.

## Running

```bash
make e2e-roundtrip

./scripts/e2e-roundtrip.sh -run TestRoundtrip_PushIsIdempotent -v
```

## Adding a scenario

- Scenarios use `roundtrip_integration_test.go` and `roundtrip_helpers_test.go`; start with `e := newRoundtripEnv(t)`.
- Keep provider API calls in harness helpers. Exercise Hunkyard behavior through the compiled binary rather than calling provider implementation functions directly.
- Assert remote IDs, thread/reply structure, resolution, deletion, and repeated pull/push idempotency where applicable.

## Caveats

- These tests are intentionally excluded from default `go test ./...`: they are slow, networked, rate-limited, and mutate sandbox repositories.
- Closed PR history remains in the sandbox, but temporary branches and open PRs should not remain after cleanup.
- If a process is killed before `t.Cleanup`, inspect the sandbox for branches named `rt-*`.
