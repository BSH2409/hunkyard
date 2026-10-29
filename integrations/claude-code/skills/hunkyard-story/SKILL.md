---
name: hunkyard-story
description: Use when the user explicitly requests a local Hunkyard story file for a review. Do not infer a story request from a generic code review.
---

# Local review story

Create the requested story locally from the diff, then run `hunkyard story --story-file FILE --skip-llm` with the same launching owner as the hunkyard skill. Read the hunkyard skill and its shared review protocol for feedback handling. Story content and skill choices are reviewed in the existing host session. Hunkyard does not spawn a model CLI or look up remote PR metadata.
