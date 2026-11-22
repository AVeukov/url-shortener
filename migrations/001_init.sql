CREATE TABLE IF NOT EXISTS short_links (
    code         VARCHAR(32) PRIMARY KEY,
    original_url TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ,
    click_count  BIGINT      NOT NULL DEFAULT 0 CHECK (click_count >= 0),
    CONSTRAINT original_url_not_empty CHECK (length(original_url) > 0)
);

CREATE INDEX IF NOT EXISTS short_links_expires_at_idx
    ON short_links (expires_at)
    WHERE expires_at IS NOT NULL;
