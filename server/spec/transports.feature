Feature: Transports
  As an agent or tool integrator
  I want the same tool registry over a JSON web API and over MCP
  So that any client can call the bus with a bearer token

  # Routes:
  #   GET  /healthz             no auth
  #   GET  /v1/tools            bearer auth
  #   POST /v1/tools/{name}     bearer auth
  #   GET  /v1/inbox/stream     bearer auth (see inbox.feature)
  #   GET  /v1/guide            bearer auth (see guide.feature)
  #   /mcp                      MCP streamable HTTP, bearer auth
  #   POST /v1/admin/*          loopback + admin token (see "Admin endpoints")
  # Error body: {"error":{"code":"<code>","message":"<message>"}}

  Background:
    Given the bus is running with admin token "admin-token-0123456789-0123456789-abcdef"

  Rule: Health check

    Scenario: The health check needs no token
      When a client sends GET /healthz without an Authorization header
      Then the response status is 200
      And the "Content-Type" header is "text/plain; charset=utf-8"
      And the response body is "ok"

    Scenario: The server reports its protocol version
      When a client sends GET /healthz without an Authorization header
      Then the response status is 200 and the body is "ok"
      And the response header "Agent-Bus-Protocol" is "2"

  Rule: Bearer authentication on the web API

    Scenario Outline: Authorization header "<header>"
      When a client calls GET /v1/tools with Authorization header "<header>"
      Then the response status is <status>
      And the error is <outcome>

      Examples:
        | header                  | status | outcome                                     |
        | Bearer <admin token>    | 200    | absent; the caller is the admin             |
        | Bearer <leader token>   | 200    | absent; the caller is the leader            |
        |                         | 401    | unauthenticated "bearer token required"     |
        | Basic <leader token>    | 401    | unauthenticated "bearer token required"     |
        | Bearer                  | 401    | unauthenticated "bearer token required"     |
        | Bearer amb_nope         | 401    | unauthenticated "invalid token"             |

    Scenario: Calling a tool without a token
      When a client calls POST /v1/tools/bus_whoami without an Authorization header
      Then the response status is 401
      And the call fails with unauthenticated "bearer token required"

    Scenario: The tool handler without an authenticated caller
      # Unreachable through the server routes, which authenticate first.
      When the tool call handler runs without an authenticated caller
      Then the call fails with unauthenticated "bearer token required"

  Rule: Listing tools (GET /v1/tools)

    Scenario: The list holds all 34 tools sorted by name
      When the admin calls GET /v1/tools
      Then the response status is 200
      And the response is a JSON array of 34 objects with "name", "description" and "inputSchema"
      And the tools are sorted by name with no duplicate names
      And every tool has a non-empty description
      And the first tool's inputSchema has type "object"

    Scenario: Input schemas use a closed, flat subset of JSON Schema
      When the admin calls GET /v1/tools
      Then every inputSchema has type "object", "properties", "required" and "additionalProperties" false
      And schemas use only the keys "type", "description", "properties", "required" and "additionalProperties"
      And every property type is one of "string", "integer" or "boolean"
      And every required field is listed in "properties"
      And a tool without required fields has "required" set to an empty list

  Rule: Calling tools (POST /v1/tools/{name})

    Scenario: A successful call wraps the tool result
      When the admin calls POST /v1/tools/bus_mint_leader with body {"name":"boss"}
      Then the response status is 200
      And the "Content-Type" header is "application/json"
      And the response body is {"result": <tool result>}

    Scenario: The first result after new mail also carries a mail field
      # The key is absent unless the caller's mailbox scope holds an unnoticed edge, and then
      # present on that one result only (mail-hint.feature). Strict clients must allow it.
      Given the leader "boss" has an empty mailbox
      And member "worker" sent a direct message to "boss" in space "proj"
      When leader "boss" calls POST /v1/tools/bus_whoami
      Then the response body is {"result": <tool result>, "mail": {"waiting": 1, "critical": 0, "spaces": ["proj"]}}
      And the next call's response body is {"result": <tool result>}

    Scenario: End-to-end happy path over the web API
      Given the admin called bus_mint_leader with {"name":"boss"} and got a leader token
      And leader "boss" called bus_create_space with {"space":"proj"}
      And leader "boss" called bus_invite_member with {"space":"proj","name":"worker"} and the member accepted the code with POST /v1/invites/accept
      And leader "boss" called bus_create_topic with {"space":"proj","topic":"news"}
      When member "worker" calls bus_send_topic with {"space":"proj","topic":"news","body":"hi"}
      And leader "boss" calls bus_inbox_read with {}
      Then the result is one inbox entry with body "hi", from "worker" and topic "news"

    Scenario: List results are JSON arrays, never null
      When a tool returns an empty list
      Then the result is encoded as []

    Scenario: Unknown tool
      When the admin calls POST /v1/tools/bus_nope
      Then the response status is 404
      And the call fails with not_found "tool bus_nope not found"

    Scenario: Request body over 1 MiB
      When the admin calls POST /v1/tools/bus_whoami with a body larger than 1 MiB
      Then the response status is 400
      And the call fails with invalid "request body exceeds 1 MiB"

    Scenario: Request body that cannot be read
      When the admin calls a tool and reading the request body fails with "<read error>"
      Then the response status is 400
      And the call fails with invalid "reading request body: <read error>"

  Rule: Tool arguments are decoded strictly

    # Unknown fields and wrong types are checked before required fields.

    Scenario Outline: Arguments that decode (<case>)
      When a tool with optional arguments "space" (string) and "n" (integer) is called with <body>
      Then the tool receives <decoded>

      Examples:
        | case   | body                 | decoded                 |
        | empty  | an empty body        | zero values             |
        | null   | null                 | zero values             |
        | fields | {"space":"a","n":3}  | space "a" and n 3       |

    Scenario Outline: Arguments that do not decode (<case>)
      When a tool with optional arguments "space" (string) and "n" (integer) is called with <body>
      Then the call fails with invalid "<message>"

      Examples:
        | case          | body       | message                                                                     |
        | unknown field | {"x":1}    | invalid arguments: json: unknown field "x"                                  |
        | wrong type    | {"n":"x"}  | invalid arguments: json: cannot unmarshal string into Go struct field ... |
        | malformed     | {          | invalid arguments: unexpected EOF                                           |

    Scenario: Unknown argument on a real tool
      When the admin calls POST /v1/tools/bus_mint_leader with body {"name":"x","bogus":1}
      Then the response status is 400
      And the call fails with invalid "invalid arguments: json: unknown field \"bogus\""

    Scenario Outline: Required arguments of a tool requiring "space" and "n" (<case>)
      When that tool is called with <body>
      Then <outcome>

      Examples:
        | case       | body                  | outcome                                   |
        | no args    | an empty body         | the call fails with invalid "space is required" |
        | missing n  | {"space":"a"}         | the call fails with invalid "n is required"     |
        | null space | {"space":null,"n":1}  | the call fails with invalid "space is required" |
        | ok         | {"space":"a","n":1}   | the tool receives space "a" and n 1             |

    Scenario: Missing required argument on a real tool
      When the admin calls POST /v1/tools/bus_mint_leader with body {}
      Then the response status is 400
      And the call fails with invalid "name is required"

  Rule: Application errors map to HTTP status codes

    Scenario Outline: Error kind <code>
      When a request fails with <error>
      Then the response status is <status>
      And the response body is {"error":{"code":"<code>","message":"<message>"}}

      Examples:
        | error                                      | status | code            | message        |
        | invalid "bad"                              | 400    | invalid         | bad            |
        | unauthenticated "who"                      | 401    | unauthenticated | who            |
        | forbidden "no"                             | 403    | forbidden       | no             |
        | not_found "gone"                           | 404    | not_found       | gone           |
        | a wrapped conflict "dup"                   | 409    | conflict        | dup            |
        | a non-application error "secret db detail" | 500    | internal        | internal error |

    Scenario: Internal error details are not leaked
      When a request fails with a non-application error "secret db detail"
      Then the response body does not contain "secret db detail"
      And the error is logged on the server

    Scenario: JSON responses
      When a handler writes {"a":1} with status 201
      Then the response status is 201
      And the "Content-Type" header is "application/json"
      And the response body is {"a":1} followed by a newline

  Rule: MCP at /mcp exposes the same tools

    # Stateless streamable HTTP with JSON responses; tools, plus the agent guide as instructions.

    Scenario: initialize returns the agent guide as instructions
      # Rule and budget in guide.feature.
      When a client connects to /mcp with a leader token and sends initialize
      Then the InitializeResult has instructions equal to the embedded guide

    Scenario: tools/list returns every registered tool sorted by name
      Given the registry holds tools "whoami" and "forbid"
      And leader "lead" connected to /mcp with its token
      When leader "lead" lists the MCP tools
      Then the tools are "forbid", "whoami" in that order
      And each tool carries the registry's description and input schema

    Scenario: The production registry over MCP
      When a client lists the MCP tools on the running bus
      Then it gets the same 34 tools as GET /v1/tools, sorted by name

    Scenario: A successful tool call returns JSON text and structured content
      Given leader "lead" connected to /mcp with its token
      When leader "lead" calls MCP tool "whoami" without arguments
      Then the result is not an error
      And the text content is the result JSON, containing "\"kind\":\"leader\""
      And the structured content is the same JSON object

    Scenario: A non-object result has text content only
      When an MCP tool call returns a JSON array
      Then the text content is the array JSON
      And the result has no structured content

    Scenario: A tool error comes back as an error result
      Given leader "lead" connected to /mcp with its token
      When leader "lead" calls MCP tool "forbid"
      Then the result is an error
      And the text content is "forbidden: nope"

    Scenario: Application error text format
      When an MCP tool call fails with not_found "tool x not found"
      Then the result is an error with text "not_found: tool x not found"

    Scenario: A non-application tool failure is not a tool result
      When an MCP tool fails with an error that is not an application error
      Then the MCP call fails with a protocol error instead of an error result

    Scenario Outline: MCP bearer authentication (<case>)
      When a client connects to /mcp <how>
      Then the HTTP request is rejected with status 401

      Examples:
        | case        | how                                  |
        | no token    | without an Authorization header      |
        | bad token   | with bearer token "amb_nope"         |

    Scenario: MCP tokens are the same tokens as the web API
      When a client connects to /mcp with the admin token or a leader or member token
      Then its tool calls run as that admin, leader or member

  Rule: Admin endpoints are loopback-only and need the admin token

    # POST /v1/admin/shutdown and POST /v1/admin/admin-token serve the local CLI (stop, rotate-token).
    # Any admin may shut down; only the bootstrap admin may replace the instance admin token.
    # Behind a reverse proxy every request arrives from loopback, so there the admin token alone protects them.

    Scenario Outline: Guard on <route> from <remote> with the <token> token
      When a client from <remote> calls POST <route> with the <token> token
      Then the response status is 403
      And the error is forbidden "<message>"

      Examples:
        | route                 | remote      | token        | message                                                       |
        | /v1/admin/shutdown    | 10.0.0.2    | admin        | admin endpoints are only available from localhost             |
        | /v1/admin/admin-token | 192.168.1.1 | admin        | admin endpoints are only available from localhost             |
        | /v1/admin/shutdown    | 127.0.0.1   | leader       | admin token required                                          |
        | /v1/admin/admin-token | 127.0.0.1   | minted admin | only the bootstrap admin may replace the instance admin token |

    Scenario Outline: Shutdown from <remote>
      When a client from <remote> calls POST /v1/admin/shutdown with the admin token
      Then the response status is 200 with body {"ok":true}
      And the server shuts down gracefully

      Examples:
        | remote    |
        | 127.0.0.1 |
        | ::1       |

    Scenario: Swapping the admin token
      Given a new token of at least 32 characters
      When a loopback client calls POST /v1/admin/admin-token with the admin token and body {"token":"<new token>"}
      Then the response status is 200 with body {"ok":true}
      And the old admin token gets 401 unauthenticated
      And the new admin token is accepted

    Scenario Outline: Invalid token swap: <case>
      When a loopback client calls POST /v1/admin/admin-token with the admin token and body <body>
      Then the response status is 400
      And the error is invalid with a message starting with "<message>"

      Examples:
        | case      | body          | message                                    |
        | too short | {"token":"x"} | admin token must be at least 32 characters |
        | bad JSON  | {             | invalid JSON body                          |

  Rule: Graceful shutdown

    Scenario: Stopping the server cancels streams and shuts down gracefully
      Given the server is listening on "127.0.0.1:0"
      When the server's context is cancelled
      Then all inbox streams are cancelled
      And in-flight requests get up to 10 seconds to finish
      And the server stops without error

    Scenario: An unusable listen address fails to serve
      When the server is started on address "bad:addr:x"
      Then it fails with an error starting with "serve:"
      And the inbox streams are not cancelled
