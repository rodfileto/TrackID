"""TrackID's exact matcher for `bulk-match match` (docs/large-gallery-plan.md, Decision 1).

The Go command chooses the batch and writes the work directory; this program compares, exactly:

  * the batch against itself;
  * the batch against every face of earlier batches, streamed shard by shard;
  * the batch against every embedding outside the gallery (traces), read from the work dir;

and, for every face on either side, keeps its neighbours scoring at least the floor, at most k of
them. A face whose k-th neighbour still scores at least the floor hit the cap: it is a hub. Each
direction is searched (batch -> earlier and earlier -> batch), so a batch face crowded out of an
earlier hub's list is still kept from its own side, and the other way round.

Search runs in FP16 on the GPU (FAISS brute force, `knn_gpu`), or on the CPU without one. Every
candidate is then rescored in FP32 from the stored vectors, so stored scores do not carry FP16
rounding. Scores are inner products of unit vectors: cosine similarity, 1 - cosine distance.

Work directory, in (written by Go):
    params.json          {"shards", "floor", "k", "batch", "gpu", "dim"}
    batch_ids.npy        int64 feature_embeddings ids of the batch
    earlier_ids.npy      int64 ids of faces matched in earlier batches
    others_ids.npy       int64 ids of embeddings outside the gallery
    others_vectors.npy   float32 (n, dim)
Out (read by Go):
    pairs_a.npy, pairs_b.npy   int64, a < b
    pairs_score.npy            float32
    hubs_ids.npy, hubs_kth.npy int64, float32
    result.json                {"comparisons", "pairs", "hubs", "seconds", "device"}
Shards (written by bulk-enroll): <name>.ids.npy (int64) with <name>.vectors.npy (float16).
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

import numpy as np

try:  # FAISS is optional: without it, NumPy does the search (slowly; for tests).
    import faiss  # type: ignore
except ImportError:  # pragma: no cover - depends on the environment
    faiss = None

# Queries per search call: bounds the FP32 rescoring buffer (chunk x k x dim floats).
QUERY_CHUNK = 4096


def log(msg: str) -> None:
    print(f"bulk-match: {msg}", file=sys.stderr, flush=True)


class Searcher:
    """Exact top-k by inner product, on one or more GPUs, the CPU, or NumPy."""

    def __init__(self, gpu: str):
        self.devices: list[int] = []
        self.resources = []
        self.name = "numpy"
        if faiss is None:
            log("FAISS not installed: searching with NumPy")
            return
        self.name = "faiss-cpu"
        available = faiss.get_num_gpus() if hasattr(faiss, "get_num_gpus") else 0
        if gpu == "cpu":
            return
        if gpu in ("", "auto"):
            self.devices = list(range(available))
        else:
            self.devices = [int(d) for d in gpu.split(",") if d.strip()]
            missing = [d for d in self.devices if d >= available]
            if missing:
                raise SystemExit(f"bulk-match: GPU {missing} requested, {available} available")
        if self.devices:
            self.resources = [faiss.StandardGpuResources() for _ in self.devices]
            self.name = "faiss-gpu:" + ",".join(map(str, self.devices))

    def _one(self, slot: int, xq: np.ndarray, xb: np.ndarray, k: int) -> np.ndarray:
        if self.devices:
            _, idx = faiss.knn_gpu(self.resources[slot], xq, xb, k,
                                   metric=faiss.METRIC_INNER_PRODUCT, device=self.devices[slot])
            return idx
        if faiss is not None:
            _, idx = faiss.knn(np.ascontiguousarray(xq, dtype=np.float32),
                               np.ascontiguousarray(xb, dtype=np.float32), k,
                               metric=faiss.METRIC_INNER_PRODUCT)
            return idx
        sims = xq.astype(np.float32) @ xb.astype(np.float32).T
        idx = np.argpartition(-sims, k - 1, axis=1)[:, :k]
        return idx

    def search(self, xq: np.ndarray, xb: np.ndarray, k: int) -> tuple[np.ndarray, np.ndarray]:
        """Top-k of xb for every row of xq: (scores float32 rescored in FP32, row indices int64,
        -1 where xb has fewer than k rows). Rows are not sorted."""
        n = len(xq)
        k_eff = min(k, len(xb))
        scores = np.full((n, k), -np.inf, dtype=np.float32)
        index = np.full((n, k), -1, dtype=np.int64)
        if n == 0 or k_eff == 0:
            return scores, index
        if self.devices:
            xq, xb = xq.astype(np.float16, copy=False), xb.astype(np.float16, copy=False)
        chunks = [(s, min(s + QUERY_CHUNK, n)) for s in range(0, n, QUERY_CHUNK)]

        def run(slot_chunk):
            slot, (s, e) = slot_chunk
            idx = self._one(slot, np.ascontiguousarray(xq[s:e]), xb, k_eff)
            idx = idx.astype(np.int64, copy=False)
            valid = idx >= 0
            safe = np.where(valid, idx, 0)
            q32 = xq[s:e].astype(np.float32)
            b32 = xb[safe].astype(np.float32)  # (chunk, k, dim)
            exact = np.einsum("qd,qkd->qk", q32, b32)
            exact[~valid] = -np.inf
            scores[s:e, :k_eff] = exact
            index[s:e, :k_eff] = np.where(valid, idx, -1)

        slots = max(1, len(self.devices))
        work = [(i % slots, c) for i, c in enumerate(chunks)]
        if slots > 1:
            with ThreadPoolExecutor(slots) as pool:
                list(pool.map(run, work))
        else:
            for w in work:
                run(w)
        return scores, index


class Shards:
    """The shard directory: every shard's ids, with its vectors memory-mapped."""

    def __init__(self, directory: Path):
        self.parts: list[tuple[np.ndarray, np.ndarray]] = []
        for ids_path in sorted(directory.glob("*.ids.npy")):
            if ids_path.name.startswith("."):
                continue
            vec_path = ids_path.with_name(ids_path.name[: -len(".ids.npy")] + ".vectors.npy")
            if not vec_path.exists():
                continue
            ids = np.load(ids_path)
            vectors = np.load(vec_path, mmap_mode="r")
            if len(ids) != len(vectors):
                raise SystemExit(f"bulk-match: {ids_path} and {vec_path} disagree in length")
            self.parts.append((ids, vectors))

    def gather(self, wanted: np.ndarray, dim: int) -> np.ndarray:
        """The vectors of wanted, in wanted's order (float16). Fails on an id no shard holds."""
        out = np.zeros((len(wanted), dim), dtype=np.float16)
        found = np.zeros(len(wanted), dtype=bool)
        if len(wanted) == 0:
            return out
        order = np.argsort(wanted, kind="stable")
        sorted_wanted = wanted[order]
        for ids, vectors in self.parts:
            pos = np.searchsorted(sorted_wanted, ids)
            pos_clip = np.minimum(pos, len(sorted_wanted) - 1)
            hit = (pos < len(sorted_wanted)) & (sorted_wanted[pos_clip] == ids)
            if not hit.any():
                continue
            rows = np.nonzero(hit)[0]
            targets = order[pos_clip[rows]]
            fresh = ~found[targets]
            out[targets[fresh]] = vectors[rows[fresh]]
            found[targets[fresh]] = True
        if not found.all():
            missing = wanted[~found][:5]
            raise SystemExit(f"bulk-match: {int((~found).sum())} ids not in any shard, e.g. {missing.tolist()}")
        return out

    def stream(self, wanted_sorted: np.ndarray):
        """Yield (ids, vectors float16) for the rows of each shard whose id is in wanted_sorted,
        each id once."""
        seen = np.zeros(len(wanted_sorted), dtype=bool)
        for ids, vectors in self.parts:
            if len(wanted_sorted) == 0:
                return
            pos = np.searchsorted(wanted_sorted, ids)
            pos_clip = np.minimum(pos, len(wanted_sorted) - 1)
            hit = (pos < len(wanted_sorted)) & (wanted_sorted[pos_clip] == ids)
            rows = np.nonzero(hit)[0]
            rows = rows[~seen[pos_clip[rows]]]
            if len(rows) == 0:
                continue
            seen[pos_clip[rows]] = True
            yield ids[rows], np.asarray(vectors[rows])
        if not seen.all():
            missing = wanted_sorted[~seen][:5]
            raise SystemExit(f"bulk-match: {int((~seen).sum())} earlier ids not in any shard, e.g. {missing.tolist()}")


class Pairs:
    """Pairs at or above the floor, and hubs."""

    def __init__(self, floor: float, k: int):
        self.floor, self.k = floor, k
        self.a: list[np.ndarray] = []
        self.b: list[np.ndarray] = []
        self.s: list[np.ndarray] = []
        self.hub_ids: list[np.ndarray] = []
        self.hub_kth: list[np.ndarray] = []

    def add(self, query_ids: np.ndarray, scores: np.ndarray, neighbour_ids: np.ndarray) -> None:
        """query_ids (n,), scores and neighbour_ids (n, k): one row per query, -1 for none."""
        keep = (neighbour_ids >= 0) & (scores >= self.floor) & (neighbour_ids != query_ids[:, None])
        qi, kj = np.nonzero(keep)
        self.a.append(query_ids[qi])
        self.b.append(neighbour_ids[qi, kj])
        self.s.append(scores[qi, kj])
        # A query is a hub when all k slots hold a neighbour at or above the floor.
        full = keep.sum(axis=1) >= self.k
        if full.any():
            self.hub_ids.append(query_ids[full])
            self.hub_kth.append(np.where(keep[full], scores[full], np.inf).min(axis=1).astype(np.float32))

    def finish(self):
        if not self.a:
            empty_i, empty_f = np.zeros(0, np.int64), np.zeros(0, np.float32)
            return empty_i, empty_i, empty_f, empty_i, empty_f
        a, b = np.concatenate(self.a), np.concatenate(self.b)
        s = np.concatenate(self.s).astype(np.float32)
        lo, hi = np.minimum(a, b), np.maximum(a, b)
        order = np.lexsort((-s, hi, lo))  # by pair, best score first
        lo, hi, s = lo[order], hi[order], s[order]
        first = np.ones(len(lo), dtype=bool)
        first[1:] = (lo[1:] != lo[:-1]) | (hi[1:] != hi[:-1])
        hubs = np.concatenate(self.hub_ids) if self.hub_ids else np.zeros(0, np.int64)
        kth = np.concatenate(self.hub_kth) if self.hub_kth else np.zeros(0, np.float32)
        return lo[first], hi[first], s[first], hubs.astype(np.int64), kth.astype(np.float32)


def merge_topk(top_s, top_i, new_s, new_i, k):
    """Keep, per row, the k best of the running and the new candidates."""
    s = np.concatenate([top_s, new_s], axis=1)
    i = np.concatenate([top_i, new_i], axis=1)
    pick = np.argpartition(-s, k - 1, axis=1)[:, :k]
    return np.take_along_axis(s, pick, axis=1), np.take_along_axis(i, pick, axis=1)


def match(work: Path) -> dict:
    started = time.time()
    params = json.loads((work / "params.json").read_text())
    floor, k, dim = float(params["floor"]), int(params["k"]), int(params.get("dim", 512))
    searcher = Searcher(str(params.get("gpu", "auto")))
    shards = Shards(Path(params["shards"]))
    batch_ids = np.load(work / "batch_ids.npy").astype(np.int64)
    earlier_ids = np.sort(np.load(work / "earlier_ids.npy").astype(np.int64))
    others_ids = np.load(work / "others_ids.npy").astype(np.int64)
    others_vec = np.load(work / "others_vectors.npy").astype(np.float32)
    if others_vec.ndim != 2 or (len(others_vec) and others_vec.shape[1] != dim):
        others_vec = others_vec.reshape(len(others_ids), dim)
    log(f"batch {params['batch']}: {len(batch_ids)} faces, {len(earlier_ids)} earlier, "
        f"{len(others_ids)} outside the gallery; floor {floor}, k {k}, {searcher.name}")

    batch_vec = shards.gather(batch_ids, dim)
    n = len(batch_ids)
    top_s = np.full((n, k), -np.inf, dtype=np.float32)
    top_i = np.full((n, k), -1, dtype=np.int64)
    pairs = Pairs(floor, k)

    # The batch against itself: k + 1, since each face finds itself.
    s, idx = searcher.search(batch_vec, batch_vec, k + 1)
    nid = np.where(idx >= 0, batch_ids[np.maximum(idx, 0)], -1)
    s = np.where(nid == batch_ids[:, None], -np.inf, s)
    nid = np.where(np.isfinite(s), nid, -1)
    top_s, top_i = merge_topk(top_s, top_i, s, nid, k)

    def against(side_ids: np.ndarray, side_vec: np.ndarray) -> None:
        nonlocal top_s, top_i
        s, idx = searcher.search(batch_vec, side_vec, k)
        nid = np.where(idx >= 0, side_ids[np.maximum(idx, 0)], -1)
        top_s, top_i = merge_topk(top_s, top_i, s, nid, k)
        s, idx = searcher.search(side_vec, batch_vec, k)
        nid = np.where(idx >= 0, batch_ids[np.maximum(idx, 0)], -1)
        pairs.add(side_ids, s, nid)

    done = 0
    for ids, vec in shards.stream(earlier_ids):
        against(ids, vec)
        done += len(ids)
        log(f"earlier faces: {done}/{len(earlier_ids)}")
    if len(others_ids):
        against(others_ids, others_vec)

    pairs.add(batch_ids, top_s, top_i)
    a, b, score, hubs, kth = pairs.finish()
    np.save(work / "pairs_a.npy", a)
    np.save(work / "pairs_b.npy", b)
    np.save(work / "pairs_score.npy", score)
    np.save(work / "hubs_ids.npy", hubs)
    np.save(work / "hubs_kth.npy", kth)
    comparisons = n * (n - 1) // 2 + n * len(earlier_ids) + n * len(others_ids)
    result = {
        "comparisons": int(comparisons),
        "pairs": int(len(a)),
        "hubs": int(len(np.unique(hubs))),
        "seconds": round(time.time() - started, 3),
        "device": searcher.name,
    }
    (work / "result.json").write_text(json.dumps(result, indent=2) + "\n")
    log(f"{result['pairs']} pairs at or above {floor}, {result['hubs']} hubs, "
        f"{comparisons:.3g} comparisons in {result['seconds']} s")
    return result


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--work", required=True, help="work directory written by bulk-match match")
    args = parser.parse_args(argv)
    match(Path(args.work))
    return 0


if __name__ == "__main__":
    sys.exit(main())
