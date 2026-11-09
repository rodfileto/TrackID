# Intelligence operations

An operation is a named line of intelligence work: what analysts are following, and everything that
belongs to it, in one place. Today cases, persons, occurrences and clusters are only reached one by
one, so the work that ties them together lives outside TrackID.

## Roadmap

1. **The entity** (done). `operations`: a name, a stable slug, a description, active or closed. List,
   create and edit under the intelligence capability, and a page per operation. An organization can
   attach a panel of its own to an operation by its slug (`Extension.operationPanels`), for what only
   it has, such as a connection to another system.
2. **Links.** `operation_links` ties an operation to biometric cases, persons, occurrences and
   clusters: one table with a kind and a reference, as `article_mentions` does, recording who linked
   what, when and why. Unlinking keeps the record. Links are added from the operation's page and from
   each linked item's page, which also lists the operations it belongs to.
3. **The operation's picture.** Its page shows the linked items: the occurrences on the map, the
   persons and clusters as the graph, and the cases with their analyses, using the existing views
   limited to what is linked.
4. **Suggestions.** Items connected to what is already linked (a cluster of a linked person, the
   occurrence of a linked case) are offered for a person to accept. Nothing is linked automatically.
5. **Access and audit.** Who may see each operation: members, not every analyst, since an operation
   can be more sensitive than its parts. Reads go to `access_log`, as reads of persons and cases do.
   Until then, every analyst sees every operation, so the entity holds nothing but its description.
6. **The article and the AI mode.** An operation gets an article, like a person's, and the AI mode can
   be asked about one operation, within what its links reach.

## Decisions

- **Links don't grant access.** Linking a case to an operation doesn't change who can open the case.
- **Closed is not deleted.** An operation's history stays; there is no delete.
- **The slug doesn't change**, so an organization's panel and any link to the page keep working when
  the operation is renamed.
