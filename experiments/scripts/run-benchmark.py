#!/usr/bin/env python3
"""Orchestrate repeated, interleaved L0–L5 GitHub Actions benchmark runs.

Handles pre-flight host checks, an untimed unified warm-up pass, interleaved
measured rounds with configurable repetitions, artifact downloads, resumable
state tracking, and automatic statistical aggregation.
"""

from __future__ import annotations

import argparse
import io
import json
import os
import platform
import random
import re
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import zipfile
from datetime import datetime, timezone
from pathlib import Path

ALL_LEVELS = ('L0', 'L1', 'L2', 'L3', 'L4', 'L5')
WORKFLOW_FILES = {
    'L0': 'benchmark-l0.yaml',
    'L1': 'benchmark-l1.yaml',
    'L2': 'benchmark-l2.yaml',
    'L3': 'benchmark-l3.yaml',
    'L4': 'benchmark-l4.yaml',
    'L5': 'benchmark-l5.yaml',
}
EXPECTED_COMPONENTS = (
    'inventory',
    'payments',
    'orders',
    'api-gateway',
    'notifications',
    'analytics',
    'frontend',
)
EXPECTED_HOST_CONTAINERS = {'benchmark-infra-registry-1', 'benchmark-control-plane'}


class StripAuthOnCrossHostRedirect(urllib.request.HTTPRedirectHandler):
    """Strip Authorization header when GitHub redirects artifact downloads to Blob Storage."""

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        new_req = super().redirect_request(req, fp, code, msg, headers, newurl)
        if new_req is None:
            return None
        old_host = urllib.parse.urlparse(req.full_url).netloc
        new_host = urllib.parse.urlparse(new_req.full_url).netloc
        if old_host != new_host:
            new_req.remove_header('Authorization')
        return new_req


class GitHubClient:
    def __init__(self, repo: str, token: str) -> None:
        self.repo = repo
        self.token = token
        self.opener = urllib.request.build_opener(StripAuthOnCrossHostRedirect())

    def _request(self, method: str, url: str, payload: dict | None = None) -> tuple[int, bytes]:
        data = json.dumps(payload).encode('utf-8') if payload is not None else None
        headers = {
            'Accept': 'application/vnd.github+json',
            'Authorization': f'Bearer {self.token}',
            'X-GitHub-Api-Version': '2022-11-28',
            'User-Agent': 'supply-chain-benchmark-runner',
        }
        if data is not None:
            headers['Content-Type'] = 'application/json'
        req = urllib.request.Request(url, data=data, headers=headers, method=method)
        try:
            with self.opener.open(req, timeout=60) as resp:
                return resp.status, resp.read()
        except urllib.error.HTTPError as exc:
            body = exc.read().decode('utf-8', errors='replace')
            raise RuntimeError(f'GitHub API {method} {url} failed ({exc.code}): {body}') from exc

    def api_json(self, method: str, path: str, payload: dict | None = None) -> dict:
        url = f'https://api.github.com{path}'
        _, body = self._request(method, url, payload)
        return json.loads(body.decode('utf-8')) if body else {}

    def list_workflow_runs(self, workflow_file: str, event: str | None = 'workflow_dispatch', per_page: int = 10) -> list[dict]:
        query = f'per_page={per_page}'
        if event:
            query += f'&event={urllib.parse.quote(event)}'
        data = self.api_json('GET', f'/repos/{self.repo}/actions/workflows/{workflow_file}/runs?{query}')
        return data.get('workflow_runs', [])

    def any_benchmark_run_active(self) -> list[tuple[str, int, str]]:
        active: list[tuple[str, int, str]] = []
        for level, wf in WORKFLOW_FILES.items():
            for run in self.list_workflow_runs(wf, event=None, per_page=5):
                if run.get('status') in {'queued', 'in_progress', 'waiting', 'pending'}:
                    active.append((level, int(run['id']), str(run['status'])))
        return active

    def dispatch_workflow(self, workflow_file: str, ref: str) -> None:
        self.api_json('POST', f'/repos/{self.repo}/actions/workflows/{workflow_file}/dispatches', {'ref': ref})

    def get_run(self, run_id: int) -> dict:
        return self.api_json('GET', f'/repos/{self.repo}/actions/runs/{run_id}')

    def list_run_artifacts(self, run_id: int) -> list[dict]:
        artifacts: list[dict] = []
        page = 1
        while True:
            data = self.api_json('GET', f'/repos/{self.repo}/actions/runs/{run_id}/artifacts?per_page=100&page={page}')
            batch = data.get('artifacts', [])
            artifacts.extend(batch)
            if len(batch) < 100:
                break
            page += 1
        return artifacts

    def download_artifact_zip(self, artifact_id: int) -> bytes:
        url = f'https://api.github.com/repos/{self.repo}/actions/artifacts/{artifact_id}/zip'
        _, body = self._request('GET', url)
        return body

    def delete_artifact(self, artifact_id: int) -> None:
        self.api_json('DELETE', f'/repos/{self.repo}/actions/artifacts/{artifact_id}')


def run_cmd(cmd: list[str], cwd: Path | None = None, check: bool = True) -> subprocess.CompletedProcess[str]:
    return subprocess.run(cmd, cwd=cwd, text=True, capture_output=True, check=check)


def detect_repo(repo_dir: Path) -> str:
    res = run_cmd(['git', 'remote', 'get-url', 'origin'], cwd=repo_dir, check=False)
    if res.returncode == 0:
        url = res.stdout.strip()
        match = re.search(r'github\.com[:/]([^/]+/[^/.]+?)(?:\.git)?$', url)
        if match:
            return match.group(1)
    return 'sikorski1/Supply-chain-security'


def collect_host_metadata(repo_dir: Path, ref: str) -> dict:
    def safe_out(cmd: list[str]) -> str | None:
        res = run_cmd(cmd, cwd=repo_dir, check=False)
        return res.stdout.strip() if res.returncode == 0 else None

    mem_kb = None
    meminfo = Path('/proc/meminfo')
    if meminfo.exists():
        for line in meminfo.read_text().splitlines():
            if line.startswith('MemTotal:'):
                mem_kb = int(line.split()[1])
                break

    cpu_model = None
    cpuinfo = Path('/proc/cpuinfo')
    if cpuinfo.exists():
        for line in cpuinfo.read_text().splitlines():
            if line.startswith('model name'):
                cpu_model = line.split(':', 1)[1].strip()
                break

    return {
        'hostname': platform.node(),
        'os': platform.platform(),
        'kernel': platform.release(),
        'arch': platform.machine(),
        'cpu_model': cpu_model,
        'cpu_count': os.cpu_count(),
        'mem_total_gib': round(mem_kb / (1024 * 1024), 2) if mem_kb else None,
        'docker_version': safe_out(['docker', 'version', '--format', '{{.Server.Version}}']),
        'kind_version': safe_out(['kind', 'version']),
        'kubectl_version': safe_out(['kubectl', 'version', '--client', '--short'])
        or safe_out(['kubectl', 'version', '--client']),
        'git_commit_sha': safe_out(['git', 'rev-parse', 'HEAD']),
        'git_ref': ref,
    }


def preflight_checks(repo_dir: Path, levels: list[str], ref: str, allow_dirty: bool, dry_run: bool) -> list[str]:
    warnings: list[str] = []
    errors: list[str] = []

    if not dry_run and not os.environ.get('GITHUB_TOKEN'):
        errors.append('GITHUB_TOKEN environment variable is required (fine-grained PAT with Actions: Read & Write).')

    # Local registry check
    reg = run_cmd(['curl', '-fsS', '--max-time', '5', 'http://localhost:5001/v2/'], check=False)
    if reg.returncode != 0:
        errors.append('Local registry http://localhost:5001/v2/ is not reachable. Run: bash deploy/setup-kind.sh')

    # kind cluster check
    node = run_cmd(['kubectl', '--context', 'kind-benchmark', 'get', 'nodes', '--no-headers'], check=False)
    if node.returncode != 0 or ' Ready ' not in f' {node.stdout} ':
        errors.append('kind-benchmark cluster node is not Ready. Run: bash deploy/setup-kind.sh')

    # Gatekeeper check if L5 is included
    if 'L5' in levels:
        gk = run_cmd(
            ['kubectl', '--context', 'kind-benchmark', '-n', 'gatekeeper-system', 'get', 'deployment', 'gatekeeper-controller-manager'],
            check=False,
        )
        if gk.returncode != 0:
            errors.append('OPA Gatekeeper is not installed in kind-benchmark (required for L5). Run: bash deploy/setup-gatekeeper.sh')

    # Self-hosted runner check
    runner = run_cmd(['pgrep', '-f', 'Runner.Listener'], check=False)
    if runner.returncode != 0:
        warnings.append('Local GitHub Actions runner (Runner.Listener) does not appear to be running.')

    # Unrelated Docker containers warning
    ps = run_cmd(['docker', 'ps', '--format', '{{.Names}}\t{{.Status}}'], check=False)
    if ps.returncode == 0:
        for line in ps.stdout.splitlines():
            if not line.strip():
                continue
            name, status = (line.split('\t', 1) + [''])[:2]
            if name not in EXPECTED_HOST_CONTAINERS:
                warnings.append(f'Unrelated container running on benchmark host: {name} ({status}). Consider stopping it to reduce jitter.')

    # Git cleanliness & remote sync check
    status = run_cmd(['git', 'status', '--porcelain'], cwd=repo_dir, check=False)
    if status.stdout.strip():
        msg = 'Working tree has uncommitted changes; GitHub Actions will run the pushed remote commit, not local edits.'
        if allow_dirty:
            warnings.append(msg)
        else:
            errors.append(msg + ' Commit/push first or pass --allow-dirty.')

    head_sha = run_cmd(['git', 'rev-parse', 'HEAD'], cwd=repo_dir, check=False).stdout.strip()
    remote_sha = run_cmd(['git', 'rev-parse', f'origin/{ref}'], cwd=repo_dir, check=False)
    if remote_sha.returncode == 0 and head_sha and remote_sha.stdout.strip() != head_sha:
        msg = f'Local HEAD ({head_sha[:7]}) differs from origin/{ref} ({remote_sha.stdout.strip()[:7]}).'
        if allow_dirty:
            warnings.append(msg)
        else:
            errors.append(msg + ' Push to origin first or pass --allow-dirty.')

    for w in warnings:
        print(f'[WARN] {w}', file=sys.stderr)
    if errors:
        for e in errors:
            print(f'[ERROR] {e}', file=sys.stderr)
        sys.exit(1)
    return warnings


def build_schedule(levels: list[str], repetitions: int, warmup_level: str | None, order: str, seed: int) -> list[dict]:
    schedule: list[dict] = []
    if warmup_level:
        schedule.append({
            'step_index': 0,
            'phase': 'warmup',
            'round': 0,
            'position': 1,
            'level': warmup_level,
            'workflow': WORKFLOW_FILES[warmup_level],
            'status': 'pending',
            'run_id': None,
            'conclusion': None,
        })

    rng = random.Random(seed)
    step_index = len(schedule)
    for round_num in range(1, repetitions + 1):
        if order == 'sequential':
            round_levels = list(levels)
        elif order == 'rotated':
            shift = (round_num - 1) % len(levels)
            round_levels = list(levels[shift:]) + list(levels[:shift])
        elif order == 'random':
            round_levels = list(levels)
            rng.shuffle(round_levels)
        else:
            raise ValueError(f'Unsupported order: {order}')

        for pos, level in enumerate(round_levels, start=1):
            schedule.append({
                'step_index': step_index,
                'phase': 'measured',
                'round': round_num,
                'position': pos,
                'level': level,
                'workflow': WORKFLOW_FILES[level],
                'status': 'pending',
                'run_id': None,
                'conclusion': None,
            })
            step_index += 1
    return schedule


def save_manifest(path: Path, manifest: dict) -> None:
    tmp = path.with_suffix('.json.tmp')
    tmp.write_text(json.dumps(manifest, indent=2) + '\n')
    tmp.replace(path)


def wait_for_idle(client: GitHubClient, poll_interval: int) -> None:
    while True:
        active = client.any_benchmark_run_active()
        if not active:
            return
        desc = ', '.join(f'{lvl}#{rid}({st})' for lvl, rid, st in active)
        print(f'  Waiting for active benchmark run(s) to finish: {desc}', flush=True)
        time.sleep(poll_interval)


def dispatch_and_wait(
    client: GitHubClient,
    workflow_file: str,
    ref: str,
    poll_interval: int,
    timeout_minutes: int,
) -> dict:
    wait_for_idle(client, poll_interval)
    before_ids = {int(r['id']) for r in client.list_workflow_runs(workflow_file, per_page=15)}

    dispatched_at = time.monotonic()
    client.dispatch_workflow(workflow_file, ref)

    run_id: int | None = None
    discover_deadline = dispatched_at + 180
    while time.monotonic() < discover_deadline:
        time.sleep(min(5, poll_interval))
        for run in client.list_workflow_runs(workflow_file, per_page=15):
            rid = int(run['id'])
            if rid not in before_ids:
                run_id = rid
                break
        if run_id is not None:
            break

    if run_id is None:
        raise RuntimeError(f'Timed out waiting for newly dispatched run of {workflow_file} to appear.')

    print(f'  Dispatched {workflow_file} -> run_id={run_id}', flush=True)
    deadline = dispatched_at + timeout_minutes * 60
    last_status = None
    while time.monotonic() < deadline:
        run = client.get_run(run_id)
        status = run.get('status')
        if status != last_status:
            print(f'  run_id={run_id} status={status}', flush=True)
            last_status = status
        if status == 'completed':
            return run
        time.sleep(poll_interval)

    raise RuntimeError(f'Timed out after {timeout_minutes}m waiting for run_id={run_id} ({workflow_file}).')


def download_run_artifacts(
    client: GitHubClient,
    run_id: int,
    target_dir: Path,
    delete_remote: bool = False,
) -> list[str]:
    target_dir.mkdir(parents=True, exist_ok=True)
    artifacts = client.list_run_artifacts(run_id)
    downloaded: list[str] = []
    for art in artifacts:
        if art.get('expired'):
            continue
        art_id = int(art['id'])
        name = art['name']
        art_dir = target_dir / name
        art_dir.mkdir(parents=True, exist_ok=True)
        zip_bytes = client.download_artifact_zip(art_id)
        with zipfile.ZipFile(io.BytesIO(zip_bytes)) as zf:
            members = zf.namelist()
            zf.extractall(art_dir)
        downloaded.append(name)
        if delete_remote and members:
            try:
                client.delete_artifact(art_id)
            except RuntimeError as exc:
                print(f'  [WARN] Could not delete remote artifact {name} (id={art_id}): {exc}', file=sys.stderr)
    return sorted(downloaded)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('-n', '--repetitions', type=int, default=5, help='Number of measured rounds per level (default: 5)')
    parser.add_argument('--levels', nargs='+', choices=ALL_LEVELS, default=list(ALL_LEVELS), help='Security levels to benchmark (default: L0 L1 L2 L3 L4 L5)')
    parser.add_argument('--warmup-level', default='L5', choices=[*ALL_LEVELS, 'none'], help='Single untimed warm-up level before Round 1, or "none" (default: L5)')
    parser.add_argument('--order', choices=('rotated', 'sequential', 'random'), default='rotated', help='Per-round level ordering strategy (default: rotated)')
    parser.add_argument('--seed', type=int, default=42, help='Random seed when --order=random (default: 42)')
    parser.add_argument('--ref', default='main', help='Git branch/ref to dispatch workflows on (default: main)')
    parser.add_argument('--repo', default=None, help='GitHub owner/repo (default: auto-detected from git origin)')
    parser.add_argument('--experiment-id', default=None, help='Experiment folder name (default: exp-<UTC timestamp>)')
    parser.add_argument('--resume', action='store_true', help='Resume an existing experiment directory (requires --experiment-id)')
    parser.add_argument('--output-dir', default='experiments/results', help='Base output directory (default: experiments/results)')
    parser.add_argument('--poll-interval', type=int, default=5, help='GitHub API poll interval in seconds (default: 5)')
    parser.add_argument('--timeout-minutes', type=int, default=45, help='Max wait per workflow run in minutes (default: 45)')
    parser.add_argument('--pause-seconds', type=int, default=1, help='Cooldown pause between workflow runs in seconds (default: 1)')
    parser.add_argument('--continue-on-failure', action='store_true', help='Keep running remaining steps if a workflow run fails')
    parser.add_argument('--delete-remote-artifacts', action='store_true', help='Delete each GitHub Actions artifact after downloading and extracting it locally (prevents hitting the 500 MB free-tier quota on long runs)')
    parser.add_argument('--allow-dirty', action='store_true', help='Allow uncommitted local changes or unpushed HEAD')
    parser.add_argument('--skip-preflight', action='store_true', help='Skip local host/cluster pre-flight checks')
    parser.add_argument('--skip-analyze', action='store_true', help='Do not automatically run analyze-results.py after completion')
    parser.add_argument('--dry-run', action='store_true', help='Print the planned schedule and exit without dispatching workflows')
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    if args.repetitions < 1:
        sys.exit('--repetitions must be >= 1')
    if args.resume and not args.experiment_id:
        sys.exit('--resume requires --experiment-id')

    repo_dir = Path(__file__).resolve().parents[2]
    repo = args.repo or detect_repo(repo_dir)
    warmup_level = None if args.warmup_level == 'none' else args.warmup_level

    warnings: list[str] = []
    if not args.skip_preflight:
        warnings = preflight_checks(repo_dir, args.levels, args.ref, args.allow_dirty, args.dry_run)

    exp_id = args.experiment_id or datetime.now(timezone.utc).strftime('exp-%Y%m%dT%H%M%SZ')
    exp_dir = (repo_dir / args.output_dir / exp_id) if not Path(args.output_dir).is_absolute() else (Path(args.output_dir) / exp_id)
    manifest_path = exp_dir / 'manifest.json'

    if args.resume:
        if not manifest_path.exists():
            sys.exit(f'Cannot resume: manifest not found at {manifest_path}')
        manifest = json.loads(manifest_path.read_text())
        print(f'Resuming experiment {exp_id} from {manifest_path}')
    else:
        schedule = build_schedule(args.levels, args.repetitions, warmup_level, args.order, args.seed)
        manifest = {
            'experiment_id': exp_id,
            'created_at_utc': datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ'),
            'repo': repo,
            'ref': args.ref,
            'levels': args.levels,
            'repetitions': args.repetitions,
            'warmup_level': warmup_level,
            'order': args.order,
            'seed': args.seed if args.order == 'random' else None,
            'delete_remote_artifacts': args.delete_remote_artifacts,
            'host': collect_host_metadata(repo_dir, args.ref),
            'preflight_warnings': warnings,
            'steps': schedule,
        }

    total_steps = len(manifest['steps'])
    print(f'Experiment: {exp_id} | repo={repo}@{args.ref} | levels={manifest["levels"]} | repetitions={manifest["repetitions"]} | order={manifest["order"]} | total_workflow_runs={total_steps}')
    for step in manifest['steps']:
        tag = 'WARMUP' if step['phase'] == 'warmup' else f'R{step["round"]:02d}.{step["position"]}'
        print(f'  [{step["step_index"] + 1:02d}/{total_steps:02d}] {tag:6s} {step["level"]} ({step["workflow"]}) [{step["status"]}]')

    if args.dry_run:
        print('Dry run complete; no workflows dispatched.')
        return

    exp_dir.mkdir(parents=True, exist_ok=True)
    save_manifest(manifest_path, manifest)

    client = GitHubClient(repo, os.environ['GITHUB_TOKEN'])

    for step in manifest['steps']:
        if step['status'] == 'completed':
            continue

        tag = 'warmup' if step['phase'] == 'warmup' else f'round-{step["round"]:02d}-{step["level"].lower()}'
        print(f'\n==> [{step["step_index"] + 1}/{total_steps}] Running {tag} ({step["workflow"]})', flush=True)
        step['status'] = 'running'
        step['dispatched_at_utc'] = datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
        save_manifest(manifest_path, manifest)

        run = dispatch_and_wait(client, step['workflow'], args.ref, args.poll_interval, args.timeout_minutes)
        run_id = int(run['id'])
        conclusion = run.get('conclusion')
        step['run_id'] = run_id
        step['html_url'] = run.get('html_url')
        step['head_sha'] = run.get('head_sha')
        step['run_started_at'] = run.get('run_started_at')
        step['updated_at'] = run.get('updated_at')
        step['conclusion'] = conclusion

        raw_dir = exp_dir / 'raw' / f'{step["step_index"]:03d}-{tag}-run-{run_id}'
        downloaded = download_run_artifacts(client, run_id, raw_dir, delete_remote=args.delete_remote_artifacts)
        step['artifacts_dir'] = str(raw_dir.relative_to(exp_dir))
        step['artifacts'] = downloaded
        step['status'] = 'completed' if conclusion == 'success' else 'failed'
        save_manifest(manifest_path, manifest)

        print(f'  Finished run_id={run_id} conclusion={conclusion} artifacts={len(downloaded)}/{len(EXPECTED_COMPONENTS)}', flush=True)
        if conclusion != 'success':
            print(f'  [ERROR] Workflow run failed: {run.get("html_url")}', file=sys.stderr)
            if not args.continue_on_failure:
                print(f'  Stopping early. Fix the issue and resume with: --experiment-id {exp_id} --resume', file=sys.stderr)
                sys.exit(1)

        if args.pause_seconds > 0 and step['step_index'] + 1 < total_steps:
            time.sleep(args.pause_seconds)

    manifest['finished_at_utc'] = datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
    save_manifest(manifest_path, manifest)
    print(f'\nExperiment {exp_id} finished! Manifest: {manifest_path}')

    if not args.skip_analyze:
        analyzer = Path(__file__).resolve().with_name('analyze-results.py')
        subprocess.run([sys.executable, str(analyzer), str(exp_dir)], check=True)


if __name__ == '__main__':
    main()
