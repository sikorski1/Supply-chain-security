# Running Repeated CI/CD Benchmark Experiments (L0–L5)

This directory contains the orchestration and analysis scripts used to run
reproducible, interleaved benchmark experiments across security levels L0–L5.

## Prerequisites

1. Commit and push your repository changes to `main` (GitHub Actions runs the pushed commit).
2. Ensure the local benchmark infrastructure is running:
   ```bash
   bash deploy/setup-kind.sh
   bash deploy/setup-gatekeeper.sh   # required when L5 is included
   ```
3. Start the self-hosted GitHub Actions runner (`actions-runner/run.sh`).
4. Stop unrelated containers on the host to avoid CPU/disk jitter during formal measurements.
5. Export a GitHub fine-grained Personal Access Token with **Actions: Read and write** on this repository:
   ```bash
   export GITHUB_TOKEN="github_pat_..."
   ```

## Running an Experiment

Preview the schedule and run pre-flight checks without dispatching anything:

```bash
python3 experiments/scripts/run-benchmark.py --repetitions 10 --dry-run
```

Run an interleaved 10-round experiment (1 untimed `L5` warm-up pass + 10 rotated rounds of `L0`–`L5`, 61 workflow runs total):

```bash
python3 experiments/scripts/run-benchmark.py --repetitions 10
```

Useful options:

* `--repetitions N` (`-n N`) — number of measured repetitions per security level (default: `5`).
* `--levels L0 L1 L2 ...` — subset of security levels to run (default: `L0 L1 L2 L3 L4 L5`).
* `--order rotated|sequential|random` — per-round ordering (default: `rotated` to balance sequence/thermal drift across levels).
* `--warmup-level L5|none` — untimed warm-up pass before Round 1 (default: `L5`, excluded from statistics).
* `--experiment-id <id> --resume` — resume an interrupted experiment from its saved `manifest.json`.
* `--continue-on-failure` — keep running remaining rounds if a single workflow fails.

## Re-running Analysis Standalone

`run-benchmark.py` runs `analyze-results.py` automatically at the end. You can also re-run analysis at any time:

```bash
python3 experiments/scripts/analyze-results.py experiments/results/<experiment-id>
```

Outputs in `experiments/results/<experiment-id>/summary/`:

* `all-runs.csv` — flat tidy dataset of every measured `(round, level, component)` row.
* `pipeline-stats.csv` — full 7-component application pipeline totals (median, mean, std, min, p95, max, overhead vs. L0, incremental delta).
* `component-stats.csv` — per-component statistics across levels.
* `stage-stats.csv` — per-stage statistics summed across all 7 components.
* `summary.md` — Markdown tables ready for review or thesis reporting.
