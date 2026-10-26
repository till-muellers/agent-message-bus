Feature: The bootstrap admin and the instance admin token
  As the operator of a bus
  I want a bootstrap admin from server startup
  So that the instance admin token is a named admin identity

  # The bootstrap admin:
  #   - is the agent "admin", created on the first start of a database (an existing
  #     row of kind admin is reused; a row of another kind makes the server refuse to start);
  #   - authenticates with the instance admin token: AMB_ADMIN_TOKEN if set, else
  #     <datadir>/admin-token. That token is only held in memory as a hash and never
  #     stored in the database.
  # Agent names stay global, so a name is a leader, a member or an admin, never two of them.

  Background:
    Given the bus was started with the admin token "admin-token-0123456789-0123456789-abcdef"

  Rule: The instance admin token is the bootstrap admin "admin"

    Scenario: The instance admin token authenticates as the bootstrap admin
      When the bootstrap admin asks who it is
      Then the call returns kind "admin", name "admin" and spaces []

    Scenario: The bootstrap admin exists once, created on the first start
      Then the bus holds exactly one agent named "admin" with kind "admin"

    Scenario: A restart reuses the bootstrap admin
      When the server restarts with the same database
      Then the bus still holds exactly one agent named "admin"

    Scenario: The instance admin token is not stored in the database
      Then no token record holds the SHA-256 of "admin-token-0123456789-0123456789-abcdef"

    Scenario: "admin" cannot be minted as a leader
      When the bootstrap admin mints leader "admin"
      Then the call fails with conflict "agent admin is an admin agent"
