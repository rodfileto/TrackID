"""
Makes the CUDA/cuDNN shared libraries from pip-installed nvidia-*-cu12
packages (onnxruntime-gpu's dependencies) visible to the dynamic linker.

glibc's dynamic linker reads LD_LIBRARY_PATH once at process startup;
changing os.environ afterwards has no effect on later dlopen() calls -
which is how onnxruntime loads the CUDA execution provider the first
time a GPU session is created. So if GPU is requested and the required
paths aren't already present, we re-exec this process with a corrected
environment before anything imports onnxruntime.

No-op if GPU isn't requested, or the nvidia-*-cu12 pip packages aren't
installed (i.e. CPU-only onnxruntime is in use).
"""
import glob
import os
import sys


def ensure_cuda_libs_on_path() -> None:
    use_gpu = (
        os.getenv("FACE_USE_GPU", "False").lower() == "true"
        or os.getenv("FACE_QUALITY_USE_GPU", "False").lower() == "true"
    )
    if not use_gpu:
        return

    try:
        import nvidia
    except ImportError:
        return

    lib_dirs = sorted(
        d for path in nvidia.__path__ for d in glob.glob(os.path.join(path, "*", "lib"))
    )
    if not lib_dirs:
        return

    current = os.environ.get("LD_LIBRARY_PATH", "")
    current_entries = set(current.split(":")) if current else set()
    if all(d in current_entries for d in lib_dirs):
        return

    os.environ["LD_LIBRARY_PATH"] = ":".join(lib_dirs) + (":" + current if current else "")
    os.execv(sys.executable, [sys.executable] + sys.argv)
