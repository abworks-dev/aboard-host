CREATE TABLE schema_metadata (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 kind TEXT NOT NULL CHECK(kind='workspace'),
 version INTEGER NOT NULL CHECK(version=1)
) STRICT;
CREATE TABLE migration_ledger (
 sequence INTEGER PRIMARY KEY CHECK(sequence>0),
 version INTEGER NOT NULL UNIQUE CHECK(version>0),
 checksum TEXT NOT NULL CHECK(length(checksum)=64),
 state TEXT NOT NULL CHECK(state='applied')
) STRICT;
CREATE TABLE workspace_identity (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 workspace_id TEXT NOT NULL UNIQUE,
 tenant_id TEXT NOT NULL,
 format_version INTEGER NOT NULL CHECK(format_version=2),
 schema_version INTEGER NOT NULL CHECK(schema_version=1),
 identity_digest TEXT NOT NULL CHECK(length(identity_digest)=64),
 display_name TEXT NOT NULL,
 business_state TEXT NOT NULL CHECK(business_state IN ('ready','unavailable')),
 UNIQUE(workspace_id,tenant_id)
) STRICT;
CREATE TABLE workspace_head (
 workspace_id TEXT PRIMARY KEY REFERENCES workspace_identity(workspace_id),
 revision INTEGER NOT NULL CHECK(revision>=0)
) STRICT;
CREATE TABLE text_record (
 workspace_id TEXT NOT NULL,
 tenant_id TEXT NOT NULL,
 resource_id TEXT NOT NULL,
 current_version INTEGER NOT NULL CHECK(current_version>0),
 PRIMARY KEY(workspace_id,resource_id),
 FOREIGN KEY(workspace_id,tenant_id) REFERENCES workspace_identity(workspace_id,tenant_id),
 FOREIGN KEY(workspace_id,resource_id,current_version) REFERENCES text_version(workspace_id,resource_id,version) DEFERRABLE INITIALLY DEFERRED
) STRICT;
CREATE TABLE candidate (
 workspace_id TEXT NOT NULL REFERENCES workspace_identity(workspace_id),
 candidate_id TEXT NOT NULL,
 principal_id TEXT NOT NULL,
 resource_id TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('create-text','update-text')),
 base_revision INTEGER NOT NULL CHECK(base_revision>=0),
 base_digest TEXT,
 title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 256),
 body TEXT NOT NULL CHECK(length(CAST(body AS BLOB))<=65536),
 content_digest TEXT NOT NULL CHECK(length(content_digest)=64),
 candidate_digest TEXT NOT NULL CHECK(length(candidate_digest)=64),
 preview_version INTEGER NOT NULL CHECK(preview_version>0),
 preview_epoch TEXT NOT NULL,
 expires_at TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('previewed','applied')),
 PRIMARY KEY(workspace_id,candidate_id),
 UNIQUE(workspace_id,candidate_id,principal_id),
 CHECK((kind='create-text' AND base_digest IS NULL) OR (kind='update-text' AND base_digest IS NOT NULL AND length(base_digest)=64))
) STRICT;
CREATE TABLE save_operation (
 workspace_id TEXT NOT NULL REFERENCES workspace_identity(workspace_id),
 operation_id TEXT NOT NULL,
 principal_id TEXT NOT NULL,
 candidate_id TEXT NOT NULL,
 input_digest TEXT NOT NULL CHECK(length(input_digest)=64),
 admission_epoch TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('accepted','committed','rejected','unknown')),
 failure_code TEXT,
 PRIMARY KEY(workspace_id,operation_id),
 UNIQUE(workspace_id,operation_id,candidate_id,principal_id,input_digest),
 FOREIGN KEY(workspace_id,candidate_id,principal_id) REFERENCES candidate(workspace_id,candidate_id,principal_id)
) STRICT;
CREATE TABLE text_version (
 workspace_id TEXT NOT NULL,
 resource_id TEXT NOT NULL,
 version INTEGER NOT NULL CHECK(version>0),
 workspace_revision INTEGER NOT NULL CHECK(workspace_revision>0),
 title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 256),
 body TEXT NOT NULL CHECK(length(CAST(body AS BLOB))<=65536),
 content_digest TEXT NOT NULL CHECK(length(content_digest)=64),
 operation_id TEXT NOT NULL,
 created_by TEXT NOT NULL,
 PRIMARY KEY(workspace_id,resource_id,version),
 UNIQUE(workspace_id,workspace_revision),
 UNIQUE(workspace_id,operation_id),
 UNIQUE(workspace_id,resource_id,version,workspace_revision,content_digest,operation_id,created_by),
 FOREIGN KEY(workspace_id,resource_id) REFERENCES text_record(workspace_id,resource_id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(workspace_id,operation_id) REFERENCES save_operation(workspace_id,operation_id)
) STRICT;
CREATE TABLE commit_receipt (
 workspace_id TEXT NOT NULL,
 operation_id TEXT NOT NULL,
 input_digest TEXT NOT NULL,
 principal_id TEXT NOT NULL,
 candidate_id TEXT NOT NULL,
 resource_id TEXT NOT NULL,
 new_version INTEGER NOT NULL CHECK(new_version>0),
 new_revision INTEGER NOT NULL CHECK(new_revision>0),
 content_digest TEXT NOT NULL CHECK(length(content_digest)=64),
 PRIMARY KEY(workspace_id,operation_id),
 FOREIGN KEY(workspace_id,operation_id,candidate_id,principal_id,input_digest) REFERENCES save_operation(workspace_id,operation_id,candidate_id,principal_id,input_digest),
 FOREIGN KEY(workspace_id,resource_id,new_version,new_revision,content_digest,operation_id,principal_id) REFERENCES text_version(workspace_id,resource_id,version,workspace_revision,content_digest,operation_id,created_by)
) STRICT;
CREATE TABLE audit_entry (
 workspace_id TEXT NOT NULL REFERENCES workspace_identity(workspace_id),
 sequence INTEGER NOT NULL CHECK(sequence>0),
 principal_id TEXT NOT NULL,
 operation_id TEXT NOT NULL,
 action TEXT NOT NULL,
 outcome_code TEXT NOT NULL,
 revision INTEGER NOT NULL CHECK(revision>=0),
 PRIMARY KEY(workspace_id,sequence),
 FOREIGN KEY(workspace_id,operation_id) REFERENCES save_operation(workspace_id,operation_id)
) STRICT;
CREATE TRIGGER immutable_text_version_update BEFORE UPDATE ON text_version BEGIN SELECT RAISE(ABORT,'immutable version'); END;
CREATE TRIGGER immutable_text_version_delete BEFORE DELETE ON text_version BEGIN SELECT RAISE(ABORT,'immutable version'); END;
CREATE TRIGGER immutable_receipt_update BEFORE UPDATE ON commit_receipt BEGIN SELECT RAISE(ABORT,'immutable receipt'); END;
CREATE TRIGGER immutable_receipt_delete BEFORE DELETE ON commit_receipt BEGIN SELECT RAISE(ABORT,'immutable receipt'); END;
