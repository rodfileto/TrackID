# TrackID Backend

FastAPI backend for facial recognition and tracking system.

## Project Structure

```
backend/
├── app/
│   ├── api/          # Route handlers
│   ├── core/         # Config, database setup
│   ├── models/       # SQLAlchemy ORM models
│   ├── schemas/      # Pydantic request/response schemas
│   ├── services/     # Business logic layer
│   └── dependencies/ # FastAPI dependency injection
├── logs/             # Application logs
├── main.py           # FastAPI app entry point
├── requirements.txt  # Python dependencies
└── .env.example      # Environment variables template
```

## Setup

1. Create virtual environment:
   ```bash
   python3.11 -m venv venv
   source venv/bin/activate
   ```

2. Install dependencies:
   ```bash
   pip install -r requirements.txt
   ```

3. Create `.env` from `.env.example`:
   ```bash
   cp .env.example .env
   ```

4. Run dev server:
   ```bash
   python main.py
   ```

   API available at `http://localhost:8000`
   OpenAPI docs at `http://localhost:8000/docs`

## Stack

- **FastAPI** - Modern async web framework
- **SQLAlchemy 2.0** - Async ORM
- **PostgreSQL 16 + pgvector** - Vector similarity search
- **Taskiq** - Async task queue
- **InsightFace + OpenCV** - CV/face detection
