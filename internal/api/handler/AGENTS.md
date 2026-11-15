# internal/api/handler — HTTP Handlers

Before editing or reviewing this area, read the applicable sections of
[local requirements](AGENT-REFERENCE.md). Read the complete reference when changing
architecture, contracts, authorization, state transitions or cross-module behavior.
The reference retains all requirements previously held here.


## Architecture

Read [Architecture](AGENT-REFERENCE.md#architecture) before work involving this topic.

## Auth Handlers (`auth.go`)

Read [Auth Handlers (`auth.go`)](AGENT-REFERENCE.md#auth-handlers-authgo) before work involving this topic.

## Every `/api/v1/me/*` route refuses an anonymous caller (`signed_in_door.go`)

Read [Every `/api/v1/me/*` route refuses an anonymous caller (`signed_in_door.go`)](AGENT-REFERENCE.md#every-apiv1me-route-refuses-an-anonymous-caller-signed_in_doorgo) before work involving this topic.

## Account Deletion (`me_delete.go`)

Read [Account Deletion (`me_delete.go`)](AGENT-REFERENCE.md#account-deletion-me_deletego) before work involving this topic.

## User Job Handlers (`user_jobs.go`)

Read [User Job Handlers (`user_jobs.go`)](AGENT-REFERENCE.md#user-job-handlers-user_jobsgo) before work involving this topic.

## Mail Inbox + Harness Surface (`gmail.go`, `inbox.go`, `inbox_linking.go`, `inbox_harness.go`)

Read [Mail Inbox + Harness Surface (`gmail.go`, `inbox.go`, `inbox_linking.go`, `inbox_harness.go`)](AGENT-REFERENCE.md#mail-inbox--harness-surface-gmailgo-inboxgo-inbox_linkinggo-inbox_harnessgo) before work involving this topic.

## Assistant (`assistant.go`, `assistant_*_tools.go`)

Read [Assistant (`assistant.go`, `assistant_*_tools.go`)](AGENT-REFERENCE.md#assistant-assistantgo-assistant__toolsgo) before work involving this topic.

## Talent Network catalogue (`talent_catalog.go`)

Read [Talent Network catalogue (`talent_catalog.go`)](AGENT-REFERENCE.md#talent-network-catalogue-talent_cataloggo) before work involving this topic.

## The caller's own membership (`me_talent_network.go`)

Read [The caller's own membership (`me_talent_network.go`)](AGENT-REFERENCE.md#the-callers-own-membership-me_talent_networkgo) before work involving this topic.

## Résumé / CV (`resume.go`, `cv_roast.go`)

Read [Résumé / CV (`resume.go`, `cv_roast.go`)](AGENT-REFERENCE.md#résumé--cv-resumego-cv_roastgo) before work involving this topic.

## Application forms (`apply_form.go`)

Read [Application forms (`apply_form.go`)](AGENT-REFERENCE.md#application-forms-apply_formgo) before work involving this topic.

## Pagination (`handler.go`)

Read [Pagination (`handler.go`)](AGENT-REFERENCE.md#pagination-handlergo) before work involving this topic.

## Error Convention

Read [Error Convention](AGENT-REFERENCE.md#error-convention) before work involving this topic.
