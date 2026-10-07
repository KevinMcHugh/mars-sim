-- migrate:up

-- Creature Lab's own schema (docs/creature-lab.md). Conventions follow the
-- inventory app it is modelled on: xid ids as CHAR(20), soft delete through
-- deleted_at, and every query filtering deleted_at IS NULL. There are no
-- tenants: the catalog is one shared set of species, and api keys identify
-- who did what.

CREATE TABLE api_keys (
    id           CHAR(20)    PRIMARY KEY,
    name         TEXT        NOT NULL,
    key_hash     TEXT        NOT NULL,
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ
);

CREATE UNIQUE INDEX api_keys_hash_active ON api_keys (key_hash) WHERE deleted_at IS NULL;

-- OAuth 2.1 for claude.ai custom connectors: dynamically registered public
-- clients, PKCE codes, and access/refresh token pairs. A token acts as the
-- api key that approved it.
CREATE TABLE oauth_clients (
    id            CHAR(20)    PRIMARY KEY,
    redirect_uris JSONB       NOT NULL,
    client_name   TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);

CREATE TABLE oauth_codes (
    code_hash             TEXT        PRIMARY KEY,
    client_id             CHAR(20)    NOT NULL REFERENCES oauth_clients(id),
    api_key_id            CHAR(20)    NOT NULL REFERENCES api_keys(id),
    redirect_uri          TEXT        NOT NULL,
    code_challenge        TEXT        NOT NULL,
    code_challenge_method TEXT        NOT NULL,
    scope                 TEXT,
    expires_at            TIMESTAMPTZ NOT NULL,
    consumed_at           TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- client_id is NULL for a web-page session minted by /login.
CREATE TABLE oauth_tokens (
    id                 CHAR(20)    PRIMARY KEY,
    token_hash         TEXT        NOT NULL UNIQUE,
    client_id          CHAR(20)    REFERENCES oauth_clients(id),
    api_key_id         CHAR(20)    NOT NULL REFERENCES api_keys(id),
    scope              TEXT,
    expires_at         TIMESTAMPTZ NOT NULL,
    refresh_token_hash TEXT        UNIQUE,
    refresh_expires_at TIMESTAMPTZ,
    revoked_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- A species as the game's roster code rolled it. data is the whole
-- sim.AlienSpecies as JSON, kept because re-rolling the seed on a newer
-- commit can give a different species; generator_rev says which commit
-- rolled it. The other columns are copies for listing and search.
-- form_count is how many sprite slots it has: one per form of its life, 1
-- for the single-form majority.
CREATE TABLE species (
    id              CHAR(20)    PRIMARY KEY,
    seed            BIGINT      NOT NULL,
    generator_rev   TEXT        NOT NULL,
    singular        TEXT        NOT NULL,
    plural          TEXT        NOT NULL,
    scientific_name TEXT        NOT NULL,
    emoji           TEXT        NOT NULL DEFAULT '',
    temperament     TEXT        NOT NULL,
    form_count      INT         NOT NULL CHECK (form_count >= 1),
    description     TEXT        NOT NULL,
    data            JSONB       NOT NULL,
    notes           TEXT        NOT NULL DEFAULT '',
    created_by      CHAR(20)    REFERENCES api_keys(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX species_active ON species (created_at DESC) WHERE deleted_at IS NULL;

-- Every SVG anyone drew for a slot (species, form). The svg is a few dozen
-- lines, so it lives in a text column. At most one candidate per slot is
-- accepted: that is the slot's sprite.
CREATE TABLE sprite_candidates (
    id          CHAR(20)    PRIMARY KEY,
    species_id  CHAR(20)    NOT NULL REFERENCES species(id),
    form        INT         NOT NULL CHECK (form >= 0),
    svg         TEXT        NOT NULL,
    note        TEXT        NOT NULL DEFAULT '',
    author      TEXT        NOT NULL DEFAULT '',
    created_by  CHAR(20)    REFERENCES api_keys(id),
    accepted_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX sprite_candidates_slot ON sprite_candidates (species_id, form, created_at DESC) WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX sprite_candidates_one_accepted
    ON sprite_candidates (species_id, form)
    WHERE accepted_at IS NOT NULL AND deleted_at IS NULL;

-- migrate:down

DROP TABLE sprite_candidates;
DROP TABLE species;
DROP TABLE oauth_tokens;
DROP TABLE oauth_codes;
DROP TABLE oauth_clients;
DROP TABLE api_keys;
