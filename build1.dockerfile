FROM ubuntu:24.04

# 1. Core Tools & SSH & Healthcheck Utils
RUN apt-get update && apt-get install -y \
    curl git tmux golang-go nodejs npm python3 python3-pip \
    sudo build-essential openssh-server netcat-openbsd \
    && rm -rf /var/lib/apt/lists/*

# 2. Create dev user with sudo access
RUN useradd -m -s /bin/bash dev && \
    echo 'dev:dev' | chpasswd && \
    echo 'dev ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers

# 3. Setup SSH (enable password auth)
RUN mkdir /var/run/sshd && \
    echo 'root:root' | chpasswd && \
    sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config && \
    sed -i 's/#PasswordAuthentication yes/PasswordAuthentication yes/' /etc/ssh/sshd_config

# 4. Install AI Coding Tools
RUN npm install -g @anthropic-ai/claude-code || true
RUN npm install -g opencode-ai || true

# 5. Install Gastown & Beads (Orchestrator tools) & beads_viewer
ENV GOPATH=/root/go
ENV PATH=$PATH:$GOPATH/bin
RUN go install github.com/steveyegge/beads/cmd/bd@latest || true
RUN go install github.com/steveyegge/gastown/cmd/gt@latest || true
RUN curl -fsSL "https://raw.githubusercontent.com/Dicklesworthstone/beads_viewer/main/install.sh?$(date +%s)" | bash

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
# Start SSH\n\
service ssh start\n\
# Keep container running\n\
tail -f /dev/null\n' > /entrypoint.sh && chmod +x /entrypoint.sh

# SSH & Core Access
EXPOSE 22

# Development Servers
EXPOSE 3000 5000 8000 8080 5500 6000

# Monitoring & Dev Tools
EXPOSE 9000 9090 9100 9200 9300

# Extra/Unused (Future-proofing)
EXPOSE 9400 9500 9600 9700 9800 9900

CMD ["/entrypoint.sh"]