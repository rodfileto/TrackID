"""Run an OWL DL reasoner over the vendored ontologies to assert consistency.

Dev/CI tool, not runtime: requires Java + ROBOT (robot.jar, see
scripts/fetch_robot.sh). Merges every vendored Turtle file — BFO, CCO, and any
`tid:` domain files under vendor/tid/ — and runs a DL reasoner (HermiT) over
the merged ontology, reporting unsatisfiable classes and the OWL profile.

A domain TTL dropped into vendor/tid/ is picked up automatically, which makes
this the regression guard for domain additions: a logically inconsistent
addition (e.g. a class that is both Continuant and Occurrent) is caught here.
Run with `python -m app.ontology.reasoner`.
"""
from __future__ import annotations

import hashlib
import os
import subprocess
import sys
import tempfile
from dataclasses import dataclass, field
from pathlib import Path

from app.ontology.importer import VENDOR_DIR, ontology_files

_BACKEND_DIR = Path(__file__).resolve().parents[2]
ROBOT_JAR = Path(os.environ["ROBOT_JAR"]) if "ROBOT_JAR" in os.environ else _BACKEND_DIR / "tools" / "robot.jar"

# HermiT is OWL 2 DL; required because BFO uses complementOf, which EL
# reasoners cannot handle. JFact is the fallback.
REASONER = os.environ.get("ONTOLOGY_REASONER", "HermiT")


@dataclass
class ConsistencyReport:
    consistent: bool
    profile: str | None = None
    unsatisfiable: list[str] = field(default_factory=list)
    details: str = ""


def _run(command: list[str], check: bool = True) -> subprocess.CompletedProcess:
    result = subprocess.run(command, capture_output=True, text=True)
    if check and result.returncode != 0:
        raise RuntimeError(
            f"ROBOT command failed ({' '.join(command[:4])} ...): {result.stderr.strip()}"
        )
    return result


def _check_profile(merged: Path) -> str:
    result = _run(
        [
            "java", "-jar", str(ROBOT_JAR), "validate-profile",
            "--input", str(merged), "--profile", "DL",
        ],
        check=False,
    )
    return "OWL 2 DL" if result.returncode == 0 else "not OWL 2 DL"


def _clean(output: str) -> str:
    """Drop JVM startup noise (sun.misc.Unsafe deprecation warnings)."""
    return "\n".join(
        line for line in output.splitlines()
        if line.strip() and not line.startswith("WARNING:")
    )


def _parse_unsatisfiable(stdout: str) -> list[str]:
    marker = "unsatisfiable: "
    return [
        line.split(marker, 1)[1].strip()
        for line in stdout.splitlines()
        if marker in line
    ]


def _merge(files: list[Path]) -> Path:
    """ROBOT-merge the inputs into a cached OWL file, reusing it when unchanged.

    ROBOT's merge over BFO + all of CCO is slow (~100s), so the merged artifact
    is cached in the system temp dir keyed by input path + mtime + size. Adding
    or editing a `tid:` file invalidates the cache and triggers a re-merge.
    """
    cache_dir = Path(tempfile.gettempdir()) / "trackid-ontology-cache"
    cache_dir.mkdir(parents=True, exist_ok=True)
    digest = hashlib.sha256()
    for path in files:
        stat = path.stat()
        digest.update(f"{path}:{stat.st_mtime_ns}:{stat.st_size}\n".encode())
    merged = cache_dir / f"{digest.hexdigest()}.owl"
    if merged.exists():
        return merged

    merge_command = ["java", "-jar", str(ROBOT_JAR), "merge", "--output", str(merged)]
    for path in files:
        merge_command += ["--input", str(path)]
    _run(merge_command)
    return merged


def check_consistency(vendor_dir: Path | None = None) -> ConsistencyReport:
    vendor_dir = vendor_dir or VENDOR_DIR
    if not ROBOT_JAR.exists():
        raise FileNotFoundError(
            f"robot.jar not found at {ROBOT_JAR}; run scripts/fetch_robot.sh"
        )

    files = ontology_files(vendor_dir)
    if not files:
        return ConsistencyReport(consistent=True, profile=None, details="no ontology files found")

    merged = _merge(files)

    with tempfile.TemporaryDirectory() as tmp:
        tmpdir = Path(tmp)

        # ROBOT exits non-zero and logs `unsatisfiable: <iri>` (to stdout) when
        # the ontology has unsatisfiable classes; consistent -> exit 0.
        reason_result = _run(
            [
                "java", "-jar", str(ROBOT_JAR), "reason",
                "--input", str(merged),
                "--reasoner", REASONER,
                "--output", str(tmpdir / "reasoned.owl"),
            ],
            check=False,
        )

        unsatisfiable = _parse_unsatisfiable(reason_result.stdout)
        consistent = reason_result.returncode == 0 and not unsatisfiable

        return ConsistencyReport(
            consistent=consistent,
            profile=_check_profile(merged),
            unsatisfiable=unsatisfiable,
            details=_clean(reason_result.stdout.strip() + "\n" + reason_result.stderr.strip()),
        )


def main() -> int:
    report = check_consistency()
    print(f"consistent: {report.consistent}")
    print(f"profile:    {report.profile or 'n/a'}")
    if report.unsatisfiable:
        print(f"unsatisfiable classes ({len(report.unsatisfiable)}):")
        for iri in report.unsatisfiable:
            print(f"  - {iri}")
    else:
        print("unsatisfiable classes: none")
    if report.details:
        print(f"reasoner: {report.details}")
    return 0 if report.consistent else 1
if __name__ == "__main__":
    sys.exit(main())
