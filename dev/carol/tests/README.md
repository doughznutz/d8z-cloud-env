# Carol's behaviour tests

What these guard is behaviour the code's structure alone doesn't ensure:
that the report agents can read the generated docs **only** through the
doc server's plain-text view, and nothing else - and that the docs and
that view are what they should be.

| Suite | Where it runs | Needs | Checks |
|---|---|---|---|
| `build/` | `docker build` (the `test` stage; a failure fails the build) | nothing | each CLI's *installed* config allows only `curl -s http://carol:8503/...`; the text view renders and links correctly; the extractors embed no source; the agent user can't read `/app` |
| `system/` | the running `carol-agents` | a deployed stack | no mounts, only the LLM keys in its env, every doc page reachable as text with no broken links and no source |
| `workspace/` | the running `carol`, against its real workspace | a deployed stack | every extractor generates docs from the actual workspace without failing (into a temp dir - published docs untouched) |
| `live/` | the running `carol`, on demand | og running | a real agent (cn via og) reads the docs, and can't leak carol's code, workspace files, `/etc/passwd` or API keys |

Run the post-deploy suites with `tests/run.sh [system|workspace|live|all]`.

**Tests never call a paid model** (claude, gemini, codex) - those cost
money. Their limits are checked without a model, by `build/` (their
configs) and `system/` (their container). The live suite uses only cn
against og, which is local and free; it's skipped when og isn't up.
