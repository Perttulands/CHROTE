# Components

Active components in a CHROTE install:

| Component | Role |
| --- | --- |
| Go server | HTTP API, embedded dashboard, terminal proxy |
| React dashboard | Browser cockpit UI |
| tmux | Durable terminal/session substrate |
| ttyd | Browser terminal transport behind CHROTE |
| bd | Modern Beads issue source |
| beads_viewer `bv` | Optional graph-aware Beads TUI sidecar |
| Tailscale Serve | Tailnet-only HTTPS access |

Current service lanes:

| Lane | Source | State | Ports |
| --- | --- | --- | --- |
| `/srv` proving lane | `/srv/chrote` with data under `/srv/data/chrote` | system unit `chrote-srv.service` | HTTP `8095`, ttyd `7686` |
| legacy rollback lane | `/home/perttu/chrote` | user unit `chrote.service` | HTTP `8094`, ttyd `7683` |

Services Platform V1 components:

| Component | Source | CHROTE role |
| --- | --- | --- |
| TTS Gateway | configured upstream | Full TTS console for health, queue/messages, status, playback, voices, and enqueue actions |
| Context Citadel | configured upstream | Operator console for context document list/read/edit/save/history and grounded questions |
| Service adapter config | CHROTE process env | Server-side service URLs and tokens; browser clients call CHROTE proxies only |

Service adapter environment:

| Variable | Default | Purpose |
| --- | --- | --- |
| `CHROTE_TTS_URL` | `http://127.0.0.1:3100` | TTS Gateway base URL |
| `CHROTE_CONTEXT_API_URL` | `http://127.0.0.1:3200` | Context Citadel base URL |
| `CHROTE_CONTEXT_API_TOKEN` | unset | Optional bearer token injected by the Go server for Context Citadel requests |

The `/srv` proving lane keeps private values in `/srv/chrote/config/chrote.env`
and the installed system units read `/etc/chrote/chrote-srv.env`. The legacy
rollback user unit may load `~/.config/chrote/services.env`. These files are not
tracked components and must not contain values in docs or diffs.

Not active in this install:

| Component | Status |
| --- | --- |
| Gastown | Not installed or assumed |
| Old BV web proxy | Removed; use `bv` inside tmux instead |
| Ralph | Removed from active UI |
| Teams harness launcher | Roadmap only; topology ideas are documented for later |
| ChroteChat/Clawdbot | Not active; proxy-operator idea is documented for later |
