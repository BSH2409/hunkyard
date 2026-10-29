# Hunkyard in Docker

Run the coding agent and Hunkyard inside the container, then open the review in your host browser. Hunkyard binds directly to `0.0.0.0:5000` inside the container; no bridge process is needed.

## Build this checkout

From the repository root:

```bash
docker build -t hunkyard-agent -f integrations/docker/Dockerfile .
```

The image builds Hunkyard from the local source and installs Claude Code. You can replace the agent installation with your preferred CLI.

## Run

Publish the port on host loopback so other machines cannot reach the unauthenticated browser API:

```bash
docker run -it --rm \
    -v "$PWD":/workspace/repo \
    -w /workspace/repo \
    -p 127.0.0.1:5000:5000 \
    hunkyard-agent
```

Start your agent normally. When it launches Hunkyard, open `http://localhost:5000` on your host. Stop the review with `hunkyard stop --session ID` from its owning agent session, or stop the container when finished.

## Use an existing container

Publish port 5000 on host loopback, then run Hunkyard normally through Codex or Claude. No network flags or environment variables are needed:

```bash
hunkyard
```

Publish `127.0.0.1:5000:5000` with Docker or Compose and open `http://localhost:5000` on the host. The default listener is `0.0.0.0:5000`, and network binding is enabled by default.

For local-only listening outside Docker, override the host:

```bash
hunkyard --host 127.0.0.1
```

`HUNKYARD_HOST` and the global `host` config also override the default. Outbound requests remain restricted to loopback regardless of the listener setting.

## Multiple containers and persisted reviews

Keep the internal port at 5000 and choose a different host port for each container, for example `-p 127.0.0.1:5001:5000`. Open `http://localhost:5001` for that container.

Mount a volume at `/root/.hunkyard` to retain review state across container removal. Mount the agent's configuration directory separately if you want its login state to persist. Hunkyard needs no provider credentials, database, or outbound runtime requests.
