Feature: Plugin release and worker start
  As a leader who starts workers
  I want to know that the omp installation of the worker understands the invite
  So that a worker does not die at startup on an unknown flag while I watch a tab open and close

  # The plugin is distributed through GitHub: "omp plugin install
  # github:till-muellers/agent-message-bus" installs it as a git dependency and
  # "omp plugin upgrade @till-muellers/omp-agent-message-bus" moves the pin to a
  # newer commit. There is no npm registry package.
  #
  # omp rejects unknown flags before any extension runs: a worker started with
  # "omp --bus-url <url> --bus-accept <code>" exits with code 2 and the message
  # "Error: unknown flag: --bus-accept" when the loaded plugin predates the
  # invite codes. The flags are registered by the extension
  # (extension/index.ts:200-201), so the worker needs a plugin that registers
  # them.
  #
  # Two things make stale and fresh distinguishable:
  #   1. every release commit bumps the plugin version in package.json, so the
  #      version omp reports for the installed copy changes with the code, and
  #   2. the extension knows the protocol version it implements, so it can
  #      compare it with the version of the plugin the worker will load.
  #
  # A spawn checks that comparison BEFORE it invites anybody, so a doomed spawn
  # does not burn an invite code and its 10-minute lifetime.
  # Decided (PO-11, round 6): S1, this spawn-time version check.

  Background:
    Given the admin minted leader "lead"
    And leader "lead" created space "demo"
    And the installed omp plugin is version "0.2.0"
    And the extension protocol version is "0.2.0"

  Rule: A spawn checks the worker's plugin version before it invites

    # The worker loads the extension the leader forwards, if any, and otherwise
    # the installed plugin. The leader knows both: its own forwarded extension
    # path and the installed plugin version reported by "omp plugin list".

    Scenario: A worker is started when the worker's plugin understands the invite
      When leader "lead" starts worker "worker" in space "demo" with task "fix the bug"
      Then the call returns name "worker", space "demo", launcher "orca"
      And an invite code was created for member "worker" in space "demo"

    Scenario: A worker is not started when the worker's plugin is too old
      Given the installed omp plugin is version "0.1.0"
      When leader "lead" starts worker "worker" in space "demo" with task "fix the bug"
      Then the call fails with "the worker would load omp plugin 0.1.0, which is older than the protocol 0.2.0; run omp plugin upgrade @till-muellers/omp-agent-message-bus"
      And no invite code is stored
      And no terminal was opened

    Scenario: A forwarded extension decides the worker's plugin, not the installed one
      Given the leader forwards extension "0.2.0" to the worker
      And the installed omp plugin is version "0.1.0"
      When leader "lead" starts worker "worker" in space "demo"
      Then the call returns name "worker", space "demo", launcher "orca"

    Scenario: The spawn proceeds when the worker's plugin version cannot be read
      Given the worker's omp plugin version is unknown
      When leader "lead" starts worker "worker" in space "demo"
      Then the call returns name "worker", space "demo", launcher "orca"

  Rule: The versions in play are visible to the leader

    Scenario: The status shows the extension protocol version and the installed plugin version
      When leader "lead" asks for the bus status
      Then the status shows extension protocol version "0.2.0"
      And the status shows installed plugin version "0.2.0"

    Scenario: The status warns when the installed plugin is older than the protocol
      Given the installed omp plugin is version "0.1.0"
      When leader "lead" asks for the bus status
      Then the status shows installed plugin version "0.1.0"
      And the status warns "the installed omp plugin 0.1.0 is older than the extension protocol 0.2.0; run omp plugin upgrade @till-muellers/omp-agent-message-bus"

  Rule: A GitHub release makes the new plugin version visible

    # The plugin version lives in package.json and is bumped in every release
    # commit; it is the single source of truth for the protocol version the
    # extension implements. A tag marks the release so the commit is
    # referenceable, and the release workflow checks that the tag and the
    # package version agree.

    Scenario: The plugin package version matches the release tag
      Given a release "v0.2.0" of the plugin
      Then the plugin package version is "0.2.0"
      And the extension protocol version is "0.2.0"

    Scenario: A release with a mismatched version is rejected
      Given a release "v0.2.0" of the plugin
      And the plugin package version is "0.1.0"
      Then the release fails with "package version 0.1.0 does not match tag v0.2.0"

    Scenario: The installed copy reports the version of the code it holds
      Given the installed omp plugin is version "0.2.0"
      When the plugin list is shown
      Then the plugin version is "0.2.0"

    Scenario: Upgrading moves the installed plugin to the newer release
      Given the installed omp plugin is version "0.1.0"
      When the plugin is upgraded
      Then the plugin version is "0.2.0"
      And the plugin registers the invite flags

    Scenario: The published plugin registers the invite flags
      Given the published plugin "0.2.0" is installed
      When omp is started with "--bus-url http://127.0.0.1:8080 --bus-accept k7m2-x9pq"
      Then omp does not fail with "unknown flag: --bus-accept"

  Rule: The extension refuses a server that is too old

    # Plugin and server are released separately (analysis.md, item 8, R6). The server
    # reports its protocol version (an integer, bumped with every change the extension
    # depends on) in the response header "Agent-Bus-Protocol" of GET /healthz; the body
    # stays "ok". The extension declares the minimum server protocol it needs and checks
    # it before it redeems an invite code or activates its tools. Example of the break it
    # prevents: the invite task is a direct message on the new server
    # (docs/design/specs/guidance/first-prompt.feature), so a new extension on an old server
    # would lose the task.
    # Values below are examples: the extension needs protocol 2, the old server has none.

    Scenario: The server reports its protocol version
      When a client sends GET /healthz without an Authorization header
      Then the response status is 200 and the body is "ok"
      And the response header "Agent-Bus-Protocol" is "2"

    Scenario: A server without a protocol header is refused before the invite is redeemed
      Given the bus at <url> answers GET /healthz without an "Agent-Bus-Protocol" header
      When "omp --bus-url <url> --bus-accept k7m2-x9pq" starts
      Then the extension does not post the code to /v1/invites/accept
      And the invite code is still unused
      And the session shows "agent-bus: the server at <url> is too old (protocol none, this plugin needs 2); upgrade the agent-message-bus server"

    Scenario: A server below the minimum protocol activates no bus tools
      Given the bus at <url> reports protocol "1"
      When the user runs "/bus login <url> <token>"
      Then no bus tool is activated
      And the session shows "agent-bus: the server at <url> is too old (protocol 1, this plugin needs 2); upgrade the agent-message-bus server"

    Scenario: A server at or above the minimum is used
      Given the bus at <url> reports protocol "3"
      When "omp --bus-url <url> --bus-accept k7m2-x9pq" starts
      Then the code is redeemed and the bus tools are active

    Scenario: /bus status shows the server protocol
      Given the bus at <url> reports protocol "2"
      When leader "lead" asks for the bus status
      Then the status shows server protocol "2" and the minimum "2"
