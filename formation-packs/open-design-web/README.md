# OpenDesign Web Prototype workflow pack

An Archon/Formations workflow for turning a mission brief into a real web prototype through explicit direction, implementation, deterministic integrity checks, an independent design jury, bounded refinement, and evidence-rich handoff.

## Use

```bash
archon --workspace /path/to/workspace workflow inspect formation-packs/open-design-web
archon --workspace /path/to/workspace workflow instantiate formation-packs/open-design-web my-site \
  --title "My site" \
  --goal "Design and build ..."
archon --workspace /path/to/workspace board validate my-site
```

Assign agents to every slot before a tmux run. The jury is an orchestrated formation: one controller plus independent critic, brand, accessibility, and copy workers. Enable curated script gates with `CHROTE_FORMATIONS_SCRIPT_GATES=1` when running the board.

The builder and refiner must return both `ref` and `artifactRef` for the prototype output. The human direction gate deliberately blocks until approved or rejected. A failed integrity or scorecard gate routes to the refiner; the refiner returns to integrity before jury review.

## Authority boundaries

- `validate_web.py` owns structural artifact integrity.
- the Formations scorecard evaluator owns reviewer presence, weights, composite score, threshold, and must-fix policy;
- agents provide evidence and per-reviewer scores but cannot self-declare the gate result;
- visual judgment remains evidence-backed rather than pretending static checks measure taste.

## Attribution

The workflow is an original implementation informed by the publicly described OpenDesign multi-agent design workflow pattern. No OpenDesign source code, prompts, or runtime artifacts are vendored. See `NOTICE.md`.
