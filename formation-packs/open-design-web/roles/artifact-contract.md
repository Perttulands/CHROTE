# Routed artifact contract

Use the Formations output contract exactly. For a web artifact, the routed output payload must include:

- `ref`: a workspace-relative path to the primary `index.html` file;
- `artifactRef`: the same workspace-relative path so deterministic gates can inspect it;
- `text`: a concise summary, or for jury output the strict scorecard JSON.

Never point a reference outside the workspace. Do not claim files, screenshots, interactions, or checks you did not create or execute. Keep long evidence in workspace files and route their references.
