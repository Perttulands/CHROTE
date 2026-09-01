# CHROTE vision

CHROTE makes a host full of command-line agents manageable from one browser.

Its primary user is one trusted operator running many long-lived processes in tmux. The operator should be able to see those sessions, arrange several terminals, start new work, steer an agent, and send information between sessions without fighting raw tmux commands or losing track of the work.

The browser is an access point. tmux still owns the sessions and the processes inside them. Closing CHROTE, changing devices, or restarting the CHROTE service must leave that work alone.

## The experience

Terminals are the center of CHROTE. Terminal tabs hold windows that can show different tmux sessions side by side. Session discovery, creation, attachment, Peek, labels, and Send to Session should feel immediate and dependable.

Files, server status, and settings support that terminal work. The interface should be polished enough that running agents through tmux feels simple on a desktop and remains useful from a phone or tablet. Tailscale or another private network lets the operator reach the same host and sessions from anywhere.

CHROTE is neutral about what runs in a terminal. Codex, Claude Code, another CLI agent, a build, or a shell command all work because CHROTE attaches to tmux rather than requiring a special agent runtime.

## Product shape

The core is the terminal workspace, session access, files, server status, and settings. Beads, scheduled prompts, service integrations, and agent formations are first-party components that can extend that core.

This modular shape is for keeping responsibilities clear. CHROTE is not a plugin marketplace, and components do not get to redefine session ownership or make the terminal core depend on them.

## What should remain true

- One trusted operator can understand and steer a large set of tmux sessions.
- The host's real resources remain authoritative. CHROTE does not keep shadow copies of sessions, files, or work state.
- Existing tmux work survives browser disconnects and CHROTE restarts.
- Broad host access is intentional inside the operator's private network and Unix permission boundary.
- Desktop offers full workspace control. Smaller devices preserve the essential loop of observing and steering agents.
