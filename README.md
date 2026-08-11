# TrackID

Facial recognition and tracking system with React frontend and FastAPI backend.

## Quick Start

### Development (Recommended)

Run backend infrastructure in Docker, frontend locally with HMR:

```bash
# Terminal 1: Backend infrastructure (Postgres, Redis, FastAPI)
docker-compose -f docker-compose.dev.yml up

# Terminal 2: Frontend (Vite dev server with auto-reload)
cd frontend
npm run dev
```

Access:
- **Frontend**: http://localhost:5173
- **API**: http://localhost:8000
- **API Docs**: http://localhost:8000/docs
- **Database**: localhost:5432 (postgres/password)
- **Redis**: localhost:6379

### Production (Docker)

Run everything containerized:

```bash
docker-compose up -d
```

Access:
- **Frontend**: http://localhost:3000
- **API**: http://localhost:8000
- **API Docs**: http://localhost:8000/docs

## Project Structure

```
trackid/
├── frontend/              # React + Vite + TailwindCSS
│   ├── src/
│   ├── Dockerfile         # Multi-stage nginx build
│   ├── nginx.conf         # Nginx config with API proxy
│   └── vite.config.ts     # Vite config with dev proxy
│
├── backend/               # FastAPI + SQLAlchemy
│   ├── app/
│   │   ├── api/          # Route handlers
│   │   ├── core/         # Config, database
│   │   ├── models/       # ORM models
│   │   ├── schemas/      # Pydantic schemas
│   │   ├── services/     # Business logic
│   │   └── dependencies/ # FastAPI injection
│   ├── main.py           # FastAPI entry point
│   ├── Dockerfile        # Backend container
│   └── requirements.txt   # Python dependencies
│
├── docker-compose.yml     # Production setup (all services in Docker)
├── docker-compose.dev.yml # Development setup (backend in Docker, frontend local)
└── .env.example          # Environment variables template
```

## Stack

| Layer | Technology | Purpose |
|-------|-----------|---------|
| Frontend | React 19 + Vite + TailwindCSS | Dashboard UI, graph views |
| API | FastAPI (Python 3.11+) | REST endpoints, WebSockets |
| Database | PostgreSQL 16 + pgvector | Relational + vector search |
| ORM | SQLAlchemy 2.0 (Async) | Database mapping |
| Task Queue | Taskiq + Redis | Async video processing |
| CV/AI | InsightFace + OpenCV | Face detection & embedding |

## Environment Variables

Copy `.env.example` to `.env`:

```bash
cp backend/.env.example backend/.env
```

Configure as needed:
```
DEBUG=False
DATABASE_URL=postgresql+asyncpg://postgres:password@postgres:5432/trackid
REDIS_URL=redis://redis:6379
```

## Development Workflow

### Backend

```bash
# Terminal 1: Run infrastructure
docker-compose -f docker-compose.dev.yml up

# Terminal 2: Navigate to backend
cd backend

# Install dependencies (first time only, or after requirements.txt changes)
pip install -r requirements.txt

# Run migrations (when available)
alembic upgrade head

# API docs: http://localhost:8000/docs
```

### Frontend

```bash
# Terminal 2: Navigate to frontend
cd frontend

# Install dependencies (first time only)
npm install

# Start dev server with HMR
npm run dev

# Open http://localhost:5173
```

API calls automatically proxy to `http://localhost:8000/api` via Vite proxy.

## Building for Production

```bash
# Build frontend
cd frontend
npm run build

# Build and run all services
docker-compose up -d
```

## Debugging

**Backend logs:**
```bash
docker logs trackid-backend -f
```

**Database:**
```bash
docker exec -it trackid-postgres psql -U postgres -d trackid
```

**Redis:**
```bash
docker exec -it trackid-redis redis-cli
```

## Research & Publications

TrackID is built to support two complementary scientific contributions:

1. **Paper 1 (Tactical / Forensic)**: Reducing cognitive bottlenecks in forensic video review via decision support systems and HITL compliance.
   - Target: Decision Support Systems, Forensic Science International, IEEE Transactions on Human-Machine Systems.
   - See: [docs/PAPER1_FORENSIC.md](docs/PAPER1_FORENSIC.md)

2. **Paper 2 (Strategic / Intelligence)**: Revealing hidden organizational structures in criminal networks via spatio-temporal graph analysis.
   - Target: Expert Systems with Applications, Knowledge-Based Systems, Network Science.
   - See: [docs/PAPER2_INTELLIGENCE.md](docs/PAPER2_INTELLIGENCE.md)

**Strategy Overview**: [docs/RESEARCH_STRATEGY.md](docs/RESEARCH_STRATEGY.md) — why and how these two papers are separated despite sharing a single codebase.

## Next Steps

1. **Define database models** → `backend/app/models/`
2. **Create API routes** → `backend/app/api/`
3. **Build services** → `backend/app/services/`
4. **Configure task queue** → Taskiq workers for video processing
5. **Read research strategy** → `docs/RESEARCH_STRATEGY.md` to understand publication roadmap
