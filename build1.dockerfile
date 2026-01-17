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

# 2. Create dev user with sudo access (ensure UID 1000)
RUN userdel -r ubuntu 2>/dev/null || true && \
    useradd -m -u 1000 -s /bin/bash dev && \
    echo 'dev:dev' | chpasswd && \
    echo 'dev ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers

# 3. Setup SSH (enable password auth)
RUN mkdir /var/run/sshd && \
    echo 'root:root' | chpasswd && \
    sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config && \
    sed -i 's/#PasswordAuthentication yes/PasswordAuthentication yes/' /etc/ssh/sshd_config

# 4. Install AI Coding Tools
RUN npm install -g @anthropic-ai/claude-code

# 5. Install Gastown & Beads (Orchestrator tools) & beads_viewer
# Install to /root/go then copy to /usr/local/bin so dev user can access them
ENV GOPATH=/root/go
ENV PATH=$PATH:$GOPATH/bin
# Ensure consistent tmux socket location for all processes (API, ttyd, SSH)
ENV TMUX_TMPDIR=/tmp
RUN go install github.com/steveyegge/beads/cmd/bd@latest
RUN go install github.com/steveyegge/gastown/cmd/gt@latest
# FIX: Move go binaries to global path
RUN cp /root/go/bin/* /usr/local/bin/ 2>/dev/null || true
RUN curl -fsSL "https://raw.githubusercontent.com/Dicklesworthstone/beads_viewer/main/install.sh?$(date +%s)" | bash
# Copy go binaries to public location
RUN cp /root/go/bin/* /usr/local/bin/ 2>/dev/null || true

# 6. Setup directories and permissions
# /code - mounted from E:/Code (RW for dev & root)
# /vault - mounted from E:/Vault (RO for dev, RW for root)
RUN mkdir -p /code && \
    mkdir -p /vault && \
    chown -R dev:dev /home/dev && \
    chown dev:dev /code && \
    chown root:root /vault && \
    chmod 775 /code && \
    chmod 755 /vault

# 6a. Setup minimal tmux config (for root - all Gastown operations run as root)
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

# 6a2. Create tmux session launcher script for ttyd
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

# 6b. Add README to container root
COPY arena-readme.md /README.md

# 6c. Copy API server (owned by dev so it can run as dev)
COPY api /srv/api
RUN cd /srv/api && npm install --production 2>/dev/null || npm install express
RUN chown -R dev:dev /srv/api

# 6d. Copy nginx config and dashboard
COPY nginx/nginx.conf /etc/nginx/nginx.conf
COPY dashboard/dist /usr/share/nginx/html

WORKDIR /code

# 7. Entrypoint script - simplified for root-only operation
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