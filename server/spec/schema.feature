Feature: One fresh schema as the service's first iteration
  As the maintainer of an alpha bus
  I want the schema collapsed into one migration that knows the admin kind
  So that the migration history starts clean

  # server/migrations/0001_schema.up.sql holds the whole schema, marked irreversible
  # (no .down.sql). From 0002 on, "never edit an applied migration" applies again.
  #
  # Accepted risk: an existing pre-reset database already has version 1 recorded, so
  # the runner silently skips the new 0001 and the first admin write fails with a CHECK
  # constraint error. Operators delete a pre-reset database by hand:
  #   "Pre-release databases are not upgraded: stop the server and delete
  #    agent-message-bus.db, -wal and -shm before starting this version."

  Rule: A fresh database gets the whole schema in one migration

    Scenario: The first start creates the schema
      Given AMB_DB_PATH points to a file that does not exist
      When the server starts
      Then schema_migrations holds exactly version 1
      And the tables are agents, spaces, tokens, participants, topics, topic_subscriptions, blackboards, threads, posts, thread_subscriptions, inbox and invites

    Scenario: agents accept the kinds admin, leader and member
      Given a fresh database
      Then an agents row with kind "admin" can be stored
      And an agents row with kind "owner" is rejected by the CHECK constraint

    Scenario: participants accept the roles admin, leader and member
      Given a fresh database
      Then a participants row with role "admin" can be stored
      And a participants row with role "owner" is rejected by the CHECK constraint

    Scenario: A second start applies nothing
      Given the server started once on the database
      When the server starts again
      Then schema_migrations still holds exactly version 1

  Rule: Later schema changes are numbered migrations again

    Scenario: A later migration applies on top of 0001_schema
      Given a database created by 0001_schema
      And a new migration 0002_example.up.sql
      When the server starts
      Then 0002_example is applied once
