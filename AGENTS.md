# Repository Guide

## Workflow

- Root module requires Go 1.24.5. Run commands from repository root; runtime paths for `./static` and `./data.db` depend on it.
- Full local verification: `go fmt ./...`, `go vet ./...`, then `go test ./...`.
- Focused tests: `go test ./llm`; single test: `go test ./llm -run '^TestChatCompletionsEndpoint$'`.
- Build with `go build -o story_ai .`; run with `go run .` after configuring an OpenAI-compatible endpoint.
- Edit `templates/*.templ`, never generated `templates/*_templ.go`. Run `templ generate` before build/test and commit generated files. Templ CLI is not pinned, so inspect generated diffs for version-only churn.
- No repository CI, task runner, pre-commit hook, or linter config exists; local Go checks are authoritative.

## Runtime

- `main.go` loads root `.env`. `OPENAI_MODEL` is required. `OPENAI_BASE_URL` accepts a base URL or full `/chat/completions` URL and defaults to OpenAI. `OPENAI_API_KEY` may be empty for local endpoints. Generation defaults are controlled by `OPENAI_TEMPERATURE`, `OPENAI_RESPONSE_FORMAT`, `OPENAI_START_MAX_TOKENS`, and `OPENAI_TURN_MAX_TOKENS`. See `.env.example`.
- Port defaults to `9779`. `METRICS_DATABASE_URL` and `STATS_SERVICE_URL` default to localhost ports `8081` and `8080`; those companion services are optional, but story activity attempts asynchronous POSTs to them.
- `/start` reads inspiration from committed SQLite file `data.db`. Sessions and story history are process-local memory, not SQLite; restart loses active games.
- `./deploy.sh` targets hard-coded GCP project `gen-lang-client-0878805821`, region `us-east1`, changes active `gcloud` project, enables APIs, builds, and deploys publicly. Never run it without explicit deployment intent.

## Architecture And Traps

- `main.go` wires provider selection (`llm/`), in-memory sessions (`session/`), handlers, static files, and routes. Story flow lives primarily in `handlers/handler.go`; domain JSON shapes live in `story/state.go`; model behavior lives in `prompts/prompts.go`.
- Model-response contract spans `prompts.BasePrompt`, JSON tags in `story/state.go`, and `handlers.AIResponse` parsing.
- HTMX updates use out-of-band swaps from `templates/update.templ`. Generated story HTML passes through the allowlist in `handlers/validation.go` before `templ.Raw`; background colors are strict hex values. Treat response parsing/rendering changes as an HTML-injection boundary.
- Packages under `middleware/` and custom `logger/` exist but are not connected in `main.go`. Do not assume CSRF, rate limiting, request-size limiting, or custom logging is active without wiring it explicitly.
