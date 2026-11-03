-- +goose Up
-- Articles (docs/person-article-plan.md): what intelligence analysts write about a subject, one
-- shared article per subject, Wikipedia style, with @ mentions of cases, occurrences, inquiries,
-- persons, clusters, requests and laudos. Only persons have articles for now; subject_kind leaves
-- room for others. An article is analysis, not data: nothing in the matcher, the clusters or the
-- reports reads these tables.
--
-- subject_ref is the subject's text key (persons.person_id), not a foreign key, so other kinds fit
-- later and a removed subject does not take its article's history with it.
CREATE TABLE articles (
    id BIGSERIAL PRIMARY KEY,
    subject_kind TEXT NOT NULL CHECK (subject_kind IN ('PERSON')),
    subject_ref TEXT NOT NULL,
    current_revision_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT articles_subject_key UNIQUE (subject_kind, subject_ref)
);

-- Drafts generated from the records (articles.PersonTemplate): so an analyst does not start a
-- person's article from scratch, the server writes a first draft from what the records link to
-- the person (identity, reviewed clusters, cases and their occurrences, inquiries, requests and
-- laudos, persons linked to the same cases). Every draft generated is logged here, whether or
-- not it is saved: who generated it, when, for whom, with which version of the generator, and
-- the draft itself. Never updated or deleted.
--
-- A revision saved from a draft names it (article_revisions.template_id), so the history shows
-- which text came from the records and which the analyst wrote.
CREATE TABLE article_templates (
    id BIGSERIAL PRIMARY KEY,
    subject_kind TEXT NOT NULL CHECK (subject_kind IN ('PERSON')),
    subject_ref TEXT NOT NULL,
    generator TEXT NOT NULL,
    language TEXT NOT NULL,
    body JSONB NOT NULL,
    body_sha256 BYTEA NOT NULL CHECK (length(body_sha256) = 32),
    generated_by BIGINT NOT NULL REFERENCES users(id),
    generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX article_templates_subject_idx ON article_templates (subject_kind, subject_ref, generated_at DESC);

-- The record of each revision: who wrote it, when and why, its size and the SHA-256 of its body
-- (the canonical JSON the server stored). Never updated or deleted. The text itself is in
-- article_revision_bodies, apart, so that a later redaction (the plan's decision 8) can remove a
-- revision's text and leave this record.
CREATE TABLE article_revisions (
    id BIGSERIAL PRIMARY KEY,
    article_id BIGINT NOT NULL REFERENCES articles(id),
    revision_number INTEGER NOT NULL CHECK (revision_number > 0),
    parent_revision_id BIGINT REFERENCES article_revisions(id),
    body_sha256 BYTEA NOT NULL CHECK (length(body_sha256) = 32),
    body_chars INTEGER NOT NULL CHECK (body_chars >= 0),
    edit_summary TEXT,
    author_id BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    template_id BIGINT REFERENCES article_templates(id),
    CONSTRAINT article_revisions_number_key UNIQUE (article_id, revision_number)
);

ALTER TABLE articles ADD CONSTRAINT articles_current_revision_fkey
    FOREIGN KEY (current_revision_id) REFERENCES article_revisions(id);

-- A revision's text. body is ProseMirror JSON, checked by the server against the allowed nodes
-- (articles.Normalize); body_text its plain text, for search and diffs. Never updated or
-- deleted, for now.
CREATE TABLE article_revision_bodies (
    revision_id BIGINT PRIMARY KEY REFERENCES article_revisions(id),
    body JSONB NOT NULL,
    body_text TEXT NOT NULL
);
CREATE INDEX article_revision_bodies_text_idx ON article_revision_bodies
    USING GIN (to_tsvector('simple', body_text));

-- The mentions in each article's current revision, rewritten on every save: what backlinks read.
-- ref is the target's key (see articles.Kind): no foreign key, so a removed target leaves the
-- article as it was, and reading shows the mention as removed.
CREATE TABLE article_mentions (
    article_id BIGINT NOT NULL REFERENCES articles(id),
    kind TEXT NOT NULL CHECK (kind IN ('CASE', 'OCCURRENCE', 'INQUIRY', 'PERSON', 'CLUSTER', 'REQUEST', 'REPORT')),
    ref TEXT NOT NULL,
    mention_count INTEGER NOT NULL DEFAULT 1 CHECK (mention_count > 0),
    PRIMARY KEY (article_id, kind, ref)
);
CREATE INDEX article_mentions_target_idx ON article_mentions (kind, ref);

-- Revisions are append-only, enforced (as docs/hash-chain-plan.md decision 3 does for the
-- decision tables).
-- +goose StatementBegin
CREATE FUNCTION articles_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER article_revisions_append_only BEFORE UPDATE OR DELETE ON article_revisions
    FOR EACH ROW EXECUTE FUNCTION articles_append_only();
CREATE TRIGGER article_revisions_no_truncate BEFORE TRUNCATE ON article_revisions
    FOR EACH STATEMENT EXECUTE FUNCTION articles_append_only();
CREATE TRIGGER article_revision_bodies_append_only BEFORE UPDATE OR DELETE ON article_revision_bodies
    FOR EACH ROW EXECUTE FUNCTION articles_append_only();
CREATE TRIGGER article_revision_bodies_no_truncate BEFORE TRUNCATE ON article_revision_bodies
    FOR EACH STATEMENT EXECUTE FUNCTION articles_append_only();
CREATE TRIGGER article_templates_append_only BEFORE UPDATE OR DELETE ON article_templates
    FOR EACH ROW EXECUTE FUNCTION articles_append_only();
CREATE TRIGGER article_templates_no_truncate BEFORE TRUNCATE ON article_templates
    FOR EACH STATEMENT EXECUTE FUNCTION articles_append_only();


-- +goose Down
DROP TABLE article_mentions;
ALTER TABLE articles DROP CONSTRAINT articles_current_revision_fkey;
DROP TABLE article_revision_bodies;
DROP TABLE article_revisions;
DROP TABLE article_templates;
DROP TABLE articles;
DROP FUNCTION articles_append_only();
