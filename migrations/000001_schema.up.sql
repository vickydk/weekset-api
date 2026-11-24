CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE users (
    id uuid PRIMARY KEY,
    email text NOT NULL,
    password_hash text NOT NULL,
    display_name text NOT NULL,
    account_type text NOT NULL CHECK (account_type IN ('client', 'trainer', 'manager', 'center')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_email_key ON users (lower(email));

CREATE TABLE fitness_centers (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    timezone text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE branches (
    id uuid PRIMARY KEY,
    fitness_center_id uuid NOT NULL REFERENCES fitness_centers (id),
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, fitness_center_id)
);

CREATE INDEX branches_fitness_center_id_idx ON branches (fitness_center_id);

CREATE TABLE memberships (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id),
    fitness_center_id uuid NOT NULL REFERENCES fitness_centers (id),
    role text NOT NULL CHECK (role IN ('client', 'trainer', 'manager', 'center_admin')),
    status text NOT NULL CHECK (status IN ('active', 'suspended')),
    branch_id uuid REFERENCES branches (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, fitness_center_id),
    UNIQUE (id, fitness_center_id),
    CHECK (role <> 'trainer' OR branch_id IS NOT NULL),
    CHECK (role <> 'center_admin' OR branch_id IS NULL),
    FOREIGN KEY (branch_id, fitness_center_id) REFERENCES branches (id, fitness_center_id)
);

CREATE UNIQUE INDEX memberships_one_staff_membership_idx
    ON memberships (user_id)
    WHERE role IN ('trainer', 'manager', 'center_admin');

CREATE INDEX memberships_fitness_center_id_idx ON memberships (fitness_center_id);
CREATE INDEX memberships_branch_id_idx ON memberships (branch_id);

CREATE TABLE manager_branches (
    membership_id uuid NOT NULL,
    fitness_center_id uuid NOT NULL,
    branch_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (membership_id, branch_id),
    FOREIGN KEY (membership_id, fitness_center_id)
        REFERENCES memberships (id, fitness_center_id) ON DELETE CASCADE,
    FOREIGN KEY (branch_id, fitness_center_id)
        REFERENCES branches (id, fitness_center_id)
);

CREATE INDEX manager_branches_fitness_center_id_idx ON manager_branches (fitness_center_id);
CREATE INDEX manager_branches_branch_id_idx ON manager_branches (branch_id);

CREATE TABLE trainer_assignments (
    id uuid PRIMARY KEY,
    fitness_center_id uuid NOT NULL,
    branch_id uuid NOT NULL,
    client_user_id uuid NOT NULL REFERENCES users (id),
    trainer_user_id uuid NOT NULL REFERENCES users (id),
    client_membership_id uuid NOT NULL REFERENCES memberships (id) ON DELETE CASCADE,
    trainer_membership_id uuid NOT NULL REFERENCES memberships (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (fitness_center_id, client_user_id),
    FOREIGN KEY (branch_id, fitness_center_id)
        REFERENCES branches (id, fitness_center_id)
);

CREATE INDEX trainer_assignments_fitness_center_id_idx ON trainer_assignments (fitness_center_id);
CREATE INDEX trainer_assignments_branch_id_idx ON trainer_assignments (branch_id);

CREATE TABLE availability_rules (
    id uuid PRIMARY KEY,
    fitness_center_id uuid NOT NULL REFERENCES fitness_centers (id),
    branch_id uuid NOT NULL,
    trainer_user_id uuid NOT NULL REFERENCES users (id),
    weekday smallint NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    local_start time NOT NULL,
    local_end time NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (local_end > local_start),
    FOREIGN KEY (branch_id, fitness_center_id)
        REFERENCES branches (id, fitness_center_id)
);

CREATE INDEX availability_rules_fitness_center_id_idx ON availability_rules (fitness_center_id);
CREATE INDEX availability_rules_branch_id_idx ON availability_rules (branch_id);

CREATE TABLE availability_exceptions (
    id uuid PRIMARY KEY,
    fitness_center_id uuid NOT NULL REFERENCES fitness_centers (id),
    branch_id uuid NOT NULL,
    trainer_user_id uuid NOT NULL REFERENCES users (id),
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at),
    FOREIGN KEY (branch_id, fitness_center_id)
        REFERENCES branches (id, fitness_center_id)
);

CREATE INDEX availability_exceptions_fitness_center_id_idx ON availability_exceptions (fitness_center_id);
CREATE INDEX availability_exceptions_branch_id_idx ON availability_exceptions (branch_id);

CREATE TABLE booking_series (
    id uuid PRIMARY KEY,
    fitness_center_id uuid NOT NULL REFERENCES fitness_centers (id),
    branch_id uuid NOT NULL,
    trainer_user_id uuid NOT NULL REFERENCES users (id),
    client_user_id uuid NOT NULL REFERENCES users (id),
    weekday smallint NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    local_start time NOT NULL,
    local_end time NOT NULL,
    starts_on date NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (local_end > local_start),
    UNIQUE (id, fitness_center_id, branch_id),
    FOREIGN KEY (branch_id, fitness_center_id)
        REFERENCES branches (id, fitness_center_id)
);

CREATE INDEX booking_series_fitness_center_id_idx ON booking_series (fitness_center_id);
CREATE INDEX booking_series_branch_id_idx ON booking_series (branch_id);

CREATE TABLE booking_series_exceptions (
    id uuid PRIMARY KEY,
    series_id uuid NOT NULL,
    fitness_center_id uuid NOT NULL,
    branch_id uuid NOT NULL,
    occurrence_on date NOT NULL,
    kind text NOT NULL CHECK (kind IN ('skip', 'move')),
    starts_at timestamptz,
    ends_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (series_id, occurrence_on),
    CHECK (
        (kind = 'skip' AND starts_at IS NULL AND ends_at IS NULL)
        OR (kind = 'move' AND starts_at IS NOT NULL AND ends_at IS NOT NULL AND ends_at > starts_at)
    ),
    FOREIGN KEY (series_id, fitness_center_id, branch_id)
        REFERENCES booking_series (id, fitness_center_id, branch_id) ON DELETE CASCADE
);

CREATE INDEX booking_series_exceptions_fitness_center_id_idx ON booking_series_exceptions (fitness_center_id);
CREATE INDEX booking_series_exceptions_branch_id_idx ON booking_series_exceptions (branch_id);

CREATE TABLE bookings (
    id uuid PRIMARY KEY,
    fitness_center_id uuid NOT NULL REFERENCES fitness_centers (id),
    branch_id uuid NOT NULL,
    trainer_user_id uuid NOT NULL REFERENCES users (id),
    client_user_id uuid NOT NULL REFERENCES users (id),
    series_id uuid REFERENCES booking_series (id),
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    status text NOT NULL CHECK (status IN ('requested', 'confirmed', 'declined', 'cancelled', 'completed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at),
    FOREIGN KEY (branch_id, fitness_center_id)
        REFERENCES branches (id, fitness_center_id),
    CONSTRAINT bookings_no_overlap EXCLUDE USING gist (
        trainer_user_id WITH =,
        tstzrange(starts_at, ends_at, '[)') WITH &&
    ) WHERE (status IN ('requested', 'confirmed'))
);

CREATE INDEX bookings_fitness_center_id_idx ON bookings (fitness_center_id);
CREATE INDEX bookings_branch_id_idx ON bookings (branch_id);

CREATE TABLE session_logs (
    id uuid PRIMARY KEY,
    booking_id uuid NOT NULL UNIQUE REFERENCES bookings (id),
    fitness_center_id uuid NOT NULL,
    branch_id uuid NOT NULL,
    note text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (branch_id, fitness_center_id)
        REFERENCES branches (id, fitness_center_id)
);

CREATE INDEX session_logs_fitness_center_id_idx ON session_logs (fitness_center_id);
CREATE INDEX session_logs_branch_id_idx ON session_logs (branch_id);

CREATE TABLE session_log_sets (
    id uuid PRIMARY KEY,
    session_log_id uuid NOT NULL REFERENCES session_logs (id) ON DELETE CASCADE,
    fitness_center_id uuid NOT NULL,
    branch_id uuid NOT NULL,
    position integer NOT NULL CHECK (position >= 0),
    exercise_name text NOT NULL,
    reps integer NOT NULL CHECK (reps > 0),
    load text NOT NULL,
    unit text NOT NULL CHECK (unit IN ('kg')),
    FOREIGN KEY (branch_id, fitness_center_id)
        REFERENCES branches (id, fitness_center_id)
);

CREATE INDEX session_log_sets_fitness_center_id_idx ON session_log_sets (fitness_center_id);
CREATE INDEX session_log_sets_branch_id_idx ON session_log_sets (branch_id);
CREATE INDEX session_log_sets_session_log_id_idx ON session_log_sets (session_log_id);

CREATE TABLE exercises (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE,
    name text NOT NULL
);

INSERT INTO exercises (slug, name) VALUES
    ('squat', 'squat'),
    ('hip-hinge', 'hip hinge'),
    ('lunge', 'lunge'),
    ('glute-bridge', 'glute bridge'),
    ('push-up', 'push-up'),
    ('dumbbell-bench-press', 'dumbbell bench press'),
    ('seated-row', 'seated row'),
    ('lat-pulldown', 'lat pulldown'),
    ('overhead-press', 'overhead press'),
    ('plank', 'plank'),
    ('dead-bug', 'dead bug'),
    ('farmer-carry', 'farmer carry');

CREATE TABLE solo_plans (
    id uuid PRIMARY KEY,
    fitness_center_id uuid NOT NULL REFERENCES fitness_centers (id),
    branch_id uuid NOT NULL,
    client_user_id uuid NOT NULL REFERENCES users (id),
    trainer_user_id uuid NOT NULL REFERENCES users (id),
    plan_date date NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (fitness_center_id, client_user_id, plan_date),
    FOREIGN KEY (branch_id, fitness_center_id)
        REFERENCES branches (id, fitness_center_id)
);

CREATE INDEX solo_plans_fitness_center_id_idx ON solo_plans (fitness_center_id);
CREATE INDEX solo_plans_branch_id_idx ON solo_plans (branch_id);

CREATE TABLE solo_plan_items (
    id uuid PRIMARY KEY,
    solo_plan_id uuid NOT NULL REFERENCES solo_plans (id) ON DELETE CASCADE,
    exercise_id uuid NOT NULL REFERENCES exercises (id),
    fitness_center_id uuid NOT NULL,
    branch_id uuid NOT NULL,
    position integer NOT NULL CHECK (position >= 0),
    sets integer NOT NULL CHECK (sets > 0),
    reps integer NOT NULL CHECK (reps > 0),
    rest_seconds integer NOT NULL CHECK (rest_seconds >= 0),
    FOREIGN KEY (branch_id, fitness_center_id)
        REFERENCES branches (id, fitness_center_id)
);

CREATE INDEX solo_plan_items_fitness_center_id_idx ON solo_plan_items (fitness_center_id);
CREATE INDEX solo_plan_items_branch_id_idx ON solo_plan_items (branch_id);

CREATE TABLE notifications (
    id uuid PRIMARY KEY,
    fitness_center_id uuid NOT NULL REFERENCES fitness_centers (id),
    user_id uuid NOT NULL REFERENCES users (id),
    kind text NOT NULL,
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX notifications_fitness_center_id_idx ON notifications (fitness_center_id);

CREATE TABLE audit_events (
    id uuid PRIMARY KEY,
    fitness_center_id uuid NOT NULL REFERENCES fitness_centers (id),
    actor_user_id uuid NOT NULL REFERENCES users (id),
    action text NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_events_fitness_center_id_idx ON audit_events (fitness_center_id);

CREATE TABLE invites (
    id uuid PRIMARY KEY,
    fitness_center_id uuid NOT NULL REFERENCES fitness_centers (id),
    email text NOT NULL,
    role text NOT NULL CHECK (role IN ('client', 'trainer', 'manager')),
    branch_ids uuid[] NOT NULL,
    code_hash text NOT NULL,
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX invites_fitness_center_id_idx ON invites (fitness_center_id);

CREATE TABLE refresh_tokens (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id),
    token_hash text NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE password_resets (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id),
    token_hash text NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
