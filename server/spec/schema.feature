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
      Then schema_migrations holds version 1 and the version of every later numbered migration
      And the tables are agents, spaces, tokens, participants, topics, topic_subscriptions, blackboards, threads, posts, thread_subscriptions, inbox and invites
      And the bootstrap admin "admin" exists

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
      Then schema_migrations still holds the same versions

  Rule: Later schema changes are numbered migrations again

    Scenario: A later migration applies on top of 0001_schema
      Given a database created by 0001_schema
      And a new migration 0002_example.up.sql
      When the server starts
      Then 0002_example is applied once

    Scenario: A migration rebuilds a table without losing the rows that point at it
      # Each migration runs with foreign keys off; ON DELETE actions do not fire inside it.
      Given a table "parent" with a row that table "child" references with ON DELETE CASCADE
      When a migration creates "parent_new", copies "parent" into it, drops "parent" and renames "parent_new" to "parent"
      Then the "child" row still exists
      And foreign keys are enforced again, so deleting the "parent" row deletes the "child" row

    Scenario: A migration that leaves a dangling foreign key is rolled back
      Given a table "child" that references table "parent"
      When a migration inserts a "child" row whose parent does not exist
      Then the server does not start and names that migration and the foreign key violation
      And schema_migrations does not record it

  Rule: A deleted space or thread never gives its id to a new one

    # spaces.id and threads.id are AUTOINCREMENT (migration NNNN_autoincrement_ids), so a
    # member token, inbox scope or stream of a deleted space never matches a later space,
    # and an old threadId never opens another thread.

    Scenario: A space created after the newest space was deleted gets a new id
      Given leader "lead" created space "oldsp"
      And leader "lead" invited member "reusex" to space "oldsp"
      And leader "lead" deleted space "oldsp"
      When leader "lead" creates space "newsp"
      And leader "lead" invites member "reusex" to space "newsp" and sends it a message
      Then space "newsp" has an id that "oldsp" never had
      And the mailbox scope of "reusex" in the deleted space "oldsp" holds nothing

    Scenario: A thread created after the newest thread was removed gets a new id
      Given member "m2" subscribed to thread "plan" of blackboard "b" in space "alpha"
      And thread "plan" was removed, so "m2" holds a thread_removed entry with its threadId
      When a new thread is created on blackboard "b"
      Then the new thread has a different id
      And bus_read_thread with the removed threadId fails with not_found "thread <id> not found"

    Scenario: The migration keeps every row, id and foreign key
      Given a database before NNNN_autoincrement_ids with rows in every table
      When the server starts
      Then every table holds the same rows, spaces and threads with the same ids
      And deleting a space still deletes its tokens, participants, topics, blackboards, threads, posts, subscriptions, inbox entries and invites
      And its down migration restores the plain INTEGER PRIMARY KEY and keeps every row
