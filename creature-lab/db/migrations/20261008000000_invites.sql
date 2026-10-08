-- migrate:up

-- Single-use invites minted from the CLI, after the inventory app's. An
-- invite is how a new person connects an OAuth client (claude.ai) without
-- an api key: redeeming one on the authorize page mints their key, records
-- it here, and approves the connection. An existing key holder pastes their
-- key instead, so an invite is only ever needed once per person.
CREATE TABLE invites (
    id          CHAR(20)    PRIMARY KEY,
    code_hash   TEXT        NOT NULL UNIQUE,
    note        TEXT        NOT NULL DEFAULT '',
    expires_at  TIMESTAMPTZ,
    used_at     TIMESTAMPTZ,
    api_key_id  CHAR(20)    REFERENCES api_keys(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX invites_active ON invites (created_at)
    WHERE used_at IS NULL AND deleted_at IS NULL;

-- migrate:down

DROP TABLE invites;
