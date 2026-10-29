-- GoGuacamole application schema.
-- Tokens rendered per driver: {{ID}} {{BOOL}} {{TS}} {{STR}} {{TEXT}} {{ENGINE}}
-- Timestamps are Unix epoch milliseconds (BIGINT) for portability.

CREATE TABLE users (
    id {{ID}},
    username {{STR}}(128) NOT NULL,
    password_hash {{STR}}(255) NOT NULL,
    password_scheme {{STR}}(16) NOT NULL DEFAULT 'bcrypt',
    password_salt {{STR}}(64),
    password_changed_at {{TS}} NOT NULL,
    must_change_password {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    disabled {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    valid_from {{STR}}(10),
    valid_until {{STR}}(10),
    access_window_start {{STR}}(8),
    access_window_end {{STR}}(8),
    timezone {{STR}}(64),
    full_name {{STR}}(256),
    email {{STR}}(256),
    security_stamp {{STR}}(64) NOT NULL,
    created_at {{TS}} NOT NULL,
    updated_at {{TS}} NOT NULL,
    CONSTRAINT users_username_uq UNIQUE (username)
){{ENGINE}};

CREATE TABLE user_groups (
    id {{ID}},
    name {{STR}}(128) NOT NULL,
    disabled {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    created_at {{TS}} NOT NULL,
    CONSTRAINT user_groups_name_uq UNIQUE (name)
){{ENGINE}};

CREATE TABLE user_group_members (
    group_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    PRIMARY KEY (group_id, user_id),
    CONSTRAINT ugm_group_fk FOREIGN KEY (group_id) REFERENCES user_groups (id) ON DELETE CASCADE,
    CONSTRAINT ugm_user_fk FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
){{ENGINE}};

CREATE TABLE connection_groups (
    id {{ID}},
    parent_id BIGINT,
    name {{STR}}(128) NOT NULL,
    created_by BIGINT,
    created_at {{TS}} NOT NULL,
    CONSTRAINT cg_parent_fk FOREIGN KEY (parent_id) REFERENCES connection_groups (id) ON DELETE CASCADE,
    CONSTRAINT cg_creator_fk FOREIGN KEY (created_by) REFERENCES users (id) ON DELETE SET NULL
){{ENGINE}};

CREATE TABLE connections (
    id {{ID}},
    parent_id BIGINT,
    name {{STR}}(128) NOT NULL,
    protocol {{STR}}(32) NOT NULL,
    guacd_host {{STR}}(255),
    guacd_port INTEGER,
    guacd_encryption {{STR}}(8),
    max_connections INTEGER,
    max_connections_per_user INTEGER,
    created_by BIGINT,
    created_at {{TS}} NOT NULL,
    updated_at {{TS}} NOT NULL,
    CONSTRAINT conn_parent_fk FOREIGN KEY (parent_id) REFERENCES connection_groups (id) ON DELETE CASCADE,
    CONSTRAINT conn_creator_fk FOREIGN KEY (created_by) REFERENCES users (id) ON DELETE SET NULL
){{ENGINE}};

CREATE TABLE connection_parameters (
    connection_id BIGINT NOT NULL,
    name {{STR}}(128) NOT NULL,
    value {{TEXT}} NOT NULL,
    PRIMARY KEY (connection_id, name),
    CONSTRAINT cp_conn_fk FOREIGN KEY (connection_id) REFERENCES connections (id) ON DELETE CASCADE
){{ENGINE}};

CREATE TABLE system_permissions (
    subject_type {{STR}}(8) NOT NULL,
    subject_id BIGINT NOT NULL,
    permission {{STR}}(32) NOT NULL,
    PRIMARY KEY (subject_type, subject_id, permission)
){{ENGINE}};

CREATE TABLE connection_permissions (
    subject_type {{STR}}(8) NOT NULL,
    subject_id BIGINT NOT NULL,
    connection_id BIGINT NOT NULL,
    PRIMARY KEY (subject_type, subject_id, connection_id),
    CONSTRAINT cperm_conn_fk FOREIGN KEY (connection_id) REFERENCES connections (id) ON DELETE CASCADE
){{ENGINE}};

CREATE TABLE connection_group_permissions (
    subject_type {{STR}}(8) NOT NULL,
    subject_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL,
    PRIMARY KEY (subject_type, subject_id, group_id),
    CONSTRAINT cgperm_group_fk FOREIGN KEY (group_id) REFERENCES connection_groups (id) ON DELETE CASCADE
){{ENGINE}};

CREATE TABLE favorites (
    user_id BIGINT NOT NULL,
    connection_id BIGINT NOT NULL,
    created_at {{TS}} NOT NULL,
    PRIMARY KEY (user_id, connection_id),
    CONSTRAINT fav_user_fk FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fav_conn_fk FOREIGN KEY (connection_id) REFERENCES connections (id) ON DELETE CASCADE
){{ENGINE}};

CREATE TABLE app_sessions (
    id {{STR}}(64) NOT NULL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    csrf_token {{STR}}(64) NOT NULL,
    security_stamp {{STR}}(64) NOT NULL,
    created_at {{TS}} NOT NULL,
    last_seen {{TS}} NOT NULL,
    expires_at {{TS}} NOT NULL,
    client_ip {{STR}}(64),
    user_agent {{STR}}(512),
    CONSTRAINT sess_user_fk FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
){{ENGINE}};

CREATE TABLE preferences (
    user_id BIGINT NOT NULL PRIMARY KEY,
    theme {{STR}}(32),
    locale {{STR}}(16),
    sidebar_collapsed {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    CONSTRAINT pref_user_fk FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
){{ENGINE}};

CREATE TABLE connection_history (
    id {{ID}},
    user_id BIGINT,
    username {{STR}}(128) NOT NULL,
    connection_id BIGINT,
    connection_name {{STR}}(128) NOT NULL,
    protocol {{STR}}(32) NOT NULL,
    client_ip {{STR}}(64),
    tunnel_id {{STR}}(64) NOT NULL,
    start_at {{TS}} NOT NULL,
    end_at {{TS}},
    close_reason {{STR}}(64),
    CONSTRAINT hist_user_fk FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT hist_conn_fk FOREIGN KEY (connection_id) REFERENCES connections (id) ON DELETE SET NULL
){{ENGINE}};

CREATE INDEX hist_start_idx ON connection_history (start_at);

CREATE TABLE audit_events (
    id {{ID}},
    at {{TS}} NOT NULL,
    actor_id BIGINT,
    actor_name {{STR}}(128),
    client_ip {{STR}}(64),
    action {{STR}}(64) NOT NULL,
    target_type {{STR}}(32),
    target_id {{STR}}(64),
    target_name {{STR}}(256),
    outcome {{STR}}(16) NOT NULL,
    metadata {{TEXT}},
    CONSTRAINT audit_actor_fk FOREIGN KEY (actor_id) REFERENCES users (id) ON DELETE SET NULL
){{ENGINE}};

CREATE INDEX audit_at_idx ON audit_events (at);

CREATE TABLE system_state (
    name {{STR}}(64) NOT NULL PRIMARY KEY,
    value {{STR}}(255) NOT NULL
){{ENGINE}};

INSERT INTO system_state (name, value) VALUES ('access_epoch', '0');
