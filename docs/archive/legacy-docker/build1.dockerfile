FROM ubuntu:24.04

# 1. Core Tools & SSH & Healthcheck Utils + Locales for UTF-8/emoji support
RUN apt-get update && apt-get install -y \
    curl git tmux golang-go nodejs npm python3 python3-pip \
    sudo openssh-server ttyd nginx \
    locales \
    && rm -rf /var/lib/apt/lists/* \
    && locale-gen en_US.UTF-8

# Set UTF-8 locale environment
ENV LANG=en_US.UTF-8
ENV LC_ALL=en_US.UTF-8
ENV LANGUAGE=en_US:en

# IS_SANDBOX=1 allows Claude Code to run --dangerously-skip-permissions as root
# This is safe because we're already in a Docker container (sandboxed)
ENV IS_SANDBOX=1

# 2. Setup SSH (key-based auth only - no password)
# For local dev: mount your SSH public key or use ttyd web terminal
# Password auth is disabled for security
RUN mkdir /var/run/sshd && \
    sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config && \
    sed -i 's/#PasswordAuthentication yes/PasswordAuthentication no/' /etc/ssh/sshd_config && \
    mkdir -p /root/.ssh && chmod 700 /root/.ssh

# 3. Install AI Coding Tools
RUN npm install -g @anthropic-ai/claude-code

# 4. Toolchain setup (Go env + tmux socket)
ENV GOPATH=/root/go
ENV PATH=$PATH:$GOPATH/bin
# Ensure consistent tmux socket location for all processes (API, ttyd, SSH)
ENV TMUX_TMPDIR=/tmp

# NOTE: This image intentionally does NOT download or install Gastown/Beads/beads_viewer
# during build. Provide them via runtime mounts under ./vendor and run
# `build-vendored-tools` inside the container to compile them.

# Prefer locally-built vendored tools when present.
# - If upstream/baked-in binaries exist, they are kept as *.upstream
# - Wrappers `gt`, `bd`, and `bv` prefer /opt/*-bin/* outputs
RUN set -eux && \
  if [ -x /usr/local/bin/gt ] && [ ! -e /usr/local/bin/gt.upstream ]; then mv /usr/local/bin/gt /usr/local/bin/gt.upstream; fi && \
  if [ -x /usr/local/bin/bd ] && [ ! -e /usr/local/bin/bd.upstream ]; then mv /usr/local/bin/bd /usr/local/bin/bd.upstream; fi && \
  if [ -x /usr/local/bin/bv ] && [ ! -e /usr/local/bin/bv.upstream ]; then mv /usr/local/bin/bv /usr/local/bin/bv.upstream; fi && \
  printf '#!/bin/bash\nset -euo pipefail\nexport TMUX_TMPDIR=${TMUX_TMPDIR:-/tmp}\nexport IS_SANDBOX=${IS_SANDBOX:-1}\nif [ -x /opt/gastown-bin/gt ]; then exec /opt/gastown-bin/gt "$@"; fi\nif [ -x /usr/local/bin/gt.upstream ]; then exec /usr/local/bin/gt.upstream "$@"; fi\necho "gt not found. Build it: build-vendored-tools (expects /opt/gastown-src mounted)." >&2\nexit 127\n' > /usr/local/bin/gt && \
  chmod +x /usr/local/bin/gt && \
  printf '#!/bin/bash\nset -euo pipefail\nexport TMUX_TMPDIR=${TMUX_TMPDIR:-/tmp}\nexport IS_SANDBOX=${IS_SANDBOX:-1}\nif [ -x /opt/beads-bin/bd ]; then exec /opt/beads-bin/bd "$@"; fi\nif [ -x /usr/local/bin/bd.upstream ]; then exec /usr/local/bin/bd.upstream "$@"; fi\necho "bd not found. Build it: build-vendored-tools (expects /opt/beads-src mounted)." >&2\nexit 127\n' > /usr/local/bin/bd && \
  chmod +x /usr/local/bin/bd && \
  printf '#!/bin/bash\nset -euo pipefail\nexport TMUX_TMPDIR=${TMUX_TMPDIR:-/tmp}\nexport IS_SANDBOX=${IS_SANDBOX:-1}\nif [ -x /opt/beadsviewer-bin/bv ]; then exec /opt/beadsviewer-bin/bv "$@"; fi\nif [ -x /usr/local/bin/bv.upstream ]; then exec /usr/local/bin/bv.upstream "$@"; fi\necho "bv not found. Build it: build-vendored-tools (expects /opt/beadsviewer-src mounted)." >&2\nexit 127\n' > /usr/local/bin/bv && \
  chmod +x /usr/local/bin/bv

# 5. Setup directories and permissions
# /code - mounted from E:/Code (RW)
# /vault - mounted from E:/Vault (RW)
RUN mkdir -p /code && \
    mkdir -p /vault && \
    chown root:root /code && \
    chown root:root /vault && \
    chmod 775 /code && \
    chmod 755 /vault

# 5a. Setup minimal tmux config (for root - all Gastown operations run as root)
RUN printf '# UTF-8 support for emojis and special characters\n\
set -gq utf8 on\n\
set -gq status-utf8 on\n\
setw -gq utf8 on\n\
\n\
# Start windows and panes at 1, not 0\n\
set -g base-index 1\n\
setw -g pane-base-index 1\n\
\n\
# True color support\n\
set -g default-terminal "tmux-256color"\n\
set -ga terminal-overrides ",xterm-256color:Tc"\n\
\n\
# Transparent background - inherit from terminal\n\
set -g window-style "bg=default"\n\
set -g window-active-style "bg=default"\n\
' > /root/.tmux.conf && \
    cp /root/.tmux.conf /etc/skel/.tmux.conf

# 5a2. Create tmux session launcher script for ttyd
# Now runs as root - all Gastown sessions use single tmux socket at /tmp/tmux-0/
RUN printf '#!/bin/bash\n\
# Terminal launcher script for ttyd\n\
# Usage: terminal-launch.sh [session_name] [mode]\n\
# mode: tmux (default) or shell\n\
# All operations run as root with IS_SANDBOX=1 for consistent tmux socket\n\
export HOME=/root\n\
export USER=root\n\
export PATH=/usr/local/bin:/usr/bin:/bin:$HOME/.local/bin:$HOME/go/bin\n\
export TMUX_TMPDIR=/tmp\n\
export IS_SANDBOX=1\n\
# UTF-8 locale for emoji and special character support\n\
export LANG=en_US.UTF-8\n\
export LC_ALL=en_US.UTF-8\n\
cd /code\n\
\n\
SESSION="$1"\n\
MODE="${2:-tmux}"\n\
\n\
# If mode is shell, just give a plain bash shell\n\
if [ "$MODE" = "shell" ]; then\n\
  exec bash -l\n\
fi\n\
\n\
# tmux mode: attach to session if specified\n\
if [ -n "$SESSION" ]; then\n\
  if tmux has-session -t "$SESSION"; then\n\
    exec tmux attach-session -t "$SESSION"\n\
  else\n\
    echo "Error: Session $SESSION not found."\n\
    echo "Debug info:"\n\
    echo "User: $(whoami)"\n\
    echo "TMUX_TMPDIR=$TMUX_TMPDIR"\n\
    echo "Available sessions:"\n\
    tmux list-sessions || echo "Failed to list sessions"\n\
    sleep 10\n\
    exit 1\n\
  fi\n\
else\n\
  # No session specified - just give a shell\n\
  exec bash -l\n\
fi\n\
' > /usr/local/bin/terminal-launch.sh && chmod +x /usr/local/bin/terminal-launch.sh

# 5a3. Utility scripts for root-only model
# All Gastown operations run as root with IS_SANDBOX=1, using single tmux socket at /tmp/tmux-0/
RUN printf '#!/bin/bash\n\
set -euo pipefail\n\
\n\
# Build locally-mounted tool repos (if present) into /opt/*-bin.\n\
# Intended for iterating on your own forks without rebuilding the image.\n\
\n\
mkdir -p /opt/gastown-bin /opt/beads-bin /opt/beadsviewer-bin\n\
\n\
if [ -f /opt/gastown-src/go.mod ]; then\n\
  echo "Building Gastown -> /opt/gastown-bin/gt"\n\
  cd /opt/gastown-src && go build -buildvcs=false -o /opt/gastown-bin/gt ./cmd/gt\n\
fi\n\
\n\
if [ -f /opt/beads-src/go.mod ]; then\n\
  echo "Building Beads -> /opt/beads-bin/bd"\n\
  cd /opt/beads-src && go build -buildvcs=false -o /opt/beads-bin/bd ./cmd/bd\n\
fi\n\
\n\
if [ -f /opt/beadsviewer-src/go.mod ]; then\n\
  echo "Building beads_viewer -> /opt/beadsviewer-bin/bv"\n\
  cd /opt/beadsviewer-src && go build -buildvcs=false -o /opt/beadsviewer-bin/bv ./cmd/bv\n\
fi\n\
\n\
echo "Done. Vendored binaries at /opt/gastown-bin/gt, /opt/beads-bin/bd, /opt/beadsviewer-bin/bv"\n\
' > /usr/local/bin/build-vendored-tools && chmod +x /usr/local/bin/build-vendored-tools

RUN printf '#!/bin/bash\n\
set -euo pipefail\n\
\n\
echo "== tmux sessions (all run as root with IS_SANDBOX=1) =="\n\
TMUX_TMPDIR=/tmp tmux list-sessions 2>/dev/null || echo "(none)"\n\
\n\
echo\n\
\n\
echo "== sockets =="\n\
ls -la /tmp/tmux-* 2>/dev/null || echo "(no tmux sockets)"\n\
\n\
echo\n\
echo "All Gastown operations run as root. Dashboard/API see all sessions via /tmp/tmux-0/ socket."\n\
' > /usr/local/bin/arena-sessions && chmod +x /usr/local/bin/arena-sessions

# 5b. Add README to container root
COPY README.md /README.md

# 5c. Copy API server
COPY api /srv/api
RUN cd /srv/api && npm install --production 2>/dev/null || npm install express

# 5d. Copy nginx config and dashboard
COPY nginx/nginx.conf /etc/nginx/nginx.conf
COPY dashboard/dist /usr/share/nginx/html

WORKDIR /code

# 6. Entrypoint script - simplified for root-only operation
# All Gastown operations run as root with IS_SANDBOX=1 for consistent tmux socket
RUN printf '#!/bin/bash\n\
# Export IS_SANDBOX for all child processes (allows Claude --dangerously-skip-permissions as root)\n\
export IS_SANDBOX=1\n\
export TMUX_TMPDIR=/tmp\n\
\n\
# Fix /code permissions: root owns everything for Gastown operations\n\
chmod -R 775 /code 2>/dev/null || true\n\
# Fix /vault permissions\n\
chmod -R 755 /vault 2>/dev/null || true\n\
\n\
# Ensure .tmux.conf exists for root\n\
if [ ! -f /root/.tmux.conf ]; then\n\
  cp /etc/skel/.tmux.conf /root/.tmux.conf 2>/dev/null || true\n\
fi\n\
\n\
# Build vendored tools (Gastown, Beads) if source is mounted\n\
if [ -f /opt/gastown-src/go.mod ] || [ -f /opt/beads-src/go.mod ]; then\n\
  echo "Building vendored tools..."\n\
  build-vendored-tools 2>&1 | head -20 || true\n\
fi\n\
\n\
# Start SSH\n\
service ssh start\n\
# Start nginx for dashboard\n\
nginx\n\
# Start ttyd web terminal as root (no -u/-g flags)\n\
# All sessions use single tmux socket at /tmp/tmux-0/\n\
ttyd -p 7681 -W -a /usr/local/bin/terminal-launch.sh &\n\
# Start API server as root for consistent tmux socket access\n\
cd /srv/api && TMUX_TMPDIR=/tmp IS_SANDBOX=1 node server.js &\n\
# Keep container running\n\
tail -f /dev/null\n' > /entrypoint.sh && chmod +x /entrypoint.sh

# SSH & Core Access
EXPOSE 22

# Development Servers & Web Terminal
EXPOSE 3000 5000 7681 8000 8080 5500 6000

# Monitoring & Dev Tools
EXPOSE 9000 9090 9100 9200 9300

# Extra/Unused (Future-proofing)
EXPOSE 9400 9500 9600 9700 9800 9900

CMD ["/entrypoint.sh"]