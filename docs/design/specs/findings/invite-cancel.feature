# MOVED (PO-3, round 6). Cancelling an invite is not a tool: there is no
# bus_cancel_invite for omp or MCP agents, and the registry stays at 34 server tools.
# Cancelling exists only as a web API endpoint of the GUI, by an opaque invite id:
#   docs/design/specs/web/gui-invite-cancel.feature
# /bus spawn exposes no cancel handle. bus_revoke_member still cancels every pending
# code of a name (server/spec/invites.feature).
