FROM python:3.14-slim

WORKDIR /app

# Install system dependencies
# libgl1/libglib2.0-0/libsm6/libxext6/libxrender1/libxcb1 are runtime deps of opencv-python
RUN apt-get update && apt-get install -y \
    gcc \
    postgresql-client \
    curl \
    libgl1 \
    libglib2.0-0 \
    libsm6 \
    libxext6 \
    libxrender1 \
    libxcb1 \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements
COPY requirements.txt requirements-gpu.txt ./

# Install Python dependencies. Pass --build-arg GPU=1 to install the CUDA
# build of onnxruntime instead of CPU (see requirements-gpu.txt / README).
ARG GPU=0
RUN pip install --no-cache-dir -r requirements.txt \
    && if [ "$GPU" = "1" ]; then \
         pip uninstall -y onnxruntime \
         && pip install --no-cache-dir -r requirements-gpu.txt; \
       fi

# Copy app code
COPY . .

# Expose port
EXPOSE 8000

# Health check
HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
    CMD curl -f http://localhost:8000/health || exit 1

# Run app
CMD ["python", "main.py"]
