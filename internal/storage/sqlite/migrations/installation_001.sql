CREATE TABLE schema_metadata (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 kind TEXT NOT NULL CHECK(kind='installation'),
 version INTEGER NOT NULL CHECK(version=1)
) STRICT;
CREATE TABLE migration_ledger (
 sequence INTEGER PRIMARY KEY CHECK(sequence>0),
 version INTEGER NOT NULL UNIQUE CHECK(version>0),
 checksum TEXT NOT NULL CHECK(length(checksum)=64),
 state TEXT NOT NULL CHECK(state='applied')
) STRICT;
CREATE TABLE installation_identity (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 installation_id TEXT NOT NULL UNIQUE,
 tenant_id TEXT NOT NULL UNIQUE,
 owner_principal_id TEXT NOT NULL,
 windows_sid TEXT NOT NULL,
 schema_version INTEGER NOT NULL CHECK(schema_version=1),
 secret_reference TEXT NOT NULL,
 FOREIGN KEY(owner_principal_id,tenant_id) REFERENCES principal(principal_id,tenant_id) DEFERRABLE INITIALLY DEFERRED
) STRICT;
CREATE TABLE principal (
 principal_id TEXT PRIMARY KEY,
 tenant_id TEXT NOT NULL,
 grant_version INTEGER NOT NULL CHECK(grant_version>0),
 revoked INTEGER NOT NULL CHECK(revoked IN (0,1)),
 UNIQUE(principal_id,tenant_id),
 FOREIGN KEY(tenant_id) REFERENCES installation_identity(tenant_id)
) STRICT;
CREATE TABLE directory_registration (
 workspace_id TEXT PRIMARY KEY,
 tenant_id TEXT NOT NULL,
 root_locator TEXT NOT NULL UNIQUE,
 allowed_location TEXT NOT NULL,
 volume_identity TEXT NOT NULL,
 root_file_id BLOB NOT NULL CHECK(length(root_file_id)=16),
 registration_version INTEGER NOT NULL CHECK(registration_version>0),
 state TEXT NOT NULL CHECK(state IN ('pending','registered')),
 UNIQUE(workspace_id,tenant_id),
 FOREIGN KEY(tenant_id) REFERENCES installation_identity(tenant_id)
) STRICT;
CREATE TABLE grant_entry (
 principal_id TEXT NOT NULL,
 tenant_id TEXT NOT NULL,
 workspace_id TEXT NOT NULL,
 action TEXT NOT NULL,
 PRIMARY KEY(principal_id,workspace_id,action),
 FOREIGN KEY(principal_id,tenant_id) REFERENCES principal(principal_id,tenant_id),
 FOREIGN KEY(workspace_id,tenant_id) REFERENCES directory_registration(workspace_id,tenant_id)
) STRICT;
CREATE TABLE client_session (
 session_id TEXT PRIMARY KEY,
 token_hash TEXT NOT NULL UNIQUE CHECK(length(token_hash)=64),
 principal_id TEXT NOT NULL REFERENCES principal(principal_id),
 expires_at TEXT NOT NULL,
 revoked INTEGER NOT NULL CHECK(revoked IN (0,1)),
 grant_version INTEGER NOT NULL CHECK(grant_version>0)
) STRICT;
CREATE TABLE installation_operation (
 installation_id TEXT NOT NULL REFERENCES installation_identity(installation_id),
 operation_id TEXT NOT NULL,
 principal_id TEXT NOT NULL REFERENCES principal(principal_id),
 workspace_id TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('create','register','relink')),
 target_locator TEXT NOT NULL,
 input_digest TEXT NOT NULL CHECK(length(input_digest)=64),
 phase TEXT NOT NULL CHECK(phase IN ('intentRecorded','directoryPrepared','genesisConfirmed','descriptorPublished','registrationConfirmed')),
 outcome TEXT NOT NULL CHECK(outcome IN ('accepted','committed','rejected','unknown')),
 PRIMARY KEY(installation_id,operation_id)
) STRICT;
CREATE TABLE installation_binding_intent (
 installation_id TEXT NOT NULL,
 operation_id TEXT NOT NULL,
 tenant_id TEXT NOT NULL REFERENCES installation_identity(tenant_id),
 allowed_location TEXT NOT NULL,
 identity_digest TEXT NOT NULL CHECK(length(identity_digest)=64),
 root_volume TEXT NOT NULL,
 root_file_id BLOB NOT NULL CHECK(length(root_file_id)=16),
 database_file_id BLOB NOT NULL CHECK(length(database_file_id)=16),
 descriptor_file_id BLOB NOT NULL CHECK(length(descriptor_file_id)=16),
 previous_locator TEXT NOT NULL,
 expected_registration_version INTEGER NOT NULL CHECK(expected_registration_version>=0),
 PRIMARY KEY(installation_id,operation_id),
 FOREIGN KEY(installation_id,operation_id) REFERENCES installation_operation(installation_id,operation_id)
) STRICT;
CREATE TABLE installation_creation_intent (
 installation_id TEXT NOT NULL,
 operation_id TEXT NOT NULL,
 tenant_id TEXT NOT NULL REFERENCES installation_identity(tenant_id),
 display_name TEXT NOT NULL CHECK(length(display_name) BETWEEN 1 AND 256),
 allowed_location TEXT NOT NULL,
 identity_digest TEXT NOT NULL CHECK(length(identity_digest)=64),
 root_volume TEXT NOT NULL DEFAULT '',
 root_file_id BLOB CHECK(root_file_id IS NULL OR length(root_file_id)=16),
 database_file_id BLOB CHECK(database_file_id IS NULL OR length(database_file_id)=16),
 descriptor_file_id BLOB CHECK(descriptor_file_id IS NULL OR length(descriptor_file_id)=16),
 PRIMARY KEY(installation_id,operation_id),
 FOREIGN KEY(installation_id,operation_id) REFERENCES installation_operation(installation_id,operation_id)
) STRICT;
