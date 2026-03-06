# Performance Backlog Checklist

This checklist converts the performance research into an implementation-ready plan with clear acceptance criteria and verification steps.

## Milestone A: Quick Wins

### [ ] A2. Deduplicate URL → window registration
- Priority: `P0`
- Size: `M`
- Risk: `Medium`
- Dependencies: `A1`
- Tasks:
1. Replace append-only window slices with deduped semantics per URL.
2. Make registration idempotent.
3. Keep unregister and cleanup behavior correct for destroyed windows.
4. Add tests for register/unregister/invalidate behavior.
- Acceptance criteria:
1. Repeated render cycles do not grow windows-per-url indefinitely.
2. Invalidation count scales with actual unique windows only.
3. No regressions in emote/badge rendering updates.
- Verification:
1. Validate windows-per-url metric remains bounded during sustained chat.
2. Open/close popups repeatedly and confirm no stale window references.

### [✓] A3. Replace `time.After` loops in GIF animation
- Priority: `P1`
- Size: `S`
- Risk: `Low`
- Dependencies: none
- Tasks:
1. Replace looped `time.After` usage with reusable timers.
2. Ensure timer stop/drain is correct on cancellation.
3. Confirm no goroutine leaks when popups close.
- Acceptance criteria:
1. Allocation rate decreases during GIF-heavy runs.
2. Goroutine count remains stable over long session.
3. GIF playback timing remains correct.
- Verification:
1. Compare alloc profiles before/after under same GIF load.
2. Run 10+ minute session and verify goroutine steady state.

### [ ] A4. Hot-path logging and keyword lookup cleanup
- Priority: `P1`
- Size: `S`
- Risk: `Low`
- Dependencies: none
- Tasks:
1. Gate high-frequency logs (e.g., per chat message) behind verbosity level.
2. Pre-normalize keyword map for O(1) lowercase lookups.
3. Review render/event loops for unnecessary logging.
- Acceptance criteria:
1. Default log volume is significantly lower under chat load.
2. Keyword matching behavior remains identical.
3. No user-facing loss of important warning/error logs.
- Verification:
1. Load test chat stream and compare log lines/sec before/after.
2. Validate keyword-triggered GIF behavior with mixed-case inputs.

## Milestone B: Core Media Performance

### [ ] B1. Eliminate per-frame emote scaling in render path
- Priority: `P0`
- Size: `L`
- Risk: `Medium`
- Dependencies: `A2`
- Tasks:
1. Add scaled image cache keyed by URL/frame/target-size.
2. Precompute and reuse static asset scaling where practical.
3. For GIFs, cache scaled frame artifacts per frame progression.
4. Add bounded eviction policy for memory control.
- Acceptance criteria:
1. Render path no longer performs naive scaling every frame.
2. CPU and allocs drop materially in emote-heavy scenario.
3. Image quality and sizing behavior remain unchanged.
- Verification:
1. Profile popup rendering under mixed emote traffic before/after.
2. Confirm cache hit ratio and bounded memory behavior.

### [ ] B2. Optimize download/decode pipeline
- Priority: `P1`
- Size: `M`
- Risk: `Medium`
- Dependencies: `A1`
- Tasks:
1. Add decode fast-path using content type and/or magic bytes.
2. Review worker/queue sizing based on measured throughput.
3. Add queue policy improvements for overload cases.
4. Add retry strategy for transient failures where useful.
- Acceptance criteria:
1. Lower decode CPU for non-GIF images.
2. Fewer dropped jobs under burst load.
3. No regressions in supported image formats.
- Verification:
1. Stress with burst downloads and measure drop rate.
2. Validate GIF/static/photo flows end-to-end.

## Milestone C: Latency Resilience

### [ ] C1. Decouple websocket ingest from heavy processing
- Priority: `P0`
- Size: `L`
- Risk: `High`
- Dependencies: `A1`
- Tasks:
1. Introduce bounded worker queues for event handling.
2. Define ordering guarantees across event types.
3. Add backpressure and queue metrics.
4. Ensure clean shutdown/drain behavior.
- Acceptance criteria:
1. Websocket read loop remains responsive during slow handlers.
2. Event ordering guarantees are documented and preserved.
3. p95 event handling latency no longer drives ingest stalls.
- Verification:
1. Simulate slow command/photo handlers and observe ingest continuity.
2. Validate ordering with deterministic event sequence tests.

### [ ] C2. Cache plate command assets and parsing
- Priority: `P1`
- Size: `M`
- Risk: `Medium`
- Dependencies: `C1` recommended
- Tasks:
1. Cache plate template images and marker regions at startup/lazy load.
2. Cache parsed font data and reuse face strategy.
3. Reduce per-invocation disk and parse work.
4. Add command latency metric.
- Acceptance criteria:
1. `!plate` latency significantly reduced.
2. Chat/event handling unaffected by repeated `!plate` usage.
3. Generated output remains visually correct.
- Verification:
1. Measure p50/p95 `!plate` generation latency before/after.
2. Burst `!plate` calls while chat is active and confirm responsiveness.

## Milestone D: Guardrails and Advanced Optimization

### [ ] D1. Overlay redraw optimization pass
- Priority: `P2`
- Size: `M`
- Risk: `Medium`
- Dependencies: `A1`
- Tasks:
1. Profile draw-mode workloads to identify dominant operations.
2. Evaluate partial redraw/dirty-rect approach feasibility.
3. Keep drawing UX and visual output unchanged.
- Acceptance criteria:
1. Lower CPU in active draw mode.
2. No visible artifacts or input lag regressions.
- Verification:
1. Compare draw-mode CPU profile before/after.
2. Manual UX check for stroke quality and responsiveness.

## Release Readiness Criteria

Use this as a go/no-go checklist once `A`, `B`, and `C` are complete.

### [ ] R1. CPU target met
- Under baseline load, average CPU reduced by at least 30%.

### [ ] R2. Allocation/GC target met
- Allocation rate reduced significantly and no GC thrash under GIF-heavy scenarios.

### [ ] R3. Responsiveness target met
- Websocket ingest remains stable with no visible lag spikes during heavy command/media events.

### [ ] R4. Stability target met
- No goroutine growth trend over long session (10+ minutes).

### [ ] R5. Functional parity confirmed
- Popups, GIF animations, badges, photo flow, overlay draw mode, and commands all behave as before.
