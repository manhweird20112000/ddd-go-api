#!/bin/sh
set -eu

psql -h postgres -U template -d template -v ON_ERROR_STOP=1 \
  -v identity_password="$IDENTITY_DB_PASSWORD" \
  -v notification_password="$NOTIFICATION_DB_PASSWORD" <<'SQL'
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'identity_app') THEN
    CREATE ROLE identity_app LOGIN;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'notification_app') THEN
    CREATE ROLE notification_app LOGIN;
  END IF;
END $$;
ALTER ROLE identity_app PASSWORD :'identity_password';
ALTER ROLE notification_app PASSWORD :'notification_password';
CREATE SCHEMA IF NOT EXISTS identity;
CREATE SCHEMA IF NOT EXISTS notification;
REVOKE ALL ON SCHEMA identity FROM PUBLIC;
REVOKE ALL ON SCHEMA notification FROM PUBLIC;
GRANT USAGE ON SCHEMA identity TO identity_app;
GRANT USAGE ON SCHEMA notification TO notification_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA identity TO identity_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA notification TO notification_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA identity TO identity_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA notification TO notification_app;
ALTER DEFAULT PRIVILEGES FOR ROLE template IN SCHEMA identity
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO identity_app;
ALTER DEFAULT PRIVILEGES FOR ROLE template IN SCHEMA notification
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO notification_app;
ALTER DEFAULT PRIVILEGES FOR ROLE template IN SCHEMA identity
  GRANT USAGE, SELECT ON SEQUENCES TO identity_app;
ALTER DEFAULT PRIVILEGES FOR ROLE template IN SCHEMA notification
  GRANT USAGE, SELECT ON SEQUENCES TO notification_app;
SQL
