Feature: Local instance management
  As a developer running the bus on my machine
  I want start, stop, status and token commands
  So that one instance per data dir runs without manual pid or token handling

  # One instance runs per data dir. The serving process holds the lock file
  # <datadir>/agent-message-bus.lock for its whole life (an OS lock, released when the process
  # ends, however it ends): lock held = an instance runs, lock free = none does. Once it
  # listens it publishes the state file <datadir>/agent-message-bus.state.json, written via a
  # temp file and a rename, holding {"pid","url","startedAt","instanceId"}; GET /healthz names
  # the instance in the Agent-Bus-Instance header. "An instance is running with pid 42 at <url>"
  # means it holds the lock and <url>/healthz answers with the state file's instanceId. The log
  # appends to <datadir>/agent-message-bus.log. The admin token comes from AMB_ADMIN_TOKEN if
  # set, else from <datadir>/admin-token (0600, generated on first use).

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

    Scenario: start stops its server when it does not become healthy
      Given the started server takes the data dir lock and publishes its pid but never answers /healthz with its instance id
      When I run "agent-message-bus start"
      Then after 10 s the spawned server process is killed
      And start fails with "agent-message-bus did not become healthy within 10s and was stopped (pid <pid>); log tail (<datadir>/agent-message-bus.log):" and the log tail
      And a server that exited on its own, or a lock holder whose state file is missing or names another pid, is not killed

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

    Scenario: A second server on the same data dir fails
      Given an instance is running with pid 42 at "http://127.0.0.1:8080"
      When a server starts on the same data dir with AMB_ADDR "127.0.0.1:8090"
      Then it fails with "agent-message-bus already running (pid 42) at http://127.0.0.1:8080"
      And it never opens the database

    Scenario: Two starts at the same moment run one instance
      When I run "agent-message-bus start" twice at the same moment, with AMB_ADDR 127.0.0.1:8080 and 127.0.0.1:8090
      Then exactly one server process serves the data dir
      And one start prints "agent-message-bus started (pid <pid>) at <url>; log: <datadir>/agent-message-bus.log"
      And the other prints "agent-message-bus already running (pid <pid>) at <url>"

    Scenario: The loser of a start race never takes the data dir over
      Given I run "agent-message-bus start" twice at the same moment, with AMB_ADDR 127.0.0.1:8080 and 127.0.0.1:8090
      And the server that lost has met the lock and exited with "agent-message-bus already running"
      When I run "agent-message-bus stop" less than a second later
      Then the instance the starts reported is stopped
      And no server process serves the data dir, and "agent-message-bus status" prints "not running"

    Scenario: start waits for a server that has not published its state yet
      Given a server holds the data dir lock but has not written the state file yet
      When I run "agent-message-bus start"
      Then no new process is started
      And once the server answers at its url the output is "agent-message-bus already running (pid <pid>) at <url>"
      And if it stays unconfirmed for 10 s, start fails with "agent-message-bus is running (state pending) but did not answer within 10s; retry, or end the process that holds <datadir>/agent-message-bus.lock"

    Scenario: A second server fails while the first has not published its state yet
      Given a server holds the data dir lock but has not written the state file yet
      When I run "agent-message-bus" without a command
      Then it fails with "agent-message-bus already running (state pending)"

    Scenario Outline: A state file without a lock holder is stale
      Given no instance holds the data dir lock
      And the state file <holds>
      When I run "agent-message-bus <command>"
      Then the output is "<output>"
      And the state file is removed

      Examples:
        | holds                                                      | command | output                           |
        | names pid 42 at a url that doesn't answer /healthz         | status  | not running                      |
        | is empty                                                   | status  | not running                      |
        | is not valid JSON                                          | stop    | agent-message-bus is not running |
        | names a url where another data dir's instance now answers  | status  | not running                      |

    Scenario: start starts over an unparseable stale state file
      Given no instance holds the data dir lock
      And the state file is empty
      When I run "agent-message-bus start"
      Then a detached server process is running
      And the state file holds its pid and url

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

    Scenario: status of a running instance not confirmed at its url
      Given a server holds the data dir lock
      And the state file is missing, unparseable, or names a url whose /healthz doesn't answer with its instanceId
      When I run "agent-message-bus status"
      Then the output is "running (state pending)"
      And the state file is left alone
      And "agent-message-bus status --json" prints {"running":true}

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
      Then the process is killed once it still holds the data dir lock after 10 s and the state file still names it
      And stop waits until the lock is released
      And the state file is gone

    Scenario: stop with a token the server rejects stops nothing
      Given an instance is running with pid 42 at "http://127.0.0.1:8080"
      And the admin token of this command (AMB_ADMIN_TOKEN, or <datadir>/admin-token) is not the instance's
      When I run "agent-message-bus stop"
      Then POST /v1/admin/shutdown is answered 401
      And it fails at once with "asking the instance to shut down: POST http://127.0.0.1:8080/v1/admin/shutdown: status 401: <body>; the admin token in <AMB_ADMIN_TOKEN or datadir>/admin-token is not the admin token of the running instance, nothing was stopped"
      And no process is killed and the instance keeps running

    Scenario: stop still kills an instance that answers 403 to its shutdown request
      Given an instance bound to a non-loopback address of this host runs with pid 42
      And POST /v1/admin/shutdown from this host is answered 403 "admin endpoints are only available from localhost"
      When I run "agent-message-bus stop"
      Then the process is killed once it still holds the data dir lock after 10 s and the state file still names it

    Scenario: stop kills nothing once another instance took the data dir over
      Given an instance is running with pid 42 at "http://127.0.0.1:8080"
      And after POST /v1/admin/shutdown the data dir lock stays held, but the state file names another instance
      When I run "agent-message-bus stop"
      Then no process is killed
      And it fails with "<datadir>/agent-message-bus.lock is still held after 10s, but the state file no longer names pid 42; nothing was killed"

    Scenario: stop never acts on an instance it cannot confirm
      Given a server holds the data dir lock
      And the state file names a url whose /healthz doesn't answer with its instanceId
      When I run "agent-message-bus stop"
      Then it sends no POST /v1/admin/shutdown and kills no process
      And after 10 s it fails with "agent-message-bus is running (state pending) but did not answer within 10s; retry, or end the process that holds <datadir>/agent-message-bus.lock"

    Scenario: stop leaves another data dir's instance alone
      Given the instance of this data dir was killed and left its state file
      And another data dir's instance now serves the url in that state file, with the same admin token
      When I run "agent-message-bus stop"
      Then the output is "agent-message-bus is not running"
      And the other instance got no request and keeps running
      And the state file is removed

    Scenario: stop when not running
      When I run "agent-message-bus stop"
      Then the output is "agent-message-bus is not running"
      And the command succeeds

  Rule: admin-token prints the bootstrap admin's token

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

    Scenario: The env token is trimmed like the file token
      Given AMB_ADMIN_TOKEN is " env-token-0123456789-0123456789-abcdef " with surrounding spaces
      When I run "agent-message-bus admin-token"
      Then the output is "env-token-0123456789-0123456789-abcdef"

    Scenario Outline: An unusable admin token is refused (<case>)
      # One rule for AMB_ADMIN_TOKEN, the token file and POST /v1/admin/admin-token: after
      # trimming, 32..512 visible ASCII characters, so every accepted token can be sent as
      # "Bearer <token>". A blank AMB_ADMIN_TOKEN does not fall back to the file. The server
      # itself refuses to start for the same reason (with start, the message is in the log tail).
      Given <source> holds <value>
      When I run "agent-message-bus admin-token"
      Then it fails with "<message>"

      Examples:
        | case              | source                | value                   | message                                                                                          |
        | short env token   | AMB_ADMIN_TOKEN       | "short"                 | load config: AMB_ADMIN_TOKEN: admin token must be 32..512 visible ASCII characters               |
        | blank env token   | AMB_ADMIN_TOKEN       | 40 spaces               | load config: AMB_ADMIN_TOKEN: admin token must be 32..512 visible ASCII characters               |
        | inner space, file | <datadir>/admin-token | "<16 chars> <16 chars>" | resolving admin token: admin token in <datadir>/admin-token: admin token must be 32..512 visible ASCII characters |
        | too long, file    | <datadir>/admin-token | 513 characters          | resolving admin token: admin token in <datadir>/admin-token: admin token must be 32..512 visible ASCII characters |

  Rule: rotate-token replaces the admin token live

    Scenario: rotate-token swaps the token on the running instance
      Given an instance is running with the token from <datadir>/admin-token
      When I run "agent-message-bus rotate-token"
      Then it sends POST /v1/admin/admin-token with the new token
      And <datadir>/admin-token holds the new token
      And the output is the new token
      And a call with the old token gets 401 unauthenticated
      And a call with the new token succeeds

    Scenario: Overlapping rotate-token runs take turns
      Given an instance is running with the token from <datadir>/admin-token
      When I run "agent-message-bus rotate-token" five times at the same moment
      Then every run succeeds, one after the other
      And <datadir>/admin-token holds the token the instance accepts

    Scenario: rotate-token gives up on a run that never ends
      Given another rotate-token holds <datadir>/admin-token.lock
      When I run "agent-message-bus rotate-token"
      Then after 10 s it fails with "another rotate-token still runs (<datadir>/admin-token.lock is held after 10s)"
      And <datadir>/admin-token is unchanged

    Scenario: rotate-token refuses when the env token is set
      Given AMB_ADMIN_TOKEN is "env-token-0123456789-0123456789-abcdef"
      When I run "agent-message-bus rotate-token"
      Then it fails with "AMB_ADMIN_TOKEN is set in the environment; rotate it there"

    Scenario: rotate-token leaves the token alone while the instance is not confirmed
      Given a server holds the data dir lock but its state file is not confirmed at its url for 10 s
      When I run "agent-message-bus rotate-token"
      Then it fails with "agent-message-bus is running (state pending) but did not answer within 10s; retry, or end the process that holds <datadir>/agent-message-bus.lock"
      And <datadir>/admin-token is unchanged

  Rule: AMB_REQUIRE_EXPLICIT stops any command from falling back to the default data dir

    # An opt-in guard for shells that must never touch the real data dir (coding agents running
    # throwaway servers): set it in their environment, not in the owner's. When it is true (1,
    # t, true, any ParseBool truth) every command (serve, start, stop, status, admin-token)
    # needs AMB_ADDR, AMB_WEB_ADDR, AMB_DB_PATH and AMB_ADMIN_TOKEN (rotate-token needs them
    # too but refuses anyway while AMB_ADMIN_TOKEN is set). A variable
    # counts as given when set; an empty value counts as missing except for AMB_WEB_ADDR (empty
    # disables the GUI). Unset, empty or false leaves the defaults in force.

    Scenario: The guard refuses a command that would use the defaults
      Given AMB_REQUIRE_EXPLICIT is "1"
      And AMB_ADDR, AMB_WEB_ADDR, AMB_DB_PATH and AMB_ADMIN_TOKEN are not set
      When I run "agent-message-bus start"
      Then it fails with "load config: AMB_REQUIRE_EXPLICIT is set: AMB_ADDR, AMB_WEB_ADDR, AMB_DB_PATH, AMB_ADMIN_TOKEN missing; set all four so the default data dir stays untouched"
      And no data dir, lock, state, log, database or token file is created

    Scenario Outline: The guard names exactly the missing variables
      Given AMB_REQUIRE_EXPLICIT is "1"
      And all four variables are set except <variable>, which is <state>
      When I run "agent-message-bus start"
      Then it fails with "load config: AMB_REQUIRE_EXPLICIT is set: <variable> missing; set all four so the default data dir stays untouched"

      Examples:
        | variable        | state |
        | AMB_ADDR        | unset |
        | AMB_WEB_ADDR    | unset |
        | AMB_DB_PATH     | empty |
        | AMB_ADMIN_TOKEN | empty |

    Scenario: An empty AMB_WEB_ADDR satisfies the guard
      Given AMB_REQUIRE_EXPLICIT is "1"
      And AMB_ADDR, AMB_DB_PATH and AMB_ADMIN_TOKEN are set and AMB_WEB_ADDR is empty
      When I run "agent-message-bus start"
      Then the server starts without a web GUI listener

    Scenario: A guard value that is not a boolean fails closed
      Given AMB_REQUIRE_EXPLICIT is "maybe"
      When I run "agent-message-bus start"
      Then it fails with "load config: AMB_REQUIRE_EXPLICIT: \"maybe\" is not a boolean (use 1 or 0)"

    Scenario: Without the guard the defaults stay in force
      Given AMB_REQUIRE_EXPLICIT is unset or "0"
      And AMB_ADDR, AMB_WEB_ADDR, AMB_DB_PATH and AMB_ADMIN_TOKEN are not set
      When I run "agent-message-bus start"
      Then the server starts on the default data dir and ports

  Rule: a brief external write lock on the database delays calls, it does not fail them

    # Every transaction begins IMMEDIATE, so the 5 s busy timeout covers another
    # process (an operator's sqlite3 session, a backup tool) holding the write lock.

    Scenario: A mutating call waits for an external writer
      Given the server runs
      And another process holds the database write lock for 1 second
      When the admin calls bus_create_space with space "short0"
      Then the call succeeds after the lock is released
