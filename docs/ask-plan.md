# AI mode: asking the data in plain language

Built 2026-11-05. An intelligence analyst asks about the criminal scenario ("how many occurrences of
type X in the period"), or looks up a person and follows them through cases, occurrences and inquiries
without clicking, and a language model answers from the case system's data. The page is
`/intelligence/ask`; the code is the `ask` package and `api/ask_handler.go`.

## Decisions

**Tools, not SQL.** The model never writes SQL and never sees the database. It chooses among a fixed
catalog of read-only tools and fills in their filters; the tools run parameterized queries
(`db/queries/ask.sql`), and every count comes from them. Text-to-SQL would answer more questions,
but it would put an untestable query behind each number and hand the model the whole schema,
persons and biometrics included. A question the catalog cannot answer gets "I can't answer that",
and the catalog grows when analysts need more.

**Numbers are checked, and shown.** The page shows the database's result beside the model's prose,
as its own table: the analyst reads the figure from the query, not from the model. After the model
answers, `unverifiedNumbers` lists any number in the prose that no tool returned. A number listed
may be an honest sum or percentage; the warning says only that no query produced it. The check does
not catch a real number attached to the wrong thing, which is why the results are shown too.

**Any server speaking the OpenAI protocol.** One client, configured by environment only: a local
server (Strata, Ollama, llama.cpp) or another provider, with no code for any in particular. The server
must support tool calling (`tools`, `tool_calls`). Servers disagree on which request fields they require
or reject, so `ASK_EXTRA_BODY` adds whatever one needs instead of the code guessing.

**An outside server only reads synthetic data.** The endpoint's host decides where the data goes: a
loopback or private address, or a host name with no dots, is inside; anything else is outside. Before
calling an outside server, `Service.run` checks that every occurrence and case description starts with
`SIMULATED` and every person's id with `SIM-` (trackid-sim's marks), and otherwise refuses, without calling
the model. There is no switch to turn this off: a deployment holding real data uses a local model. The page
warns while the model is outside. Occurrences, cases and persons are what the tools mainly return, so they are
what the check reads.

**Intelligence only, recorded.** The routes sit behind `RequireIntelligence`, so an analyst working blind
cannot ask, as with the map and the clusters (docs/access-profiles-plan.md). `ask_queries` keeps who
asked what, which server answered, the conversation so far, the tools' arguments and results, and the
answer or the error. Like `access_log` it is append-only, and an answer that cannot be recorded is not
given.

**Each read is logged as its page would log it.** A tool's reads go to `access_log`, with the route
`/api/ask#<tool>`: `person_cases`, `person_clusters` and `person_identities` as PERSON reads, `identity_conflicts` as a
PERSON read of everyone it returns, and each case that `case_detail`, `occurrence_detail` or
`inquiry_detail` opens as a CASE read, unless the analyst holds one of its analyses (`api.CaseAccess`). A read that
cannot be logged is not served. `search_persons` is not logged, as the person search isn't: it shows who exists,
not their history, and neither is `cluster_detail`, as the cluster page isn't. Otherwise the conversation would be a
way around the audit.

**Both link layers, always labelled.** A person reaches a case through a cluster holding a KNOWN feature of theirs
and a QUESTIONED trace of the case. Each link says `REVIEWED` (examiners confirmed it) or `LEAD` (only the matcher
made it). The person pages read the reviewed layer alone (docs/reviewed-clusters-plan.md), but analysts already see
automatic links on the Clusters page, and leads are much of what intelligence works with. The model must say which
it is and never present a lead as an identification. A link also says nothing of what someone did: nothing records
whether they were a victim, suspect or witness, so the wording is "biometrically linked", never "took part".

**Another identity is reported as evidence, never as a finding.** `person_identities` answers "may this person be
using another name?" from the enrollment-to-enrollment comparisons already stored, per modality, with which documents
matched which. It sums them up deterministically: `REVIEWED`, `TWO_MODALITIES`, `ONE_MODALITY`, `INDIRECT` (a cluster with
no direct comparison) or `HUMAN_NEGATIVE`. It says nothing of who holds the true identity: an impostor record is filed
under the other person's name, so someone listed may be the one whose identity was taken. A case with two people on one
trace flags them (`sameTraceAs`) rather than presenting two people present.

On trackid-sim's world (159 persons), whose planted identities are known, all 23 planted pairs were found and none missed:
face and fingerprint together gave 21 true and 0 false, including all 15 impostor records; one biometric 2 true and 11
false; indirect 0 true and 9 false; inconclusive only 0 true and 106 false, so those are counted and not listed. This
shows the tool's behaviour on one synthetic world, not error rates on real data.

**The cluster is the unit, and conflicts are the clusters failing to match identities one to one.**
`cluster.ListConflicts` is the one definition, read by the AI mode, the Clusters list, the cluster page and the
person page. A cluster is one biometric, so it should be one identity's; an identity is one body, so it should sit in
one cluster per modality. That gives two kinds: `ONE_BIOMETRIC_MANY_NAMES` and `ONE_IDENTITY_MANY_BIOMETRICS`.

- A cluster counts as identified when it holds an enrollment, whether or not anyone has reviewed it, so a conflict
  surfaces before anyone has looked. What people concluded comes alongside: the strength, and each person's status,
  their strongest reviewed link to case evidence (a lead, or a laudo at or above the identification level).
- Both kinds are raised on the AUTOMATIC layer. The REVIEWED layer cannot raise the second: a sample nobody reviewed
  sits alone there, and "not confirmed the same" is not "different".
- Several documents with one biometric and one name are normal, and so is a spelling variant ("Sillva"): neither is a
  conflict. Names count as one within two edits, or once initials are dropped.
- Weak conflicts are listed, labelled and ranked last. A cluster of three or more people can be a chain of lookalikes,
  so the strength is that of its strongest pair and the comparisons say which pair that is.
- The matcher stores what it found alike, so a pair with no decision says `NO_DECISION` outright. Faces can be measured
  from their embeddings, which shows two records differ; fingerprints have no such measure in a read query.
- It is computed on each call, about 30 ms on trackid-sim's world, so a decision counts at once and every surface
  agrees. On a much larger register this is the first thing to turn into a table.
- The pages compare the people's registers side by side, with birth dates and parents' names, and mark what differs.
  `cluster.AddRegisters` loads those, and only the pages call it, so parents' names never reach the model. A person page's
  comparison shows the other people in the conflict, so each of them is logged as read.
- An identity is not a person row. Nothing merges people: a person is whatever the importer sends, and a document of
  another type or from another state can be a row of its own. Records are one identity when they share a person row or
  a fiscal number, or when they share a name and birth date and no fiscal number says otherwise. Different fiscal numbers
  are two people however alike the names, and a record without one that could belong to either stays apart from both.
  `identityBasis` says which it was, and one bound by name and birth date alone may be homonyms. This reads the fields
  `bulk`'s identity key reads, but is a different test: calibration needs pairs it can call the same or different, and a
  fake document with no fiscal number must still count here.

On trackid-sim's world, whose planted identities are known: the second kind flagged 15 people, exactly those holding an
impostor record. The first caught all 15 impostor records and 2 duplicates whose names really differ, and left the 6
same-name duplicates alone; its 3 weak conflicts held no planted identity. As above, this is behaviour on one synthetic
world, not an error rate.

**People with the same name are never merged.** Person tools take a person's number, never a name, so every walk
goes through `search_persons`. With several matches the model lists them with what tells them apart and asks.

**Names go to the model; little else about a person does.** The name is in the question, and the model needs names
to tell people apart. It sees names, birth dates and document numbers, and not parents' names, photos or biometric
data, which stay behind a link to the person's page. A local model keeps them on the institute's server, and an
outside one only ever gets synthetic ones.

**Follow-ups run on references, not on prose.** Each answer returns the entities its tools found (person, case,
occurrence, inquiry; `Answer.Entities`), taken from the results. The page sends them back with the next question, so
"her cases" and "the second case" resolve from the data, and `ask_queries.focus` records them. They only tell the
model what is being referred to: the tools check everything again.

**Places are what they are.** Distances come from `occurrences_near`, not from the model. A point known only to the
municipality is its centre rather than a place, so it is not searched around, not marked on a map and is counted
as left out of nearby searches. An occurrence may have no date or place, or a window rather than a moment.

## Tools

| Tool | Answers |
|---|---|
| `data_coverage` | what the database holds: totals, first and last dates, undated and unplaced |
| `count_occurrences` | how many, by type, group, period, status, attempt, agency, state, municipality, neighbourhood or place type; broken down by any of them, or by month or year |
| `list_occurrences` | which ones, at most 50, newest first, with their cases |
| `count_cases` | biometric cases, by modality, nature or link, and by their occurrence's type and period |
| `find_municipality` | a municipality's IBGE code from its name, ignoring case and accents |
| `identity_conflicts` | across the database, who may hold a false identity: both kinds, by strength, with clusters, people and comparisons |
| `cluster_detail` | one cluster: its people, cases and occurrences, linked clusters of the other modality, conflicts |
| `person_clusters` | the clusters holding a person's samples, of both layers, and the conflicts involving them |
| `search_persons` | people by name: number, names, birth dates, documents, cases by link |
| `person_cases` | a person's cases, each with its link and its occurrence's date, types, place and inquiry |
| `person_identities` | other enrollments matching a person's: the evidence per modality, and which documents matched |
| `case_detail` | a case: its occurrence, requests, issued reports (number and date, not contents), linked people |
| `find_occurrence` | occurrences by part of a TrackID, police or inquiry number, or an agency |
| `occurrence_detail` | one occurrence: when, where (coordinates and precision), what, its cases and people |
| `inquiry_detail` | every occurrence sharing an inquiry (IPL) number, with their cases and people |
| `occurrences_near` | occurrences within a distance of another or of a point, by type and period, nearest first |

What the counts mean, each stated in the tool's result or description so the answer can say so:

- A period counts by `occurred_from`, the start of an occurrence's window, cut in São Paulo time. Both
  ends are inclusive days.
- An occurrence with no date is outside any period and one with no place is outside any place filter;
  the result reports how many that left out (`undatedExcluded`, `unplacedExcluded`).
- An occurrence with several types counts once in the total and once under each type or group, so a
  breakdown can add up to more than the total.
- The data are registered occurrences, not all the crime there is; the model is told not to present
  them as rates, causes or predictions.

A mistake in a call (an unknown type, a date that doesn't parse, a field the tool doesn't have) goes back
to the model as an error it can correct. An unknown field is refused rather than ignored: a filter
silently dropped would give a wrong count that looks right.

## Configuration

See `.env.example`. `ASK_ENDPOINT` and `ASK_MODEL` choose the model; with neither set the AI mode shares
the assist model's settings, and with those unset too it is off. `ASSIST_*` is only used as a whole, so
an API key never reaches another server.

## Limits and next steps

- The answer arrives whole, with a timer on the page. Streaming the steps as they run would help a slow
  local model.
- The conversation lives in the page. Prior answers go back as text and what they found as references, but a
  follow-up's figures still have to come from a new tool call: numbers from earlier answers count as unverified.
- The number check is weak on long results. A count the model tallies from rows is not flagged when that number
  happens to appear somewhere in them, so the tables beside the answer remain the check. Tools returning the
  tallies themselves would be firmer.
- Only intelligence analysts can ask. Examiners have narrower rights, and context can bias their work, so they
  need their own rules before this reaches them.
- Associates (people linked to the same occurrence through different traces), hotspots, rankings, description search (for modus operandi questions) and clusters are not there
  yet; each needs its own decision about what the model may read.
- Person search matches accents exactly, as the person page's does, so the model retries with the other spelling.
- A long walk is many tokens, since each turn carries the prompt, the tools and the results so far: about 19 000 to
  39 000 input tokens a question in the first tests. It matters with a paid provider and on a slow local model.
- A hostile description in the data could try to steer the model. The prompt tells it results are data,
  and the blast radius is small because every tool only reads, but the answer's prose could still be
  misled: the results table beside it is the check.
- There is no evaluation set yet: how often a given model picks the right filters is measured by trying
  it. A fixed list of questions with known answers over the synthetic world would make models comparable.
