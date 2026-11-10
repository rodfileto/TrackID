# Live updates: notifications, camera webhooks and chat

Proposed 2026-11-09. Phase 1 is built and was confirmed working on the development stack the same
day. Phases 2 to 4 are still proposals, and the questions at the end shape them. TrackID gets one way of pushing things to the browser, used by three features: stored notifications,
updates from live cameras arriving by webhook, and chat. It adds no new service and does not use
Redis for any of this.

## What is there now

- **The push path exists, with one producer.** The hub (`realtime`), stored notifications
  (`notifications`) and the bell in the header are in place. Assigning an analysis step to someone
  is the only event that notifies. Chat and cameras are not built.
- **Other pages still poll.** Pages that follow background work poll, as `CaseVideosCard` does
  every 3 seconds.
- **Two processes.** The API (`cmd/trackid`, started by `server.Run`) and the worker
  (`cmd/worker`), which holds the vision models.
- **Redis is optional for the API but not for the worker.** The API runs without it and logs that
  background jobs won't be enqueued. `cmd/worker` exits at startup if `REDIS_URL` is empty.
- **Auth is a Bearer header.** `Middleware` reads only `Authorization`, and `ActiveUser` re-checks
  the account on every request, so deactivation takes effect at once.
- **Case access is more than yes or no.** `distribution.Rights` returns `Read`, `Write`, `Holds`
  and `Blind`. A user working an analysis blind reaches only `blindRoutes`. A read of a case the
  user does not hold is written to `access_log`, and a read that can't be logged is refused.

## Decisions

1. **One API instance.** One running copy of `cmd/trackid`. Go uses every core from that one
   process, so this limits availability rather than CPU use. WebSocket connections live in the
   memory of the process that accepted them, which is what makes a single copy simpler.
2. **WebSocket, because of chat.** One connection per tab, carrying messages tagged by topic.
   The library is `coder/websocket`.
3. **Writes stay ordinary HTTP.** Chat messages are sent by POST, so they get the existing
   middleware, validation and transactions. The socket carries only subscribe, unsubscribe and
   typing from the client.
4. **Postgres is the record, the socket is only a signal.** Anything that must not be lost is
   committed first. A client that missed a push recovers by fetching from Postgres.
5. **Three paths into the hub, by where the event starts.**

   | Event | Starts in | Path |
   |---|---|---|
   | Camera status, raw events | API (webhook) | Straight to the hub |
   | Chat message | API (POST) | Commit, then the hub |
   | Notification | API or worker | `pg_notify` in the same transaction, API listens, then the hub |

6. **Camera volume stays out of `LISTEN/NOTIFY`.** Each notifying commit takes a global lock, so
   it suits low-rate events. Raw camera events never go through it.
7. **Not every camera event is a notification.** Status and raw detections are live-only. A
   stored notification is for something a person must act on, and repeats are grouped per camera
   and subject within a time window.
8. **Video is out of scope.** Webhooks carry events and metadata. Live video would need a media
   server (RTSP/WebRTC), with the browser connecting to it directly.
9. **Nothing sensitive in a notification.** A row holds a kind and a reference, and the text is
   built when it is read, under the reader's current access. The same applies to any future email.

## Architecture

```
Camera ─ POST webhook ─> API: verify signature
                          ├─ status update ──────────────────────> hub
                          └─ event to analyze: INSERT camera_events, 202
                                     │
                     Worker claims rows from Postgres
                                     │ rules, matching
                     COMMIT notification + pg_notify(id)
                                     │
                     API LISTEN ─ reads row ─────────────────────> hub

Chat ─ POST message ─> API: authorize, save, COMMIT ─────────────> hub

hub ─ WebSocket ─> browsers
```

## The hub

A new `realtime` package, behind a small interface so a message bus can replace the in-memory
delivery later without touching handlers:

```go
type Publisher interface {
	Publish(topic string, event Event)
}
```

- It maps each topic to its clients, guarded by a `sync.RWMutex`.
- Each client has a bounded channel and one writer goroutine. `Publish` never blocks.
- **A slow client never slows the others.** For camera status, keep only the latest per camera.
  For anything else, disconnect the client, which resyncs on reconnect.
- It is added to `api.Dependencies`. A nil hub answers 503, as other optional integrations do.

Topics: `user:<id>` (joined on connect), `case:<id>` (chat), `camera:<id>` and `cameras` (status).
Only `user:<id>` exists so far. The browser sends nothing yet, so any message it sends closes the
connection. Subscribe and unsubscribe arrive with chat, along with the case-access rules below.

Starting limits, to adjust under load: client buffer 64 events, ping every 30 s, access re-check
every 60 s. Chat adds inbound frames up to 4 KB and up to 100 subscriptions per connection.

### Connecting

Browsers can't set `Authorization` on a WebSocket, and a token in a URL ends up in logs.

1. `POST /api/realtime/ticket`, in the `protected` group, returns a one-time ticket valid for
   30 seconds. Going through `requestApi` gives it the existing token renewal for free.
2. `GET /api/realtime?ticket=...`, outside `protected`, consumes the ticket and upgrades. It
   checks `Origin` against the app's own origin.

Tickets live in the hub's memory and hold the user ID and the token's expiry.

### Keeping access correct on a long connection

Middleware runs once at connect, so the hub has to do what it would have done on each request:

- Close the connection when the JWT expires. The client gets a new ticket and reconnects.
- Every 60 s, reload the account and re-check `distribution.Rights` for each subscribed case. Drop
  what is no longer allowed, and close the connection if the account was deactivated.
- **Honor blind work.** A `Rights.Blind` user can't subscribe to that case's topic, and is removed
  if an analysis becomes blind while subscribed. Chat on a case could otherwise tell an examiner
  what a colleague concluded.
- Subscribing to a case the user doesn't hold is a read, so it writes `access_log` as `CaseAccess`
  does, and is refused if the log can't be written.

The rule inside `CaseAccess` depends on Gin. Extract the decision into a plain function that both
the middleware and the hub call, so the two can't drift apart.

### Worker to API

- A `notifications.Create(tx, ...)` helper inserts the row and calls `pg_notify('trackid_live',
  '<id>')`. Postgres delivers it only on commit, so a rollback sends nothing. Payloads carry only
  the ID (8 KB limit).
- The API runs one listener goroutine on a dedicated `pgx.Conn`, outside the `database/sql` pool,
  with a reconnect loop. PgBouncer in transaction mode must not sit on this connection.
- After reconnecting, the listener tells all clients to refetch, since notifications may have
  been missed in the gap.

## Data

Migrations continue from `022`, one per phase, with queries through sqlc.

| Table | Columns | Notes |
|---|---|---|
| `notifications` | `id`, `user_id`, `kind`, `subject`, `created_at`, `read_at` | `kind` and `subject` as in `access_log`. Partial index on unread rows per user |
| `chat_threads` | `id`, `case_id`, `created_at` | Assumes threads per case (question 2) |
| `chat_messages` | `id`, `thread_id`, `author_id`, `body`, `created_at` | Reconnect asks for messages after the last ID seen |
| `cameras` | `id`, `name`, `source`, `secret`, `previous_secret`, `active`, `created_at` | Two secrets allow rotation |
| `camera_states` | `camera_id`, `status`, `last_seen_at` | Latest state only, one row per camera |
| `camera_events` | `id`, `camera_id`, `source_event_id`, `kind`, `occurred_at`, `received_at`, `payload`, `status`, `locked_until`, `attempts`, `last_error` | Unique on `(camera_id, source_event_id)`. It is also the job queue |

HMAC needs the original secret, so `cameras.secret` can't be hashed like a password. It is
encrypted with a key from the environment.

## Camera webhook

`POST /api/webhooks/cameras/:cameraId`, outside `protected`:

1. Limit the body size with `http.MaxBytesReader`, and rate-limit per camera.
2. Find the camera and refuse it if inactive.
3. Verify the HMAC over timestamp and raw body with `hmac.Equal`, accepting a 5-minute window.
4. Status update: upsert `camera_states` and publish to the hub.
5. Event to analyze: `INSERT ... ON CONFLICT DO NOTHING`. Webhooks are delivered at least once.
6. Reply 202 without waiting for any analysis.

Vendors sign and shape payloads differently, so a small `camera.Adapter` turns each `source` into
one internal event type. Vendor adapters can be attached through `Dependencies.Register`.

### Processing without Asynq

`camera_events` is the queue, so saving an event and creating its job are one insert. Workers
claim rows like this:

```sql
UPDATE camera_events
SET status = 'processing', locked_until = now() + interval '2 minutes', attempts = attempts + 1
WHERE id = (
  SELECT id FROM camera_events
  WHERE status = 'pending' OR (status = 'processing' AND locked_until < now())
  ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1
)
RETURNING *;
```

- The loop polls about once a second. After 5 attempts a row becomes `failed` with `last_error`.
- A scheduled delete removes processed events after a retention period.
- If a second job type appears, move to River rather than growing this by hand.

**The worker needs one change.** The loop belongs in `cmd/worker`, so it shares the loaded vision
models instead of loading a second copy onto the GPU. That process currently requires Redis, so:

- Make `REDIS_URL` optional there. Without it, Asynq doesn't start and its handlers aren't
  registered, and the camera loop runs alone.
- Camera processing calls the matching code directly and does not enqueue Asynq tasks. Existing
  handlers such as `HandleComputeCodification` enqueue follow-up work, so they can't be reused as
  they stand.

Embedding, fingerprint and video jobs keep using Asynq and Redis, unchanged.

## Frontend

- `services/realtime.ts`: one socket per tab, ticket then connect, reconnect with backoff and
  jitter, and a refetch after every reconnect.
- A bell in `AppHeader` with the unread count and a list, based on the template's dropdown.
- A chat panel on the case page, hidden during blind work.
- A camera status view.
- The Vite proxy for `/api` needs `ws: true`.

## Phases

1. **Hub and notifications** (done). The `realtime` package, tickets, account re-checks, the
   `notifications` table and endpoints (list with unread count, mark read), the listener, and the
   bell. First producer: assigning an analysis step, manually or by distribution, except to
   oneself. While a user works an analysis blind, the bell shows no case identity.
2. **Chat.** Threads, messages, the POST endpoint, reconnect catch-up, the blind-work rule, typing
   signals.
3. **Camera status.** Camera registry, signed webhook, adapters, `camera_states`, the live view.
4. **Camera analysis.** `camera_events`, the claim loop, optional Redis in `cmd/worker`, rules,
   and grouped notifications.

Before opening any of this to real traffic, have the reverse proxy allow WebSocket upgrades on
`/api/realtime` and keep the original `Host` header, since the API refuses a WebSocket whose
`Origin` doesn't match it.

## Failure behavior

| Failure | Result |
|---|---|
| API restarts | Sockets drop, clients reconnect with backoff and refetch |
| Listener disconnects | On reconnect, clients are told to refetch |
| Crash after a chat commit | The message is saved, and clients get it on reconnect |
| Worker dies mid-job | `locked_until` expires and another worker takes the row |
| Client too slow | Camera status is coalesced, otherwise the client is disconnected and resyncs |
| Live camera status missed | Lost, and the next update replaces it |

## Testing

Covered by phase 1's tests:

- Hub: fan-out, slow clients and disconnects, under `go test -race`.
- Tickets: expiry, single use, and refusal from another origin.
- Account access: token expiry, revocation and a failed re-check close the connection.
- Notifications: created in the assignment's transaction, private to their user, and hiding the
  case once the work moves or while it is blind.
- `pg_notify`: delivered after commit, nothing after rollback, and a resync after the listener
  reconnects.

Still to write, with their phases:

- Hub: concurrent subscribe and unsubscribe.
- Case access: subscribing without rights, a blind user refused, access revoked mid-connection,
  and `access_log` written. These matter most.
- Webhook: bad signature, stale timestamp, duplicate event, oversized body.
- Queue (`*_db_test.go`): concurrent claimers never share a row, and expired locks are reclaimed.
- A camera scenario in `docs/stress-test-plan.md`, once the event rate is known.

## When this needs to change

The hub's in-memory delivery stops being enough when a second API instance runs, whether for
availability or load, because a webhook can land on a copy that doesn't hold the user's socket.

- Chat and notifications can then both go over `pg_notify`, since their volume is low.
- Raw camera events would need Redis or NATS behind `Publisher`.
- Tickets and presence would move out of memory, and Centrifugo becomes worth comparing with the
  hand-written hub.

A rolling deploy briefly runs two copies, so stop the old one before starting the new one.

## Open questions

1. **What does the webhook send, and how much?** Metadata from tens of cameras fits this plan.
   Frames, or hundreds of events per second, need batching or a different ingest path.
2. **What is chat attached to?** This plan assumes threads per case, reusing case permissions.
   Direct messages need their own access model.
3. **Are chat messages part of the official record?** This plan assumes append-only, with no
   editing or deleting. If they count as evidence, decide the retention period and whether they
   join the custody ledger (`docs/hash-chain-plan.md`), which today doesn't cover them.
4. **Who can see cameras?** A new capability, or an existing one such as intelligence?
