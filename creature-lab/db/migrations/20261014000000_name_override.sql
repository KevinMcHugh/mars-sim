-- migrate:up

-- A species' common name as a person or an agent renamed it, so species
-- that rolled alike can be told apart. The rolled name stays in singular and
-- plural (and in data), as it does for description; '' here means "use it".
ALTER TABLE species ADD COLUMN singular_override TEXT NOT NULL DEFAULT '';
ALTER TABLE species ADD COLUMN plural_override TEXT NOT NULL DEFAULT '';

-- migrate:down

ALTER TABLE species DROP COLUMN plural_override;
ALTER TABLE species DROP COLUMN singular_override;
