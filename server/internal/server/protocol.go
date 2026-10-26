package server

// Protocol is the bus protocol version reported by GET /healthz in the Agent-Bus-Protocol
// header; clients refuse servers older than they need (contracts.md §6).
//
//	2: invite task as a direct message, accept without task, kind/role admin,
//	   bus_list_spaces.admins, inbox entry field class.
const Protocol = 2
