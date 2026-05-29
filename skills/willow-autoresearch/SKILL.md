---
name: willow-autoresearch
description: "Autoresearch performance workflow for Willow. Use this skill when the user wants to make Willow faster, benchmark Willow CLI latency, run trace-guided optimization loops, compare performance changes, or use a reference repo as a realistic fixture. The bundled harness measures non-Sentry Willow commands with WILLOW_TELEMETRY=off and WILLOW_TRACE=1."
---

# Willow Autoresearch

Use this skill to improve Willow CLI performance with a tight measure-change-measure loop. Keep the loop empirical: every kept change must have a benchmark result and a correctness check.

## Benchmark First

Run the bundled harness before editing:

```bash
python3 skills/willow-autoresearch/scripts/bench_willow.py --runs 7 --json
```

For realistic repo shape, pass a read-only reference repo:

```bash
python3 skills/willow-autoresearch/scripts/bench_willow.py \
  --reference-repo ~/code/myrepo \
  --runs 7 \
  --json
```

`--reference-repo` accepts:

- a local git worktree or bare repo path
- an existing Willow repo name from the user's Willow base
- a git URL, cloned before timing starts

The harness builds Willow once, creates an isolated temporary `WILLOW_BASE_DIR`, runs with `WILLOW_TELEMETRY=off` and `WILLOW_TRACE=1`, then writes local results under `.autoresearch/`.

## Optimization Loop

1. Establish a baseline on the target fixture.
2. Inspect `trace_hotspots` and pick one bottleneck.
3. Make one focused change.
4. Run `go test ./... -count=1`.
5. Re-run the same benchmark command.
6. Keep the change only when tests pass and `total_median_ms` improves beyond jitter. Otherwise revert the change and record what was learned.

Prefer changes that reduce command work in the non-Sentry path: fewer git subprocesses, less filesystem scanning, cheaper status aggregation, better cache use, or narrower work done before first output.

## Interpreting Results

The primary metric is `metric.total_median_ms` and lower is better. Each result includes:

- `scenario_results`: per-command wall-clock medians and trace span medians
- `trace_hotspots`: largest median trace spans across scenarios
- `baseline_metric` and `best_metric`: accepted historical comparison points
- `confidence`: improvement and jitter estimates
- `status`: `baseline`, `keep`, `discard`, or `smoke`

Use the status as a guide, not a substitute for judgment. Runs below `--min-accept-runs` are smoke checks and never become the accepted baseline or best result. Re-run marginal wins before keeping them.

## Guardrails

- Do not benchmark with Sentry enabled.
- Do not mutate the reference repo or the user's real `~/.willow`.
- Do not accept a speedup that breaks behavior or removes useful trace coverage.
- Keep generated benchmark artifacts in ignored `.autoresearch/`.
