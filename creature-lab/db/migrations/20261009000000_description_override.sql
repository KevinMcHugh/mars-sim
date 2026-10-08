-- migrate:up

-- A species' field notes as a person or an agent rewrote them. The roster
-- code's own text stays in description, as rolled; '' here means "use it".
-- Kept apart rather than overwriting description, so the generated text is
-- never lost and an edit can always be reset.
ALTER TABLE species ADD COLUMN description_override TEXT NOT NULL DEFAULT '';

-- migrate:down

ALTER TABLE species DROP COLUMN description_override;
