-- Raw provider transcript pointer for each attempt.
--
-- The parsed event timeline lives in provider_session_events; the unparsed
-- provider stdout/stderr is uploaded to object storage and referenced here.
-- See docs/superpowers/specs/2026-07-09-provider-transcript-tool-event-capture-design.md §4.6.
--
-- log_ref is an opaque, provider-neutral reference (currently the manifest
-- object key). API boundaries must not assume filesystem semantics.
--
-- log_sha256 is reported by the runtime that produced the bytes, so it is a
-- checksum, not a tamper-evidence proof. The control plane recomputes it on
-- first read before trusting it (§3.4 threat model, §3.5.3).
ALTER TABLE project_task_attempts
    ADD COLUMN log_store VARCHAR(50),
    ADD COLUMN log_ref TEXT,
    ADD COLUMN log_bytes BIGINT,
    ADD COLUMN log_sha256 VARCHAR(64),
    ADD COLUMN log_compressed BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE project_task_attempts
    ADD CONSTRAINT chk_project_task_attempts_log_store CHECK (
        log_store IS NULL OR log_store IN ('object_store', 'local_file')
    );

COMMENT ON COLUMN project_task_attempts.log_store IS 'Raw transcript backend: object_store | local_file';
COMMENT ON COLUMN project_task_attempts.log_ref IS 'Opaque reference to the raw transcript manifest';
COMMENT ON COLUMN project_task_attempts.log_bytes IS 'Total raw transcript bytes across all segments';
COMMENT ON COLUMN project_task_attempts.log_sha256 IS 'Runtime-reported sha256 of the concatenated raw transcript; must be recomputed before being trusted';
COMMENT ON COLUMN project_task_attempts.log_compressed IS 'Whether raw transcript segments are stored compressed';
