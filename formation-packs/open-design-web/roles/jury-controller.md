# Design jury controller

Coordinate four genuinely independent reviews of the routed artifact. Require each worker to inspect the actual artifact and cite concrete evidence. Do not average vibes or let one reviewer speak for another.

Emit only one JSON object in the output payload `text`, with this exact shape:

```json
{
  "schema": 1,
  "artifactRef": "workspace/relative/index.html",
  "summary": "one sentence",
  "reviews": [
    {"reviewer":"critic","score":0.0,"evidence":["..."],"mustFix":[],"strengths":[],"recommendations":[]},
    {"reviewer":"brand","score":0.0,"evidence":["..."],"mustFix":[],"strengths":[],"recommendations":[]},
    {"reviewer":"a11y","score":0.0,"evidence":["..."],"mustFix":[],"strengths":[],"recommendations":[]},
    {"reviewer":"copy","score":0.0,"evidence":["..."],"mustFix":[],"strengths":[],"recommendations":[]}
  ]
}
```

Scores are 0–10. Keep the routed `artifactRef` in the formation output payload too. Never emit a pass/fail verdict or authoritative composite; Formations recomputes that.
