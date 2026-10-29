# Scripts

## e2e-roundtrip.sh

Runner for the live GitHub roundtrip suite in `internal/session`. It uses the `e2e_github` tag, `HUNKYARD_ROUNDTRIP_REPO`, and authenticated `gh`, and creates and cleans up a temporary pull request and branch. See `test/roundtrip/README.md` for shared setup and authoring guidance.
