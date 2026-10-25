Feature: Local instance management
  As a developer running the bus on my machine
  I want start, stop, status and token commands
  So that one instance per data dir runs without manual pid or token handling

  # One instance runs per data dir. The state file <datadir>/agent-message-bus.state.json
  # holds {"pid","url","startedAt"} while an instance runs; the log appends to
  # <datadir>/agent-message-bus.log. The admin token comes from AMB_ADMIN_TOKEN if set,
  # else from <datadir>/admin-token (0600, generated on first use).

  Background:
    Given an empty data dir
    And AMB_ADMIN_TOKEN is not set

  Rule: start runs one detached instance per data dir

    Scenario: start launches the server and waits until it is healthy
      When I run "agent-message-bus start"
      Then a detached server process is running
      And GET /healthz on its url returns 200
      And the state file holds its pid and url
      And the output is "agent-message-bus started (pid <pid>) at <url>; log: <datadir>/agent-message-bus.log"

    Scenario: start is idempotent
      Given an instance is running with pid 42 at "http://127.0.0.1:8080"
      When I run "agent-message-bus start"
      Then no new process is started
      And the output is "agent-message-bus already running (pid 42) at http://127.0.0.1:8080"
      And the command succeeds

    Scenario: A foreground run fails while an instance is live
      Given an instance is running with pid 42 at "http://127.0.0.1:8080"
      When I run "agent-message-bus" without a command
      Then it fails with "agent-message-bus already running (pid 42) at http://127.0.0.1:8080"

    Scenario: A stale state file is cleaned up
      Given the state file names pid 42 at a url that doesn't answer /healthz
      When I run "agent-message-bus status"
      Then the output is "not running"
      And the state file is removed

  Rule: status reports the instance

    Scenario: status as text
      Given an instance is running with pid 42 at "http://127.0.0.1:8080"
      When I run "agent-message-bus status"
      Then the output is "running (pid 42) at http://127.0.0.1:8080"

    Scenario: status as JSON
      Given an instance is running with pid 42 at "http://127.0.0.1:8080"
      When I run "agent-message-bus status --json"
      Then the output is {"running":true,"pid":42,"url":"http://127.0.0.1:8080"}

    Scenario: status when nothing runs
      When I run "agent-message-bus status --json"
      Then the output is {"running":false}

  Rule: stop shuts the instance down gracefully

    Scenario: stop asks the server to shut down
      Given an instance is running with pid 42 at "http://127.0.0.1:8080"
      When I run "agent-message-bus stop"
      Then it sends POST /v1/admin/shutdown with the admin token
      And the process exits gracefully
      And the state file is gone
      And the output is "agent-message-bus stopped (pid 42)"

    Scenario: stop kills the process if graceful shutdown fails
      Given an instance is running that doesn't exit after POST /v1/admin/shutdown
      When I run "agent-message-bus stop"
      Then the process is killed
      And the state file is gone

    Scenario: stop when not running
      When I run "agent-message-bus stop"
      Then the output is "agent-message-bus is not running"
      And the command succeeds

  Rule: admin-token prints the admin token

    Scenario: The token file is generated when missing
      When I run "agent-message-bus admin-token"
      Then <datadir>/admin-token is created with mode 0600
      And the output is the token from that file

    Scenario: The token file persists
      Given <datadir>/admin-token holds a token
      When I run "agent-message-bus admin-token"
      Then the output is that token
      And the file is unchanged

    Scenario: The env token wins
      Given AMB_ADMIN_TOKEN is "env-token-0123456789-0123456789-abcdef"
      And <datadir>/admin-token holds a different token
      When I run "agent-message-bus admin-token"
      Then the output is "env-token-0123456789-0123456789-abcdef"

  Rule: rotate-token replaces the admin token live

    Scenario: rotate-token swaps the token on the running instance
      Given an instance is running with the token from <datadir>/admin-token
      When I run "agent-message-bus rotate-token"
      Then it sends POST /v1/admin/admin-token with the new token
      And <datadir>/admin-token holds the new token
      And the output is the new token
      And a call with the old token gets 401 unauthenticated
      And a call with the new token succeeds

    Scenario: rotate-token refuses when the env token is set
      Given AMB_ADMIN_TOKEN is "env-token-0123456789-0123456789-abcdef"
      When I run "agent-message-bus rotate-token"
      Then it fails with "AMB_ADMIN_TOKEN is set in the environment; rotate it there"
