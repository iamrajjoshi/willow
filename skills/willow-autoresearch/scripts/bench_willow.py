#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
import re
import statistics
import subprocess
import sys
import tempfile
import time
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[3]
TRACE_RE = re.compile(r"^\[trace\]\s+(.*?)\s+([0-9.]+)(µs|us|ms|s)$")
MAX_FIXTURE_SIZE = 50
DEFAULT_FIXTURE_SIZE = 12


class CommandError(RuntimeError):
    def __init__(self, cmd: list[str], cwd: Path, result: subprocess.CompletedProcess[str]):
        self.cmd = cmd
        self.cwd = cwd
        self.result = result
        super().__init__(
            f"command failed ({result.returncode}) in {cwd}: {' '.join(cmd)}\n"
            f"stdout:\n{result.stdout}\n"
            f"stderr:\n{result.stderr}"
        )


@dataclass
class Fixture:
    repo_name: str
    willow_base: Path
    default_branch: str
    branches: list[str]
    reference_repo: str | None
    status_files: int


@dataclass
class Scenario:
    name: str
    command: list[str]
    willow_base: Path
    cwd: Path


def main() -> int:
    args = parse_args()
    args.fixture_size = max(1, min(args.fixture_size, MAX_FIXTURE_SIZE))

    output_dir = (REPO_ROOT / args.output_dir).resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    willow_bin = build_willow(output_dir)
    started_at = now_iso()

    with tempfile.TemporaryDirectory(prefix="willow-autoresearch-") as tmp_name:
        tmp = Path(tmp_name)
        empty_base = tmp / "empty-base"
        fixture_base = tmp / "fixture-base"
        empty_base.mkdir(parents=True)
        fixture = prepare_fixture(args.reference_repo, fixture_base, output_dir, args.fixture_size)

        scenarios = benchmark_scenarios(willow_bin, fixture, empty_base)
        scenario_results = [run_scenario(s, args.runs, args.warmups) for s in scenarios]

    total_median_ms = round(sum(s["median_ms"] for s in scenario_results), 3)
    metric = {
        "name": "total_median_ms",
        "total_median_ms": total_median_ms,
        "value": total_median_ms,
        "unit": "ms",
        "lower_is_better": True,
    }

    fixture_key = fixture_identity(fixture)
    history_path = output_dir / "runs.jsonl"
    baseline_metric, previous_best = history_metrics(history_path, fixture_key)
    status, best_metric, confidence = classify_result(
        total_median_ms,
        baseline_metric,
        previous_best,
        scenario_results,
        args.min_improvement_pct,
        args.runs,
        args.min_accept_runs,
    )

    result = {
        "run": started_at,
        "commit": git_commit(),
        "status": status,
        "fixture_key": fixture_key,
        "metric": metric,
        "baseline_metric": baseline_metric if baseline_metric is not None else total_median_ms,
        "best_metric": best_metric,
        "confidence": confidence,
        "reference_repo": fixture.reference_repo,
        "fixture_summary": {
            "repo_name": fixture.repo_name,
            "default_branch": fixture.default_branch,
            "branches": fixture.branches,
            "branch_count": len(fixture.branches),
            "status_files": fixture.status_files,
        },
        "scenario_results": scenario_results,
        "trace_hotspots": trace_hotspots(scenario_results),
    }

    append_jsonl(history_path, result)
    latest_path = output_dir / "latest.json"
    latest_path.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")

    if args.json:
        print(json.dumps(result, indent=2, sort_keys=True))
    else:
        print(f"status: {status}")
        print(f"total_median_ms: {total_median_ms:.3f}")
        print(f"best_metric: {best_metric:.3f}")
        for hotspot in result["trace_hotspots"][:5]:
            print(f"{hotspot['median_ms']:.3f}ms  {hotspot['scenario']}  {hotspot['label']}")

    return 0


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Benchmark Willow CLI latency with isolated fixtures and trace parsing."
    )
    parser.add_argument(
        "--reference-repo",
        help="Local git repo path, existing Willow repo name, or git URL used as a read-only fixture source.",
    )
    parser.add_argument(
        "--fixture-size",
        type=int,
        default=DEFAULT_FIXTURE_SIZE,
        help=f"Maximum number of branches/worktrees to mirror into the fixture (default: {DEFAULT_FIXTURE_SIZE}, max: {MAX_FIXTURE_SIZE}).",
    )
    parser.add_argument("--runs", type=int, default=7, help="Measured runs per scenario.")
    parser.add_argument("--warmups", type=int, default=2, help="Warmup runs per scenario.")
    parser.add_argument(
        "--output-dir",
        default=".autoresearch",
        help="Directory for the built binary and JSON/JSONL benchmark results.",
    )
    parser.add_argument(
        "--min-improvement-pct",
        type=float,
        default=1.0,
        help="Percent improvement over the previous best required for status=keep.",
    )
    parser.add_argument(
        "--min-accept-runs",
        type=int,
        default=3,
        help="Minimum measured runs required before a result can become an accepted baseline or keep.",
    )
    parser.add_argument("--json", action="store_true", help="Print the full result as JSON.")
    args = parser.parse_args()

    if args.runs < 1:
        parser.error("--runs must be at least 1")
    if args.warmups < 0:
        parser.error("--warmups must be non-negative")
    if args.min_accept_runs < 1:
        parser.error("--min-accept-runs must be at least 1")
    return args


def build_willow(output_dir: Path) -> Path:
    bin_dir = output_dir / "bin"
    bin_dir.mkdir(parents=True, exist_ok=True)
    willow_bin = bin_dir / "willow"
    run_checked(["go", "build", "-o", str(willow_bin), "./cmd/willow"], REPO_ROOT)
    return willow_bin


def prepare_fixture(
    reference_repo: str | None,
    willow_base: Path,
    output_dir: Path,
    fixture_size: int,
) -> Fixture:
    willow_base.mkdir(parents=True, exist_ok=True)
    for child in ("repos", "worktrees", "status", "trash"):
        (willow_base / child).mkdir(parents=True, exist_ok=True)

    if reference_repo:
        source_path, label = resolve_reference_repo(reference_repo, output_dir)
        return prepare_reference_fixture(source_path, label, willow_base, fixture_size)
    return prepare_synthetic_fixture(willow_base, fixture_size)


def resolve_reference_repo(reference_repo: str, output_dir: Path) -> tuple[Path, str]:
    expanded = Path(reference_repo).expanduser()
    if expanded.exists():
        return expanded.resolve(), str(expanded.resolve())

    if looks_like_git_url(reference_repo):
        cache_dir = output_dir / "reference-cache"
        cache_dir.mkdir(parents=True, exist_ok=True)
        cached = cache_dir / (safe_name(reference_repo) + ".git")
        if not cached.exists():
            run_checked(["git", "clone", "--bare", "--quiet", reference_repo, str(cached)], REPO_ROOT)
        return cached.resolve(), reference_repo

    willow_repo = user_willow_home() / "repos" / f"{reference_repo}.git"
    if willow_repo.exists():
        return willow_repo.resolve(), reference_repo

    raise SystemExit(
        f"reference repo not found: {reference_repo}\n"
        "Pass a local path, git URL, or existing Willow repo name."
    )


def looks_like_git_url(value: str) -> bool:
    return (
        "://" in value
        or value.startswith("git@")
        or value.startswith("ssh://")
        or value.endswith(".git")
    )


def user_willow_home() -> Path:
    env_base = os.environ.get("WILLOW_BASE_DIR")
    if env_base:
        return Path(env_base).expanduser()

    config_path = Path.home() / ".config" / "willow" / "config.json"
    try:
        data = json.loads(config_path.read_text())
    except (OSError, json.JSONDecodeError):
        return Path.home() / ".willow"

    base_dir = data.get("baseDir")
    if isinstance(base_dir, str) and base_dir.strip():
        return Path(base_dir).expanduser()
    return Path.home() / ".willow"


def prepare_reference_fixture(source_path: Path, label: str, willow_base: Path, fixture_size: int) -> Fixture:
    repo_name = safe_name(source_path.stem if source_path.suffix == ".git" else source_path.name)
    if not repo_name:
        repo_name = "reference"
    bare_dir = willow_base / "repos" / f"{repo_name}.git"
    run_checked(["git", "clone", "--bare", "--no-local", "--quiet", str(source_path), str(bare_dir)], REPO_ROOT)

    branches = git_lines(bare_dir, "for-each-ref", "--format=%(refname:short)", "refs/heads")
    if not branches:
        raise SystemExit(f"reference repo has no local branches: {label}")
    default_branch = choose_default_branch(bare_dir, branches)
    selected = select_branches(branches, default_branch, fixture_size)

    write_local_config(bare_dir, default_branch)
    copy_stack_metadata(source_path, bare_dir, selected)
    worktree_dirs = add_worktrees(bare_dir, willow_base, repo_name, selected)
    status_files = write_status_files(willow_base, repo_name, worktree_dirs)

    return Fixture(
        repo_name=repo_name,
        willow_base=willow_base,
        default_branch=default_branch,
        branches=selected,
        reference_repo=label,
        status_files=status_files,
    )


def prepare_synthetic_fixture(willow_base: Path, fixture_size: int) -> Fixture:
    source = willow_base.parent / "synthetic-source"
    source.mkdir(parents=True)
    run_checked(["git", "init", "--quiet"], source)
    run_checked(["git", "config", "user.email", "willow-bench@example.com"], source)
    run_checked(["git", "config", "user.name", "Willow Bench"], source)
    run_checked(["git", "checkout", "-B", "main"], source)
    (source / "README.md").write_text("# synthetic willow benchmark repo\n")
    run_checked(["git", "add", "README.md"], source)
    run_checked(["git", "commit", "--quiet", "-m", "initial"], source)

    branches = ["main"]
    parent = "main"
    for i in range(1, fixture_size):
        branch = f"bench/feature-{i:02d}"
        run_checked(["git", "checkout", "--quiet", parent], source)
        run_checked(["git", "checkout", "--quiet", "-b", branch], source)
        path = source / "features" / f"{i:02d}.txt"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(f"feature {i}\n")
        run_checked(["git", "add", str(path.relative_to(source))], source)
        run_checked(["git", "commit", "--quiet", "-m", f"feature {i:02d}"], source)
        branches.append(branch)
        parent = branch

    repo_name = "synthetic"
    bare_dir = willow_base / "repos" / f"{repo_name}.git"
    run_checked(["git", "clone", "--bare", "--quiet", str(source), str(bare_dir)], REPO_ROOT)
    write_local_config(bare_dir, "main")
    write_synthetic_stack(bare_dir, branches)
    worktree_dirs = add_worktrees(bare_dir, willow_base, repo_name, branches)
    status_files = write_status_files(willow_base, repo_name, worktree_dirs)

    return Fixture(
        repo_name=repo_name,
        willow_base=willow_base,
        default_branch="main",
        branches=branches,
        reference_repo=None,
        status_files=status_files,
    )


def choose_default_branch(bare_dir: Path, branches: list[str]) -> str:
    result = run_capture(["git", "symbolic-ref", "--quiet", "--short", "HEAD"], bare_dir)
    candidate = result.stdout.strip()
    if result.returncode == 0 and candidate in branches:
        return candidate
    for name in ("main", "master", "trunk", "develop"):
        if name in branches:
            return name
    return sorted(branches)[0]


def select_branches(branches: list[str], default_branch: str, fixture_size: int) -> list[str]:
    selected = [default_branch]
    for branch in sorted(b for b in branches if b != default_branch):
        selected.append(branch)
        if len(selected) >= fixture_size:
            break
    return selected


def add_worktrees(
    bare_dir: Path,
    willow_base: Path,
    repo_name: str,
    branches: list[str],
) -> dict[str, Path]:
    dirs: dict[str, Path] = {}
    used: set[str] = set()
    for branch in branches:
        dirname = unique_worktree_dir(branch, used)
        path = willow_base / "worktrees" / repo_name / dirname
        path.parent.mkdir(parents=True, exist_ok=True)
        run_checked(["git", "-C", str(bare_dir), "worktree", "add", "--force", str(path), branch], REPO_ROOT)
        dirs[branch] = path
    return dirs


def unique_worktree_dir(branch: str, used: set[str]) -> str:
    base = safe_name(branch.replace("/", "-"))
    if not base:
        base = "worktree"
    name = base
    suffix = 2
    while name in used:
        name = f"{base}-{suffix}"
        suffix += 1
    used.add(name)
    return name


def write_status_files(willow_base: Path, repo_name: str, worktree_dirs: dict[str, Path]) -> int:
    statuses = ["BUSY", "DONE", "WAIT", "IDLE", "DONE"]
    base_time = datetime.now(timezone.utc).replace(microsecond=0)
    count = 0
    for i, (branch, wt_path) in enumerate(worktree_dirs.items()):
        wt_dir = wt_path.name
        status_dir = willow_base / "status" / repo_name / wt_dir
        status_dir.mkdir(parents=True, exist_ok=True)
        status = statuses[i % len(statuses)]
        timestamp = base_time - timedelta(seconds=i * 17)
        session_id = f"bench-{i:03d}"
        payload = {
            "status": status,
            "session_id": session_id,
            "tool": "bench",
            "tool_count": i + 1,
            "timestamp": timestamp.isoformat().replace("+00:00", "Z"),
            "start_time": (timestamp - timedelta(minutes=3)).isoformat().replace("+00:00", "Z"),
            "worktree": branch,
        }
        (status_dir / f"{session_id}.json").write_text(json.dumps(payload, sort_keys=True) + "\n")
        count += 1
    return count


def write_local_config(bare_dir: Path, default_branch: str) -> None:
    payload = {"baseBranch": default_branch, "defaults": {"fetch": False}}
    (bare_dir / "willow.json").write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n")


def copy_stack_metadata(source_path: Path, bare_dir: Path, selected: list[str]) -> None:
    stack_path = find_stack_file(source_path)
    if not stack_path:
        return
    try:
        data = json.loads(stack_path.read_text())
    except (OSError, json.JSONDecodeError):
        return

    parents = data.get("parents") if isinstance(data, dict) else data
    if not isinstance(parents, dict):
        return

    selected_set = set(selected)
    filtered = {
        branch: parent
        for branch, parent in parents.items()
        if branch in selected_set and isinstance(parent, str) and parent
    }
    if filtered:
        (bare_dir / "branches.json").write_text(json.dumps({"parents": filtered}, indent=2, sort_keys=True) + "\n")


def find_stack_file(source_path: Path) -> Path | None:
    direct = source_path / "branches.json"
    if direct.exists():
        return direct

    if not source_path.exists():
        return None

    result = run_capture(["git", "rev-parse", "--git-common-dir"], source_path)
    if result.returncode != 0:
        return None
    common = Path(result.stdout.strip())
    if not common.is_absolute():
        common = source_path / common
    candidate = common / "branches.json"
    if candidate.exists():
        return candidate
    return None


def write_synthetic_stack(bare_dir: Path, branches: list[str]) -> None:
    parents: dict[str, str] = {}
    for i, branch in enumerate(branches[1:], start=1):
        parents[branch] = branches[i - 1]
    if parents:
        (bare_dir / "branches.json").write_text(json.dumps({"parents": parents}, indent=2, sort_keys=True) + "\n")


def benchmark_scenarios(willow_bin: Path, fixture: Fixture, empty_base: Path) -> list[Scenario]:
    return [
        Scenario("empty-ls", [str(willow_bin), "ls"], empty_base, REPO_ROOT),
        Scenario("ls-json", [str(willow_bin), "ls", fixture.repo_name, "--json"], fixture.willow_base, REPO_ROOT),
        Scenario("ls-table", [str(willow_bin), "ls", fixture.repo_name], fixture.willow_base, REPO_ROOT),
        Scenario(
            "status-json",
            [str(willow_bin), "status", "--repo", fixture.repo_name, "--json"],
            fixture.willow_base,
            REPO_ROOT,
        ),
        Scenario(
            "tmux-list",
            [str(willow_bin), "tmux", "list", "--repo", fixture.repo_name],
            fixture.willow_base,
            REPO_ROOT,
        ),
    ]


def run_scenario(scenario: Scenario, runs: int, warmups: int) -> dict:
    for _ in range(warmups):
        run_once(scenario)

    wall_times: list[float] = []
    trace_values: dict[str, list[float]] = {}
    stdout_bytes = 0
    stderr_bytes = 0

    for _ in range(runs):
        result, elapsed_ms = run_once(scenario)
        wall_times.append(elapsed_ms)
        stdout_bytes = max(stdout_bytes, len(result.stdout.encode()))
        stderr_bytes = max(stderr_bytes, len(result.stderr.encode()))
        for label, value in parse_trace(result.stderr):
            trace_values.setdefault(label, []).append(value)

    trace_medians = {
        label: round(statistics.median(values), 3)
        for label, values in sorted(trace_values.items())
        if values
    }

    return {
        "name": scenario.name,
        "command": printable_command(scenario.command),
        "median_ms": round(statistics.median(wall_times), 3),
        "min_ms": round(min(wall_times), 3),
        "max_ms": round(max(wall_times), 3),
        "mad_ms": round(median_absolute_deviation(wall_times), 3),
        "runs": runs,
        "stdout_bytes": stdout_bytes,
        "stderr_bytes": stderr_bytes,
        "trace_medians_ms": trace_medians,
    }


def run_once(scenario: Scenario) -> tuple[subprocess.CompletedProcess[str], float]:
    env = benchmark_env(scenario.willow_base)
    start = time.perf_counter()
    result = run_capture(scenario.command, scenario.cwd, env=env)
    elapsed_ms = (time.perf_counter() - start) * 1000
    if result.returncode != 0:
        raise CommandError(scenario.command, scenario.cwd, result)
    return result, elapsed_ms


def benchmark_env(willow_base: Path) -> dict[str, str]:
    env = os.environ.copy()
    env["WILLOW_BASE_DIR"] = str(willow_base)
    env["WILLOW_TELEMETRY"] = "off"
    env["WILLOW_TRACE"] = "1"
    env["GH_PROMPT_DISABLED"] = "1"
    env["GH_NO_UPDATE_NOTIFIER"] = "1"
    env["NO_COLOR"] = "1"
    env["TERM"] = env.get("TERM") or "dumb"
    env.pop("WILLOW_DIR", None)
    return env


def parse_trace(stderr: str) -> list[tuple[str, float]]:
    spans: list[tuple[str, float]] = []
    for line in stderr.splitlines():
        match = TRACE_RE.match(line.strip())
        if not match:
            continue
        label, raw_value, unit = match.groups()
        value = float(raw_value)
        if unit in ("µs", "us"):
            value /= 1000
        elif unit == "s":
            value *= 1000
        spans.append((label.strip(), value))
    return spans


def trace_hotspots(scenario_results: list[dict]) -> list[dict]:
    hotspots: list[dict] = []
    for scenario in scenario_results:
        for label, value in scenario["trace_medians_ms"].items():
            hotspots.append(
                {
                    "scenario": scenario["name"],
                    "label": label,
                    "median_ms": value,
                }
            )
    return sorted(hotspots, key=lambda item: item["median_ms"], reverse=True)[:15]


def fixture_identity(fixture: Fixture) -> str:
    source = fixture.reference_repo or "synthetic"
    branch_digest = ",".join(fixture.branches)
    return f"{source}|{fixture.default_branch}|{branch_digest}"


def history_metrics(history_path: Path, fixture_key: str) -> tuple[float | None, float | None]:
    if not history_path.exists():
        return None, None

    accepted: list[float] = []
    for line in history_path.read_text().splitlines():
        if not line.strip():
            continue
        try:
            entry = json.loads(line)
        except json.JSONDecodeError:
            continue
        if entry.get("fixture_key") != fixture_key:
            continue
        if entry.get("status") not in {"baseline", "keep"}:
            continue
        metric = entry.get("metric", {})
        value = metric.get("total_median_ms", metric.get("value")) if isinstance(metric, dict) else None
        if isinstance(value, (int, float)):
            accepted.append(float(value))

    if not accepted:
        return None, None
    return accepted[0], min(accepted)


def classify_result(
    total_median_ms: float,
    baseline_metric: float | None,
    previous_best: float | None,
    scenario_results: list[dict],
    min_improvement_pct: float,
    runs: int,
    min_accept_runs: int,
) -> tuple[str, float, dict]:
    max_mad_pct = 0.0
    for scenario in scenario_results:
        median_ms = scenario["median_ms"]
        if median_ms > 0:
            max_mad_pct = max(max_mad_pct, (scenario["mad_ms"] / median_ms) * 100)

    if runs < min_accept_runs:
        improvement_pct = 0.0
        if previous_best is not None:
            improvement_pct = ((previous_best - total_median_ms) / previous_best) * 100
        return (
            "smoke",
            previous_best if previous_best is not None else total_median_ms,
            {
                "label": "smoke",
                "improvement_pct": round(improvement_pct, 3),
                "max_mad_pct": round(max_mad_pct, 3),
                "score": 0.0,
                "accepted": False,
                "reason": f"requires at least {min_accept_runs} runs to accept",
            },
        )

    if previous_best is None:
        return (
            "baseline",
            total_median_ms,
            {
                "label": "baseline",
                "improvement_pct": 0.0,
                "max_mad_pct": round(max_mad_pct, 3),
                "score": 0.0,
                "accepted": True,
            },
        )

    improvement_pct = ((previous_best - total_median_ms) / previous_best) * 100
    score = improvement_pct / max(max_mad_pct, 0.1)
    status = "keep" if improvement_pct >= min_improvement_pct else "discard"
    best_metric = total_median_ms if status == "keep" else previous_best
    if score >= 3:
        label = "high"
    elif score >= 1:
        label = "medium"
    elif improvement_pct > 0:
        label = "low"
    else:
        label = "regression"

    return (
        status,
        best_metric,
        {
            "label": label,
            "improvement_pct": round(improvement_pct, 3),
            "max_mad_pct": round(max_mad_pct, 3),
            "score": round(score, 3),
            "accepted": status == "keep",
        },
    )


def append_jsonl(path: Path, payload: dict) -> None:
    with path.open("a") as f:
        f.write(json.dumps(payload, sort_keys=True) + "\n")


def git_commit() -> str:
    result = run_capture(["git", "rev-parse", "HEAD"], REPO_ROOT)
    if result.returncode != 0:
        return "unknown"
    return result.stdout.strip()


def git_lines(cwd: Path, *args: str) -> list[str]:
    result = run_checked(["git", *args], cwd)
    return [line.strip() for line in result.stdout.splitlines() if line.strip()]


def run_checked(cmd: list[str], cwd: Path, env: dict[str, str] | None = None) -> subprocess.CompletedProcess[str]:
    result = run_capture(cmd, cwd, env=env)
    if result.returncode != 0:
        raise CommandError(cmd, cwd, result)
    return result


def run_capture(
    cmd: list[str],
    cwd: Path,
    env: dict[str, str] | None = None,
) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        cmd,
        cwd=str(cwd),
        env=env,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )


def median_absolute_deviation(values: list[float]) -> float:
    median = statistics.median(values)
    return statistics.median([abs(value - median) for value in values])


def printable_command(command: list[str]) -> list[str]:
    out = []
    for part in command:
        path = Path(part)
        if path.name == "willow" and path.exists():
            out.append("willow")
        else:
            out.append(part)
    return out


def safe_name(value: str) -> str:
    value = re.sub(r"[^A-Za-z0-9._-]+", "-", value.strip())
    value = value.strip(".-")
    return value[:80]


def now_iso() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except CommandError as err:
        print(err, file=sys.stderr)
        raise SystemExit(1)
