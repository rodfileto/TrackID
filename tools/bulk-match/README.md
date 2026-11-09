# bulk-match: the exact GPU matcher

`cmd/bulk-match match` runs this program (Decision 1 of the
[large-gallery plan](../../docs/large-gallery-plan.md)). Go chooses the batch and writes a work
directory. This program compares the batch, exactly, against itself, every earlier batch
(streamed from the vector shards) and every trace, and writes the pairs at or above the floor (at
most `k` per face) back for Go to load. The file formats are in the docstring of `bulk_match.py`.

- **Search.** FAISS brute force (`knn_gpu`), FP16 on the GPU. Every candidate is then rescored in
  FP32 from the stored vectors, which are FP16.
- **Both directions.** The batch is searched against the earlier faces, and the earlier faces
  against the batch, so a cap hit on one side does not drop the pair.
- **Hubs.** A face whose k-th neighbour still scores at least the floor is recorded as a hub.

## Setup

[uv](https://docs.astral.sh/uv/), Python 3.12. The FAISS GPU wheels bring their own CUDA 12
runtime and cuBLAS; the host needs only the NVIDIA driver.

```
uv sync --project tools/bulk-match --extra gpu   # or --extra cpu, without a GPU
```

`cmd/bulk-match` runs it, by default, as:

```
uv run --project tools/bulk-match --extra gpu python tools/bulk-match/bulk_match.py --work <dir>
```

Change that with `-matcher`, and choose the device with `-gpu` (`auto`, `cpu`, `0,1`). With
FAISS missing it falls back to NumPy, which is fine for tests and slow for anything else.

## Tests

```
cd tools/bulk-match && uv run --extra gpu python -m unittest -v
```

The tests check the stored pairs and scores against brute force: on the GPU, on FAISS's CPU path
and on NumPy. The Go database test `bulk.TestBulkGallery` also runs this program, when `uv` is
installed.
