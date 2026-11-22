#!/usr/bin/env python3
"""Aggregate benchmark results.json artifacts into CSV tables and a Markdown summary.

Computes median, mean, std, min, p95, max, absolute/relative overhead vs. L0,
and incremental deltas vs. the previous security level across components,
stages, and full 7-component application runs.
"""

from __future__ import annotations

import argparse
import csv
import json
import math
import statistics
import sys
from collections import defaultdict
from pathlib import Path

LEVEL_ORDER = ('L0', 'L1', 'L2', 'L3', 'L4', 'L5')
COMPONENT_ORDER = (
    'inventory',
    'payments',
    'orders',
    'api-gateway',
    'notifications',
    'analytics',
    'frontend',
)
STAGE_ORDER = (
    'checkout',
    'install',
    'test',
    'build',
    'push',
    'sbom_source',
    'sbom_image',
    'scan_source',
    'scan_image',
    'sign_image',
    'verify_image',
    'attest_provenance',
    'verify_provenance',
    'policy_eval',
    'deploy',
)


def percentile(values: list[float], p: float) -> float:
    """Linear-interpolated percentile (matches numpy/pandas default 'linear')."""
    ordered = sorted(values)
    if len(ordered) == 1:
        return ordered[0]
    rank = (len(ordered) - 1) * (p / 100.0)
    low = math.floor(rank)
    high = math.ceil(rank)
    if low == high:
        return ordered[low]
    return ordered[low] + (ordered[high] - ordered[low]) * (rank - low)


def summarize(values: list[float]) -> dict[str, float | int | None]:
    if not values:
        return {'n': 0, 'median': None, 'mean': None, 'std': None, 'min': None, 'p95': None, 'max': None}
    return {
        'n': len(values),
        'median': round(statistics.median(values), 4),
        'mean': round(statistics.mean(values), 4),
        'std': round(statistics.stdev(values), 4) if len(values) > 1 else 0.0,
        'min': round(min(values), 4),
        'p95': round(percentile(values, 95.0), 4),
        'max': round(max(values), 4),
    }


def load_rows(exp_dir: Path, include_warmup: bool, include_failed: bool) -> tuple[list[dict], dict | None]:
    manifest_path = exp_dir / 'manifest.json'
    manifest = json.loads(manifest_path.read_text()) if manifest_path.exists() else None
    rows: list[dict] = []

    if manifest is not None:
        for step in manifest.get('steps', []):
            phase = step.get('phase', 'measured')
            if phase == 'warmup' and not include_warmup:
                continue
            art_rel = step.get('artifacts_dir')
            if not art_rel:
                continue
            step_dir = exp_dir / art_rel
            if not step_dir.exists():
                continue
            for res_file in sorted(step_dir.rglob('results.json')):
                row = json.loads(res_file.read_text())
                if row.get('status') != 'success' and not include_failed:
                    continue
                row['phase'] = phase
                row['round'] = step.get('round')
                row['position'] = step.get('position')
                rows.append(row)
    else:
        for res_file in sorted(exp_dir.rglob('results.json')):
            row = json.loads(res_file.read_text())
            if row.get('status') != 'success' and not include_failed:
                continue
            row.setdefault('phase', 'measured')
            row.setdefault('round', row.get('run_id'))
            row.setdefault('position', None)
            rows.append(row)

    return rows, manifest


def write_csv(path: Path, fieldnames: list[str], rows: list[dict]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open('w', newline='') as fh:
        writer = csv.DictWriter(fh, fieldnames=fieldnames, extrasaction='ignore')
        writer.writeheader()
        for row in rows:
            writer.writerow(row)


def fmt(val: float | int | None, suffix: str = '') -> str:
    if val is None:
        return '-'
    if isinstance(val, int):
        return f'{val}{suffix}'
    return f'{val:.3f}{suffix}'


def fmt_pct(val: float | None) -> str:
    if val is None:
        return '-'
    return f'{val:+.2f}%'


def fmt_delta(val: float | None) -> str:
    if val is None:
        return '-'
    return f'{val:+.3f}s'


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('experiment_dir', type=Path, help='Experiment directory (e.g. experiments/results/exp-...)')
    parser.add_argument('--include-warmup', action='store_true', help='Include warmup phase rows in statistics')
    parser.add_argument('--include-failed', action='store_true', help='Include failed component rows in all-runs.csv')
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    exp_dir = args.experiment_dir.resolve()
    if not exp_dir.exists():
        sys.exit(f'Directory not found: {exp_dir}')

    rows, manifest = load_rows(exp_dir, args.include_warmup, args.include_failed)
    if not rows:
        sys.exit(f'No matching results.json files found under {exp_dir}')

    summary_dir = exp_dir / 'summary'
    summary_dir.mkdir(parents=True, exist_ok=True)

    # 1. Master tidy CSV (all-runs.csv)
    all_runs_fields = [
        'phase',
        'round',
        'position',
        'level',
        'component',
        'status',
        'commit_sha',
        'run_id',
        'run_attempt',
        'runner',
        'started_at_utc',
        'finished_at_utc',
        'image',
        'image_digest',
        'syft_version',
        'trivy_version',
        'trivy_db_updated_at',
        'cosign_version',
        'opa_version',
        *(f'{s}_seconds' for s in STAGE_ORDER),
        'total_seconds',
    ]
    rows.sort(
        key=lambda r: (
            str(r.get('phase', '')),
            int(r['round']) if str(r.get('round', '')).isdigit() else 0,
            LEVEL_ORDER.index(r['level']) if r.get('level') in LEVEL_ORDER else 99,
            COMPONENT_ORDER.index(r['component']) if r.get('component') in COMPONENT_ORDER else 99,
        )
    )
    write_csv(summary_dir / 'all-runs.csv', all_runs_fields, rows)

    present_levels = [lvl for lvl in LEVEL_ORDER if any(r.get('level') == lvl for r in rows)]
    success_rows = [r for r in rows if r.get('status') == 'success' and r.get('total_seconds') is not None]

    # 2. Per-(level, component) statistics
    comp_buckets: dict[tuple[str, str], list[float]] = defaultdict(list)
    for r in success_rows:
        comp_buckets[(r['level'], r['component'])].append(float(r['total_seconds']))

    comp_medians: dict[tuple[str, str], float] = {}
    comp_stat_rows: list[dict] = []
    for lvl in present_levels:
        for comp in COMPONENT_ORDER:
            vals = comp_buckets.get((lvl, comp), [])
            if not vals:
                continue
            stats = summarize(vals)
            med = stats['median']
            assert isinstance(med, float)
            comp_medians[(lvl, comp)] = med
            base = comp_medians.get(('L0', comp))
            prev_lvl = LEVEL_ORDER[LEVEL_ORDER.index(lvl) - 1] if lvl in LEVEL_ORDER and LEVEL_ORDER.index(lvl) > 0 else None
            prev = comp_medians.get((prev_lvl, comp)) if prev_lvl else None
            abs_vs_l0 = round(med - base, 4) if base is not None else None
            rel_vs_l0 = round(((med - base) / base) * 100.0, 2) if base and base > 0 else None
            delta_prev = round(med - prev, 4) if prev is not None else None
            comp_stat_rows.append({
                'level': lvl,
                'component': comp,
                **stats,
                'abs_overhead_vs_l0_s': abs_vs_l0,
                'rel_overhead_vs_l0_pct': rel_vs_l0,
                'delta_vs_prev_level_s': delta_prev,
            })

    write_csv(
        summary_dir / 'component-stats.csv',
        [
            'level', 'component', 'n', 'median', 'mean', 'std', 'min', 'p95', 'max',
            'abs_overhead_vs_l0_s', 'rel_overhead_vs_l0_pct', 'delta_vs_prev_level_s',
        ],
        comp_stat_rows,
    )

    # 3. Full 7-component application pipeline totals per (level, run_id)
    run_totals: dict[tuple[str, str], dict[str, float]] = defaultdict(dict)
    for r in success_rows:
        run_totals[(r['level'], str(r['run_id']))][r['component']] = float(r['total_seconds'])

    pipeline_buckets: dict[str, list[float]] = defaultdict(list)
    incomplete_runs: list[tuple[str, str, int]] = []
    for (lvl, run_id), comp_map in run_totals.items():
        if len(comp_map) == len(COMPONENT_ORDER):
            pipeline_buckets[lvl].append(sum(comp_map.values()))
        else:
            incomplete_runs.append((lvl, run_id, len(comp_map)))

    pipeline_medians: dict[str, float] = {}
    pipeline_stat_rows: list[dict] = []
    for lvl in present_levels:
        vals = pipeline_buckets.get(lvl, [])
        if not vals:
            continue
        stats = summarize(vals)
        med = stats['median']
        assert isinstance(med, float)
        pipeline_medians[lvl] = med
        base = pipeline_medians.get('L0')
        prev_lvl = LEVEL_ORDER[LEVEL_ORDER.index(lvl) - 1] if lvl in LEVEL_ORDER and LEVEL_ORDER.index(lvl) > 0 else None
        prev = pipeline_medians.get(prev_lvl) if prev_lvl else None
        abs_vs_l0 = round(med - base, 4) if base is not None else None
        rel_vs_l0 = round(((med - base) / base) * 100.0, 2) if base and base > 0 else None
        delta_prev = round(med - prev, 4) if prev is not None else None
        pipeline_stat_rows.append({
            'level': lvl,
            **stats,
            'abs_overhead_vs_l0_s': abs_vs_l0,
            'rel_overhead_vs_l0_pct': rel_vs_l0,
            'delta_vs_prev_level_s': delta_prev,
        })

    write_csv(
        summary_dir / 'pipeline-stats.csv',
        [
            'level', 'n', 'median', 'mean', 'std', 'min', 'p95', 'max',
            'abs_overhead_vs_l0_s', 'rel_overhead_vs_l0_pct', 'delta_vs_prev_level_s',
        ],
        pipeline_stat_rows,
    )

    # 4. Per-(level, stage) statistics (summed across 7 components per complete run)
    run_stage_totals: dict[tuple[str, str, str], list[float]] = defaultdict(list)
    for r in success_rows:
        lvl = r['level']
        run_id = str(r['run_id'])
        if len(run_totals[(lvl, run_id)]) != len(COMPONENT_ORDER):
            continue
        for stage in STAGE_ORDER:
            val = r.get(f'{stage}_seconds')
            if val is not None:
                run_stage_totals[(lvl, run_id, stage)].append(float(val))

    stage_buckets: dict[tuple[str, str], list[float]] = defaultdict(list)
    for (lvl, _run_id, stage), comp_vals in run_stage_totals.items():
        if len(comp_vals) == len(COMPONENT_ORDER):
            stage_buckets[(lvl, stage)].append(sum(comp_vals))

    stage_stat_rows: list[dict] = []
    for lvl in present_levels:
        for stage in STAGE_ORDER:
            vals = stage_buckets.get((lvl, stage), [])
            if not vals:
                continue
            stage_stat_rows.append({'level': lvl, 'stage': stage, **summarize(vals)})

    write_csv(
        summary_dir / 'stage-stats.csv',
        ['level', 'stage', 'n', 'median', 'mean', 'std', 'min', 'p95', 'max'],
        stage_stat_rows,
    )

    # 5. Consistency metadata & Markdown report
    commits = sorted({str(r.get('commit_sha')) for r in success_rows if r.get('commit_sha')})
    trivy_dbs = sorted({str(r.get('trivy_db_updated_at')) for r in success_rows if r.get('trivy_db_updated_at')})

    md: list[str] = [
        f'# Benchmark Summary: `{exp_dir.name}`',
        '',
        f'- **Measured component rows:** {len(success_rows)} (warmup excluded: `{not args.include_warmup}`)',
        f'- **Commits observed:** {", ".join(f"`{c[:12]}`" for c in commits) or "-"}',
        f'- **Trivy DB snapshots:** {", ".join(f"`{d}`" for d in trivy_dbs) or "-"}',
    ]
    if manifest and manifest.get('host'):
        h = manifest['host']
        md.append(
            f'- **Host:** `{h.get("hostname")}` | {h.get("cpu_model")} ({h.get("cpu_count")} cores, {h.get("mem_total_gib")} GiB RAM) | Docker `{h.get("docker_version")}`'
        )
    if incomplete_runs:
        md.append(f'- **Incomplete workflow runs excluded from 7-component totals:** {len(incomplete_runs)}')

    md += [
        '',
        '## Full 7-Component Application Pipeline Totals',
        '',
        '| Level | Runs (n) | Median (s) | Mean (s) | Std (s) | p95 (s) | Min (s) | Max (s) | Overhead vs L0 | Rel vs L0 | Delta vs Prev |',
        '|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|',
    ]
    for r in pipeline_stat_rows:
        md.append(
            f'| {r["level"]} | {r["n"]} | {fmt(r["median"])} | {fmt(r["mean"])} | {fmt(r["std"])} | '
            f'{fmt(r["p95"])} | {fmt(r["min"])} | {fmt(r["max"])} | '
            f'{fmt_delta(r["abs_overhead_vs_l0_s"])} | {fmt_pct(r["rel_overhead_vs_l0_pct"])} | {fmt_delta(r["delta_vs_prev_level_s"])} |'
        )

    md += [
        '',
        '## Per-Stage Medians (Summed Across All 7 Components)',
        '',
        '| Stage | ' + ' | '.join(present_levels) + ' |',
        '|---|' + '|'.join('---:' for _ in present_levels) + '|',
    ]
    stage_med_lookup = {(r['level'], r['stage']): r['median'] for r in stage_stat_rows}
    for stage in STAGE_ORDER:
        if not any((lvl, stage) in stage_med_lookup for lvl in present_levels):
            continue
        cells = [fmt(stage_med_lookup.get((lvl, stage))) for lvl in present_levels]
        md.append(f'| `{stage}` | ' + ' | '.join(cells) + ' |')

    md += [
        '',
        '## Per-Component Medians (Total Seconds)',
        '',
        '| Component | ' + ' | '.join(present_levels) + ' | L5 vs L0 |',
        '|---|' + '|'.join('---:' for _ in present_levels) + '|---:|',
    ]
    for comp in COMPONENT_ORDER:
        cells = [fmt(comp_medians.get((lvl, comp))) for lvl in present_levels]
        l0 = comp_medians.get(('L0', comp))
        l5 = comp_medians.get(('L5', comp))
        delta = f'{l5 - l0:+.3f}s ({((l5 - l0) / l0) * 100:+.1f}%)' if (l0 and l5 is not None) else '-'
        md.append(f'| `{comp}` | ' + ' | '.join(cells) + f' | {delta} |')

    summary_md_path = summary_dir / 'summary.md'
    summary_md_path.write_text('\n'.join(md) + '\n')

    print(f'Wrote aggregated results to {summary_dir}:')
    for fname in ('all-runs.csv', 'pipeline-stats.csv', 'component-stats.csv', 'stage-stats.csv', 'summary.md'):
        print(f'  - {summary_dir / fname}')


if __name__ == '__main__':
    main()
