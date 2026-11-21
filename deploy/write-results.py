#!/usr/bin/env python3
"""Serialize benchmark stage timings and metadata to JSON and CSV."""

from __future__ import annotations

import argparse
import csv
import json
import os
from datetime import datetime, timedelta, timezone
from pathlib import Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--level', required=True, help='Benchmark level label (e.g. L0, L1, L2)')
    parser.add_argument('--stages', nargs='+', required=True, help='Ordered timed stage names')
    return parser.parse_args()


def read_text_if_exists(path: Path) -> str | None:
    if not path.exists():
        return None
    value = path.read_text().strip()
    return value or None


def main() -> None:
    args = parse_args()
    result_dir = Path(os.environ['RESULT_DIR'])

    stages: dict[str, int] = {}
    stages_path = result_dir / 'stages.tsv'
    if stages_path.exists():
        for line in stages_path.read_text().splitlines():
            if not line.strip():
                continue
            name, duration_ns = line.split('\t', 1)
            stages[name] = int(duration_ns)

    start_ns = int((result_dir / 'start').read_text().strip())
    end_path = result_dir / 'end'
    total_ns = (int(end_path.read_text().strip()) - start_ns) if end_path.exists() else None
    total_seconds = (total_ns / 1e9) if total_ns is not None else None

    started_at_utc = read_text_if_exists(result_dir / 'started-at')
    finished_at_utc: str | None = None
    if started_at_utc is not None and total_ns is not None:
        started_dt = datetime.fromisoformat(started_at_utc.replace('Z', '+00:00'))
        finished_dt = started_dt + timedelta(microseconds=round(total_ns / 1000))
        finished_at_utc = finished_dt.astimezone(timezone.utc).strftime('%Y-%m-%dT%H:%M:%S.%fZ')

    result: dict[str, object] = {
        'level': args.level,
        'component': os.environ['COMPONENT'],
        'commit_sha': os.environ['GITHUB_SHA'],
        'run_id': os.environ['GITHUB_RUN_ID'],
        'run_attempt': int(os.environ['GITHUB_RUN_ATTEMPT']),
        'runner': os.environ['RUNNER_NAME'],
        'started_at_utc': started_at_utc,
        'finished_at_utc': finished_at_utc,
        'image': os.environ['IMAGE'],
        'image_digest': read_text_if_exists(result_dir / 'image-digest'),
    }

    syft_version_path = result_dir / 'syft-version.json'
    if syft_version_path.exists():
        result['syft_version'] = json.loads(syft_version_path.read_text())['version']

    trivy_version_path = result_dir / 'trivy-version.json'
    if trivy_version_path.exists():
        trivy_info = json.loads(trivy_version_path.read_text())
        result['trivy_version'] = trivy_info.get('Version')
        vuln_db = trivy_info.get('VulnerabilityDB') or {}
        result['trivy_db_updated_at'] = vuln_db.get('UpdatedAt')

    result['status'] = os.environ['BENCHMARK_STATUS']

    for stage in args.stages:
        result[f'{stage}_seconds'] = (stages[stage] / 1e9) if stage in stages else None
    result['total_seconds'] = total_seconds

    (result_dir / 'results.json').write_text(json.dumps(result, indent=2) + '\n')
    with (result_dir / 'results.csv').open('w', newline='') as csv_file:
        writer = csv.DictWriter(csv_file, fieldnames=list(result.keys()))
        writer.writeheader()
        writer.writerow(result)


if __name__ == '__main__':
    main()
