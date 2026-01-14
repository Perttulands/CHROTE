FROM ubuntu:24.04

# 1. Core Tools & SSH & Healthcheck Utils + Locales for UTF-8/emoji support
RUN apt-get update && apt-get install -y \
    curl git tmux golang-go nodejs npm python3 python3-pip \
    sudo build-essential openssh-server netcat-openbsd ttyd nginx \
    locales \
    && rm -rf /var/lib/apt/lists/* \
    && locale-gen en_US.UTF-8

# Set UTF-8 locale environment
ENV LANG=en_US.UTF-8
ENV LC_ALL=en_US.UTF-8
ENV LANGUAGE=en_US:en

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

# 6a. Setup tmux config with mouse support and UTF-8
RUN printf '# Enable mouse support (tmux 3.x handles scroll automatically)\n\
set -g mouse on\n\
\n\
# UTF-8 support for emojis and special characters\n\
set -gq utf8 on\n\
set -gq status-utf8 on\n\
setw -gq utf8 on\n\
\n\
# Better scrollback\n\
set -g history-limit 50000\n\
\n\
# Start windows and panes at 1, not 0\n\
set -g base-index 1\n\
setw -g pane-base-index 1\n\
\n\
# Renumber windows when one is closed\n\
set -g renumber-windows on\n\
\n\
# Better colors and true color support\n\
set -g default-terminal "tmux-256color"\n\
set -ga terminal-overrides ",xterm-256color:Tc"\n\
' > /home/dev/.tmux.conf && chown dev:dev /home/dev/.tmux.conf

# 6a2. Create tmux session launcher script for ttyd
RUN printf '#!/bin/bash\n\
# Terminal launcher script for ttyd\n\
# Usage: terminal-launch.sh [session_name] [mode]\n\
# mode: tmux (default) or shell\n\
# Setup dev user environment (ttyd runs with -u/-g but no login shell)\n\
export HOME=/home/dev\n\
export USER=dev\n\
export PATH=/usr/local/bin:/usr/bin:/bin:$HOME/.local/bin:$HOME/go/bin\n\
export TMUX_TMPDIR=/tmp\n\
# UTF-8 locale for emoji and special character support\n\
export LANG=en_US.UTF-8\n\
export LC_ALL=en_US.UTF-8\n\
cd "$HOME" 2>/dev/null || cd /code\n\
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

# 7. Entrypoint script to fix permissions on mounted volumes
RUN printf '#!/bin/bash\n\
# Fix /vault permissions: root can write, dev can only read\n\
chown -R root:root /vault 2>/dev/null || true\n\
chmod -R 755 /vault 2>/dev/null || true\n\
find /vault -type f -exec chmod 644 {} \\; 2>/dev/null || true\n\
# Fix /code permissions: both can read/write\n\
chown -R dev:dev /code 2>/dev/null || true\n\
chmod -R 775 /code 2>/dev/null || true\n\
# Fix /home/dev permissions for terminal launch\n\
chown -R dev:dev /home/dev 2>/dev/null || true\n\
chmod 755 /home/dev 2>/dev/null || true\n\
# Start SSH\n\
service ssh start\n\
# Start nginx for dashboard\n\
nginx\n\
# Start ttyd web terminal with URL arg support for session switching\n\
# Usage: /terminal/?arg=session_name to attach to a tmux session\n\
ttyd -p 7681 -W -a -u 1000 -g 1000 /usr/local/bin/terminal-launch.sh &\n\
# Start API server for dashboard as dev user (TMUX_TMPDIR ensures consistent socket path)\n\
su - dev -c "export TMUX_TMPDIR=/tmp && cd /srv/api && node server.js" &\n\
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