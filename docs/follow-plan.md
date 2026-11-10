# Following persons and clusters

Built 2026-11-10 on phase 1 of `docs/realtime-plan.md` (the hub, `notifications`, the bell). Migration `023_follows`,
packages `follows` and `teams`.

## What it does

An intelligence analyst follows a person or a cluster and is told, live, when it changes:

- **Person:** a new identity document, one of their features joins a new AUTOMATIC cluster, they are linked to a new case.
- **Cluster:** new members, members that left (a split), a merge into another cluster.
- **A deployment's changes:** anything a deployment records with `follows.RecordFeature`, such as PF's FindFace camera
  hits (`trackid-pf/docs/follow-findface-plan.md`).

A user who sets a Microsoft Teams webhook also gets every notification, of any kind, in Teams.

## Decisions

1. **Only explicit follows, and only by intelligence analysts.** The follow routes sit behind `RequireIntelligence`,
   the fan-out tells only active analysts, and the bell shows what changed only to an analyst. An analyst may open any
   case, person or cluster, so a notification names cases without checking access case by case.
2. **The AUTOMATIC layer, merges included.** Speed matters more than forensic certainty here, so SYSTEM matches count.
   REVIEWED clusters are not watched: following one follows the AUTOMATIC cluster it sits inside, and the page says so.
3. **A new case is any case linked through an AUTOMATIC cluster:** a QUESTIONED trace of the case shares an AUTOMATIC
   cluster with one of the person's KNOWN features. The person page lists cases through REVIEWED clusters only, so the
   bell can name a case that page does not show yet.
4. **Generic in core.** Follows, the change log, the fan-out, grouping and Teams live in core. A deployment records its
   changes with `RecordFeature` and labels them through its frontend translations
   (`notifications.change.<CHANGE>_one/_other`).
5. **Triggers record changes to core's tables,** whichever process writes them: `identity_documents` (insert, or a new
   `person_id`), `cluster_members` (AUTOMATIC insert and delete) and `cluster_merges` (AUTOMATIC). `cluster.apply` deletes
   and inserts a member only when it moves, so the triggers see real changes only.
6. **One unread notification per user and subject.** Further changes extend it ("3 new documents, 1 new case") and move
   it to the top, so an import that adds hundreds of documents to one person doesn't flood the bell.
7. **Built when read.** A notification holds its subject and a range of change IDs; the counts are summed when it is read.

## Data

| Table | Notes |
|---|---|
| `follows` | `(user_id, subject_kind, subject_id)`. `PERSON` is `persons.person_id`, `CLUSTER` an AUTOMATIC `clusters.id` |
| `subject_changes` | The change log and the fan-out's queue: `change`, `ref`, `amount` (how many the row stands for), `fanned_out_at`. A flag, not a cursor, since transactions commit out of ID order |
| `notifications` | Adds `updated_at`, `first_change_id`, `last_change_id`, `teams_sent_at`. The `kind` CHECK is gone; a partial unique index on unread `(user_id, kind, subject)` for `PERSON_CHANGED` and `CLUSTER_CHANGED` makes grouping an upsert |
| `users` | Adds `teams_webhook` (encrypted), `teams_webhook_set_at`, `teams_last_error`, `teams_failed_at` |

| `change` | Subject | `ref` | Source |
|---|---|---|---|
| `DOCUMENT_ADDED` | PERSON | `identity_documents.id` | trigger |
| `MEMBER_ADDED` / `MEMBER_LEFT` | CLUSTER | feature ID | trigger |
| `MERGED` | CLUSTER (retired) | surviving cluster ID | trigger |
| `CLUSTER_JOINED` | PERSON | cluster ID | fan-out |
| `CASE_LINKED` | PERSON | `biometric_cases.case_id` | fan-out |
| a deployment's | PERSON and CLUSTER | its own | `RecordFeature` |

`RecordFeature(ctx, q, featureID, change, ref, amount)` runs in the deployment's transaction. It reaches the feature's
AUTOMATIC cluster, the persons whose KNOWN features sit in it and, for a KNOWN feature, its own person, even unclustered.

## Fan-out

`follows.Run` runs in the API process. It wakes on `trackid_subject_changes` and polls every 30 seconds in case a
signal was missed. `Fanout` takes every pending change in one repeatable-read transaction, under an advisory lock:

1. **Person changes, for followed persons only.** It rebuilds each one's AUTOMATIC clusters as they stood before the
   batch: a member whose first change in the batch is `MEMBER_LEFT` was in that cluster, and one whose first change is
   `MEMBER_ADDED` was not. Clusters and cases linked now but not before become `CLUSTER_JOINED` and `CASE_LINKED`.
   Comparing before with after keeps a merge, or a member moving, from reporting a case the person was already linked
   to, and a case reaches a person once however many of its traces join. A cluster renamed by a merge is not "joined".
2. **Notify** each follower of each changed subject, inserting the notification or extending the unread one, and
   signal `trackid_live` so the bell refreshes and Teams hears of it.
3. **Merges** move the retired cluster's follows to the survivor. The bell shows "merged into #N" and links there; the
   retired cluster's members leaving are not counted.
4. **Splits** show as members that left. The new cluster is not followed automatically.

Changes nobody follows are only marked. A daily prune deletes changes handled more than 90 days ago that no unread
notification still reports.

## Teams

Based on naturaliza's `send_teams_message` (`apps/users/services.py`): an Adaptive Card posted to a Power Automate
"post card in a chat or channel when a webhook request is received" flow.

- **No content.** The card says only what kind of notification is waiting ("a person you follow has changes") and its
  button opens `APP_URL/?notifications=1`, which opens the bell. No person, cluster or case, not even in the link.
- **After commit.** `notifications.Relay`'s listener hands each new or extended notification's ID to a queue with two
  senders, 10-second timeout, no redirects. The action never waits on Microsoft. A full queue, or an API restart,
  drops the Teams message; the bell still has the notification.
- **Grouped notifications** are sent again only if `teams_sent_at` is over 15 minutes old. A read notification is not sent.
- **Best-effort.** A failure is logged and kept on the user (`teams_last_error`), which the profile page shows. Errors
  never carry the URL.
- **The URL is a credential.** Encrypted with AES-GCM under `SECRETS_KEY`; without it Teams is off. The API never
  returns it. It must be `https` to a host ending in `TEAMS_WEBHOOK_HOSTS` (default `.logic.azure.com`,
  `.powerplatform.com`, `.webhook.office.com`), checked when set and before each send.
- **Routes:** `GET/PUT/DELETE /api/me/teams-webhook`, `POST /api/me/teams-webhook/test`. The profile page has the card.

Settings: `SECRETS_KEY`, `APP_URL`, `TEAMS_WEBHOOK_HOSTS`, `TEAMS_LANGUAGE` (`pt-BR` or `en`).

## API and frontend

- `GET /api/follows`, and `GET/PUT/DELETE /api/follows/:kind/:id` (`person` or `cluster`), for analysts. Each answers
  with the subject actually followed.
- A Follow button on the person and cluster pages, for analysts.
- The bell lists person and cluster notifications with their counts, linking to the person, or to the cluster (or the
  one it merged into).

## Still open

- **Bulk enrollment** records a change for every member it clusters. Grouping keeps it to one notification per
  followed subject, but the log grows by one row per member until pruned.
