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
COPY requirements.txt .

# Install Python dependencies
RUN pip install --no-cache-dir -r requirements.txt

# Copy app code
COPY . .

# Expose port
EXPOSE 8000

# Health check
HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
    CMD curl -f http://localhost:8000/health || exit 1

# Run app
CMD ["python", "main.py"]
