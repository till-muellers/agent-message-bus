# SUPERSEDED — do not implement.
#
# The "admin token is the superuser @admin" design (reserved agent row "@admin",
# no inbox, never a participant, single admin token) was replaced by a new PO ruling:
# `admin` is a full agent kind with named agents and their own inboxes. Admins act as
# leaders in every space without joining, are reachable only in spaces they joined, and
# can mint and revoke further admins. The bootstrap admin is "admin" behind the
# instance admin token.
#
# The current specification lives in docs/design/specs/admin/:
#   admin-role.feature       implicit leader authority in every space
#   admin-messaging.feature  join/leave, being messaged, the admin's inbox
#   admin-tokens.feature     bootstrap admin, bus_mint_admin, bus_list_admins, bus_revoke_admin, rotation
#   schema-reset.feature     one fresh schema; old databases are not detected and are deleted by hand
#   analysis.md              current state, seams, change list, open questions
#
# The participant-row inventory of the superseded round is re-rated in
# docs/design/specs/admin/analysis.md §4.1.
