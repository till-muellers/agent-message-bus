package server

// Protocol is the bus protocol version reported by GET /healthz in the Agent-Bus-Protocol
// header; clients refuse servers older than they need (contracts.md §6).
//
//	2: invite task as a direct message, accept without task, kind/role admin,
//	   bus_list_spaces.admins, inbox entry field class, bus_wait, bus_space_overview,
//	   the mail notice and mail hint, receipts and connected, GET /v1/guide and MCP instructions.
const Protocol = 2
