# Design — specs and architecture

This folder holds the design inputs of the bus. It was committed in slice 0 so that every slice, pull request and review can link to tracked files.

## `specs/` — the frozen input

`specs/` is a verbatim copy of the reviewed design scenarios and their analyses, one folder per topic (`admin/`, `findings/`, `guidance/`, `mail/`, `omp-parity/`, `web/`) plus the cross-folder [`specs/REVIEW.md`](specs/REVIEW.md). The scenarios are the **input**, not the running specification: when a slice implements a scenario, the rules move to [`server/spec/`](../../server/spec/) with the scenario text kept verbatim, and the copy here is **not** edited afterwards. Between the move and the implementation, the scenario in `server/spec/` is the authoritative one; until then the copy in `specs/` is.

## `arch/` — the architecture

`arch/` holds the architecture documents: the [backlog](arch/BACKLOG.md) of all slices with their order and dependencies, the [definition of ready / definition of done](arch/definition-of-ready.md), the [contracts](arch/contracts.md) that fix every client-visible change, the [questions for the product owner](arch/QUESTIONS.md), the [server](arch/server.md), [schema](arch/schema.md), [client](arch/client.md), [GUI](arch/gui.md), [pipeline](arch/pipeline.md) and [slice](arch/slices.md) descriptions (plus the [client slices](arch/slices-client.md) and the [bead map](arch/BEADS-MAP.md)).

`arch/contracts.md` and `arch/BACKLOG.md` stay **living**: they change only by pull request, when a slice changes a contract or the backlog changes. The other files are edited only when the design itself changes.

Paths inside these documents: spec paths without a prefix (e.g. `admin/analysis.md`) are relative to `docs/design/specs/`; contract sections are `docs/design/arch/contracts.md` §n.
