-- +goose Up
-- The AI mode's audit trail (docs/ask-plan.md): each question an analyst put to the model, what it
-- consulted and what it answered. Like access_log, it answers "who asked what, and when", and it
-- is never updated or deleted. The steps keep the arguments and the results as the model saw them,
-- so an answer can be checked later even after the data has changed.
--
-- endpoint -- the host of the model's server (ASK_ENDPOINT)
-- external -- whether that host is outside the institute, so the question and the results left it
-- history  -- the earlier turns of the conversation the browser sent, which shaped the answer
-- steps    -- the tools the model called: name, arguments, result or error
-- answer   -- empty when the request failed, and then error says why
CREATE TABLE ask_queries (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    username TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    model TEXT NOT NULL,
    external BOOLEAN NOT NULL,
    question TEXT NOT NULL,
    history JSONB NOT NULL DEFAULT '[]',
    steps JSONB NOT NULL DEFAULT '[]',
    answer TEXT NOT NULL DEFAULT '',
    unverified_numbers JSONB NOT NULL DEFAULT '[]',
    error TEXT NOT NULL DEFAULT '',
    duration_ms BIGINT NOT NULL DEFAULT 0,
    asked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ask_queries_user_idx ON ask_queries (user_id, asked_at DESC);

-- +goose Down
DROP TABLE ask_queries;
