# Legacy Ideas

This is the only CHROTE idea graveyard. It is deliberately non-current: it is
not product authority, a roadmap, a backlog, or permission to implement
anything. Revival requires a scoped Bead and a current product decision.

## Ideas worth remembering

- **Declarative topology:** session roles and relationships can be useful
  visualization metadata. Agent harnesses should continue to own prompts, IPC,
  lifecycle, and stop hooks.
- **Attention surface:** a status-only mobile, chat, or voice surface could
  summarize work and surface explicit approvals. A command channel needs its own
  concrete trust and audit contract first.
- **Markdown preview:** view-only rendered Markdown could improve file inspection
  without turning Files into an IDE.
- **Fail-loud Files states:** unavailable roots, permission errors, and partial
  reads should remain distinct instead of looking like empty directories.
- **Broader scheduling:** shell jobs, mail, events, retry policy, and richer
  history were explored. Current Scheduled deliberately sends literal prompts to
  tmux sessions only.
- **Shell-death diagnosis:** when a harness exits but tmux remains, inspect the
  process tree, inherited descriptors, terminal environment, and native harness
  state before blaming tmux. This belongs in an operator skill, not CHROTE code.

## Durable historical pointers

- `enterprise-substrate/run-view-projection` preserves a sanitizing multi-user
  run projection experiment.
- `enterprise-substrate/workspace-authority` preserves a workspace authority
  writer experiment.

Those annotated tags are archaeology for a future product decision, not seams
that CHROTE promises to retain. Experimental Formations and Archon work now
lives in
[chrote-agent-formations](https://github.com/Perttulands/chrote-agent-formations).
