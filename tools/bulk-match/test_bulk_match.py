"""Checks bulk_match against brute force: run with `uv run --extra gpu python -m unittest`
(or --extra cpu; with neither extra, NumPy searches)."""

import json
import tempfile
import unittest
from pathlib import Path

import numpy as np

import bulk_match

DIM = 512


def unit(x):
    return (x / np.linalg.norm(x, axis=1, keepdims=True)).astype(np.float32)


def world(rng, people=300, per=3, noise=0.35):
    """people identities with per photos each: a centre plus noise, so same-identity photos score
    high and different ones near zero."""
    centres = unit(rng.standard_normal((people, DIM)))
    vecs = np.repeat(centres, per, axis=0) + noise * rng.standard_normal((people * per, DIM)) / np.sqrt(DIM)
    return unit(vecs)


def write_shards(directory, ids, vecs, parts=3):
    directory.mkdir(parents=True, exist_ok=True)
    for i, chunk in enumerate(np.array_split(np.arange(len(ids)), parts)):
        np.save(directory / f"shard-t-{i:05d}.vectors.npy", vecs[chunk].astype(np.float16))
        np.save(directory / f"shard-t-{i:05d}.ids.npy", ids[chunk].astype(np.int64))


def run(work, shards, batch, earlier, others_ids, others_vec, floor, k, gpu):
    work.mkdir(parents=True, exist_ok=True)
    (work / "params.json").write_text(json.dumps({"shards": str(shards), "floor": floor, "k": k,
                                                   "batch": 1, "gpu": gpu, "dim": DIM}))
    np.save(work / "batch_ids.npy", batch)
    np.save(work / "earlier_ids.npy", earlier)
    np.save(work / "others_ids.npy", others_ids)
    np.save(work / "others_vectors.npy", others_vec)
    result = bulk_match.match(work)
    a, b, s = (np.load(work / f) for f in ("pairs_a.npy", "pairs_b.npy", "pairs_score.npy"))
    return result, {(int(x), int(y)): float(z) for x, y, z in zip(a, b, s)}


def truth(ids_vecs, batch, earlier, others, floor, k):
    """Every pair the matcher must keep when no face has k neighbours above the floor."""
    lookup = dict(ids_vecs)
    side = list(batch) + list(earlier) + list(others)
    want = {}
    bset = set(batch)
    for i, x in enumerate(side):
        for y in side[i + 1:]:
            if x not in bset and y not in bset:
                continue  # earlier x earlier and others x others are not this batch's work
            v = float(np.dot(lookup[x].astype(np.float32), lookup[y].astype(np.float32)))
            if v >= floor:
                want[(min(x, y), max(x, y))] = v
    return want


class TestMatch(unittest.TestCase):
    def check(self, gpu):
        rng = np.random.default_rng(7)
        vecs = world(rng)
        ids = np.arange(1000, 1000 + len(vecs), dtype=np.int64)
        perm = rng.permutation(len(ids))
        earlier, batch, others = ids[perm[:400]], ids[perm[400:850]], ids[perm[850:]]
        gallery = np.concatenate([earlier, batch])
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            pos = {int(x): i for i, x in enumerate(ids)}
            write_shards(tmp / "shards", gallery, vecs[[pos[int(x)] for x in gallery]])
            others_vec = vecs[[pos[int(x)] for x in others]]
            floor, k = 0.5, 20
            result, got = run(tmp / "work", tmp / "shards", batch, earlier, others, others_vec, floor, k, gpu)
            half = {int(x): vecs[pos[int(x)]].astype(np.float16) for x in gallery}
            half.update({int(x): vecs[pos[int(x)]] for x in others})
            want = truth(half.items(), [int(x) for x in batch], [int(x) for x in earlier],
                         [int(x) for x in others], floor, k)
            self.assertGreater(len(want), 100)
            self.assertEqual(set(got), set(want))
            for key, v in want.items():
                self.assertAlmostEqual(got[key], v, places=4)
            nb, ne, no = len(batch), len(earlier), len(others)
            self.assertEqual(result["comparisons"], nb * (nb - 1) // 2 + nb * ne + nb * no)
            self.assertEqual(result["hubs"], 0)

    def test_default_device(self):
        self.check("auto")

    def test_cpu(self):
        if bulk_match.faiss is None:
            self.skipTest("FAISS not installed")
        self.check("cpu")

    def test_numpy(self):
        saved = bulk_match.faiss
        bulk_match.faiss = None
        try:
            self.check("auto")
        finally:
            bulk_match.faiss = saved

    def test_hubs(self):
        """With k smaller than a face's neighbours above the floor, the face is a hub."""
        rng = np.random.default_rng(3)
        vecs = world(rng, people=20, per=8, noise=0.2)
        ids = np.arange(1, len(vecs) + 1, dtype=np.int64)
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            write_shards(tmp / "shards", ids, vecs, parts=1)
            result, got = run(tmp / "work", tmp / "shards", ids, ids[:0], ids[:0],
                              np.zeros((0, DIM), np.float32), 0.5, 3, "auto")
            self.assertEqual(result["hubs"], len(ids))
            hubs = np.load(tmp / "work" / "hubs_kth.npy")
            self.assertTrue((hubs >= 0.5).all())


if __name__ == "__main__":
    unittest.main()
